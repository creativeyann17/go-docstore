// cmd/godocstore/ls_cmd.go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var lsDeleted bool

var lsCmd = &cobra.Command{
	Use:   "ls [collection]",
	Short: "List collections, or the document ids of one collection",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		if len(args) == 0 {
			names, err := s.Collections()
			if err != nil {
				return err
			}
			for _, name := range names {
				c, err := s.Collection(name)
				if err != nil {
					return err
				}
				n, err := c.Count()
				if err != nil {
					return err
				}
				fmt.Printf("%-24s %d\n", name, n)
			}
			return nil
		}

		c, err := s.Collection(args[0])
		if err != nil {
			return err
		}
		if lsDeleted {
			return c.EachDeleted(func(id string, _ []byte) error {
				fmt.Println(id)
				return nil
			})
		}
		ids, err := c.IDs()
		if err != nil {
			return err
		}
		for _, id := range ids {
			fmt.Println(id)
		}
		return nil
	},
}

func init() {
	lsCmd.Flags().BoolVar(&lsDeleted, "deleted", false, "list soft-deleted document ids instead")
	rootCmd.AddCommand(lsCmd)
}
