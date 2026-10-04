package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// configFileUsed is the config file loaded by initConfig ("" if none).
var configFileUsed string

// configErr holds a config discovery or parse error from initConfig.
// Commands that depend on the config must return it before doing work.
var configErr error

var rootCmd = &cobra.Command{
	Use:   "ts-restic-server",
	Short: "A Restic REST server with pluggable storage backends",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./config.yaml|yml, then /etc/ts-restic-server/config.yaml|yml)")
}

func initConfig() {
	configFileUsed = ""
	configErr = nil

	viper.SetEnvPrefix("RESTIC")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	path, err := config.FindConfigFile(cfgFile, config.DefaultSearchDirs)
	if err != nil {
		configErr = err
		return
	}
	if path == "" {
		return
	}

	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		configErr = fmt.Errorf("reading config file %s: %w", path, err)
		return
	}
	configFileUsed = path
}
