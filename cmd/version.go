package cmd

import (
	"fmt"

	"github.com/c-mueller/ts-restic-server/internal/buildinfo"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version and build information",
	Run: func(cmd *cobra.Command, args []string) {
		info := buildinfo.Get()
		fmt.Printf("ts-restic-server %s\n", info.Version)
		fmt.Printf("  commit:     %s\n", info.Commit)
		fmt.Printf("  built:      %s\n", info.BuildDate)
		fmt.Printf("  channel:    %s\n", info.Channel)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
