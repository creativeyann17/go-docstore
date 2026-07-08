// cmd/godocstore/version_cmd.go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, commit and build date",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("godocstore %s (commit %s, built %s)\n", version, commit, date)
	},
}

func init() { rootCmd.AddCommand(versionCmd) }
