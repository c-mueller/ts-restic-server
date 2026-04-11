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
		fmt.Printf("ts-restic-server %s\n", buildinfo.Version)
		fmt.Printf("  commit:     %s\n", buildinfo.Commit)
		fmt.Printf("  built:      %s\n", buildinfo.BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
