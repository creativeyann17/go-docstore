// cmd/godocstore/import_cmd.go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import <collection> <file|dir>",
	Short: "Import JSON file(s) as documents (migrate files → DB)",
	Long: `Import one .json file (id = file name without extension) or a
directory tree (ids = relative paths without extension, so
"users/id1.json" becomes document id "users/id1").`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStoreCreate()
		if err != nil {
			return err
		}
		defer s.Close()
		c, err := s.Collection(args[0])
		if err != nil {
			return err
		}

		info, err := os.Stat(args[1])
		if err != nil {
			return err
		}
		if info.IsDir() {
			n, err := c.ImportDir(args[1])
			if err != nil {
				return err
			}
			fmt.Printf("imported %d document(s) into %s\n", n, args[0])
			return nil
		}
		if err := c.ImportFile(args[1]); err != nil {
			return err
		}
		fmt.Printf("imported 1 document into %s\n", args[0])
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export <collection> <dir>",
	Short: "Export every document to <dir> as pretty-printed .json files",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()
		c, err := s.Collection(args[0])
		if err != nil {
			return err
		}
		n, err := c.ExportDir(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("exported %d document(s) to %s\n", n, args[1])
		return nil
	},
}

func init() { rootCmd.AddCommand(importCmd, exportCmd) }
