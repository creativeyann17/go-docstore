// cmd/godocstore/rm_cmd.go
package main

import (
	"fmt"

	"github.com/creativeyann17/go-docstore"
	"github.com/spf13/cobra"
)

var rmSoft bool

var rmCmd = &cobra.Command{
	Use:   "rm <collection> <id>",
	Short: "Delete a document (hard by default, --soft to mark instead)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()
		var opts []docstore.Option
		if rmSoft {
			opts = append(opts, docstore.WithSoftDelete())
		}
		c, err := s.Collection(args[0], opts...)
		if err != nil {
			return err
		}
		if err := c.Delete(args[1]); err != nil {
			return err
		}
		if rmSoft {
			fmt.Printf("soft-deleted %s/%s (restore with `restore`, remove with `purge`)\n", args[0], args[1])
		} else {
			fmt.Printf("deleted %s/%s\n", args[0], args[1])
		}
		return nil
	},
}

var restoreCmd = &cobra.Command{
	Use:   "restore <collection> <id>",
	Short: "Clear a document's soft-delete mark",
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
		if err := c.Restore(args[1]); err != nil {
			return err
		}
		fmt.Printf("restored %s/%s\n", args[0], args[1])
		return nil
	},
}

var purgeCmd = &cobra.Command{
	Use:   "purge <collection> <id>",
	Short: "Remove a document for real, regardless of soft-delete state",
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
		if err := c.Purge(args[1]); err != nil {
			return err
		}
		fmt.Printf("purged %s/%s\n", args[0], args[1])
		return nil
	},
}

func init() {
	rmCmd.Flags().BoolVar(&rmSoft, "soft", false, "mark deleted_at instead of removing the row")
	rootCmd.AddCommand(rmCmd, restoreCmd, purgeCmd)
}
