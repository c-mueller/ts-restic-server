package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/c-mueller/ts-restic-server/internal/tsauth"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"tailscale.com/ipn"
	"tailscale.com/tsnet"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Register the Tailscale node once so serve runs without an auth key",
	Long: `Registers the Tailscale node with the configured hostname and state_dir,
waits until it is up, fetches its TLS certificate and exits. Afterwards serve
needs no auth key; the key only exists for the duration of this call.

The auth key is taken from, in order: --auth-key, --auth-key-stdin, the
environment (TS_AUTHKEY, TS_AUTH_KEY, RESTIC_TAILSCALE_AUTH_KEY) or
tailscale.auth_key in the config file (still supported, not recommended).

If the state dir already holds a valid node identity, init does nothing and
exits 0, so it is safe to run repeatedly (e.g. from Ansible). Run it as the
user that runs serve, so the state dir stays readable for serve.`,
	Example: `  echo "$TS_AUTHKEY" | ts-restic-server init --auth-key-stdin
  TS_AUTHKEY=tskey-auth-... ts-restic-server init --config /etc/ts-restic-server/config.yaml`, // pragma: allowlist secret
	Args: cobra.NoArgs,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().String("auth-key", "", "Tailscale auth key (visible in the process list; prefer --auth-key-stdin or TS_AUTHKEY)")
	initCmd.Flags().Bool("auth-key-stdin", false, "read the Tailscale auth key from the first line of stdin")
	initCmd.Flags().Duration("timeout", 2*time.Minute, "how long to wait for the node to come up")
	initCmd.Flags().Bool("force", false, "re-register with the given auth key even if node state exists")
	initCmd.Flags().Bool("verbose", false, "print tsnet logs")
}

func runInit(cmd *cobra.Command, args []string) error {
	if configErr != nil {
		return configErr
	}

	// Lenient: init only needs the tailscale section, storage secrets may
	// be unset in this context. ValidateForInit rejects leftover placeholders.
	cfg, err := config.Load(true)
	if err != nil {
		return err
	}
	if err := cfg.ValidateForInit(); err != nil {
		return err
	}

	flagKey, _ := cmd.Flags().GetString("auth-key")
	fromStdin, _ := cmd.Flags().GetBool("auth-key-stdin")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	force, _ := cmd.Flags().GetBool("force")
	verbose, _ := cmd.Flags().GetBool("verbose")

	var stdin io.Reader
	if fromStdin {
		stdin = cmd.InOrStdin()
	}
	key, err := tsauth.ResolveAuthKey(tsauth.Sources{
		Flag:      flagKey,
		Stdin:     stdin,
		Getenv:    os.Getenv,
		Config:    cfg.Tailscale.AuthKey,
		ConfigRaw: viper.GetString("tailscale.auth_key"),
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	stateDir := cfg.Tailscale.StateDir
	stateExisted := tsauth.StateExists(stateDir)

	if force && key.Value == "" {
		return errors.New("--force needs an auth key (--auth-key, --auth-key-stdin or TS_AUTHKEY)")
	}
	if !stateExisted && key.Value == "" {
		return fmt.Errorf("no auth key given and no Tailscale state in %s: pass --auth-key, --auth-key-stdin or set TS_AUTHKEY", stateDir)
	}
	if key.Source == tsauth.SourceConfig {
		fmt.Fprintln(errOut, "warning: using tailscale.auth_key from the config file; this works but is not recommended, pass the key to init instead and remove it from the config")
	}
	if os.Geteuid() == 0 {
		fmt.Fprintln(errOut, "warning: running as root; the state dir will be owned by root and unreadable for a non-root serve (use sudo -u <service-user>)")
	}

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("create tailscale state directory %s: %w", stateDir, err)
	}
	if force {
		// tsnet ignores the auth key when state exists unless forced.
		os.Setenv("TSNET_FORCE_LOGIN", "1")
	}

	tsServer := &tsnet.Server{
		Hostname: cfg.Tailscale.Hostname,
		Dir:      stateDir,
		AuthKey:  key.Value,
		UserLogf: func(string, ...any) {},
	}
	if verbose {
		logf := func(format string, a ...any) { fmt.Fprintf(errOut, format+"\n", a...) }
		tsServer.UserLogf = logf
		tsServer.Logf = logf
	}
	defer tsServer.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	lc, err := tsServer.LocalClient() // starts tsnet
	if err != nil {
		return fmt.Errorf("start tailscale: %w", err)
	}
	watcher, err := lc.WatchIPNBus(ctx, ipn.NotifyInitialState)
	if err != nil {
		return fmt.Errorf("watch tailscale state: %w", err)
	}
	defer watcher.Close()

	err = tsauth.WaitRunning(watcher.Next, tsauth.WaitOptions{
		OnNeedsMachineAuth: func() {
			fmt.Fprintln(errOut, "node is waiting for approval in the Tailscale admin console ...")
		},
	})
	switch {
	case errors.Is(err, tsauth.ErrNeedsLogin) && key.Value == "":
		return fmt.Errorf("node state in %s is no longer valid; run init again with an auth key", stateDir)
	case errors.Is(err, tsauth.ErrNeedsLogin):
		return fmt.Errorf("the auth key was not used to log in (existing state in %s?); re-run with --force", stateDir)
	case err != nil && ctx.Err() != nil:
		return fmt.Errorf("node did not come up within %s; check connectivity and the auth key (use --force if the node was removed from the tailnet): %w", timeout, err)
	case err != nil:
		return err
	}

	status, err := lc.Status(ctx)
	if err != nil {
		return fmt.Errorf("read tailscale status: %w", err)
	}
	domains := tsServer.CertDomains()
	if len(domains) == 0 {
		return errors.New("tailnet provides no certificate domain; enable MagicDNS and HTTPS certificates in the Tailscale admin console")
	}
	if _, _, err := lc.CertPair(ctx, domains[0]); err != nil {
		return fmt.Errorf("fetch TLS certificate for %s: %w", domains[0], err)
	}

	ip := "-"
	if len(status.TailscaleIPs) > 0 {
		ip = status.TailscaleIPs[0].String()
	}
	if stateExisted && !force {
		fmt.Fprintf(out, "already initialized: %s (%s), state in %s; the auth key was not used\n", domains[0], ip, stateDir)
	} else {
		fmt.Fprintf(out, "initialized: %s (%s), state in %s\n", domains[0], ip, stateDir)
	}
	return nil
}
