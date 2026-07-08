// cmd/godocstore/create_cmd.go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create [collection...]",
	Short: "Create an empty database, optionally with empty collections",
	Long: `Create the database file (and its schema) without writing any
document. With collection names, each is created empty too.

Examples:
  godocstore --db new.db create                # just the empty database
  godocstore --db new.db create users uploads  # plus two empty collections`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStoreCreate()
		if err != nil {
			return err
		}
		defer s.Close()

		for _, name := range args {
			if _, err := s.Collection(name); err != nil {
				return err
			}
			fmt.Printf("created collection %s\n", name)
		}
		fmt.Printf("database ready: %s\n", dbPath)
		return nil
	},
}

func init() { rootCmd.AddCommand(createCmd) }
