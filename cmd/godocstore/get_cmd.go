// cmd/godocstore/get_cmd.go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var getMeta bool

var getCmd = &cobra.Command{
	Use:   "get <collection> <id>",
	Short: "Print one document as pretty JSON",
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

		if getMeta {
			m, err := c.Meta(args[1])
			if err != nil {
				return err
			}
			fmt.Printf("# created: %s  updated: %s", m.CreatedAt.Format("2006-01-02 15:04:05"), m.UpdatedAt.Format("2006-01-02 15:04:05"))
			if m.DeletedAt != nil {
				fmt.Printf("  DELETED: %s", m.DeletedAt.Format("2006-01-02 15:04:05"))
			}
			fmt.Println()
		}

		raw, err := c.GetRaw(args[1])
		if err != nil {
			return err
		}
		fmt.Println(pretty(raw))
		return nil
	},
}

// pretty re-indents raw JSON; falls back to the input on parse failure.
func pretty(raw []byte) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func init() {
	getCmd.Flags().BoolVar(&getMeta, "meta", false, "print created/updated/deleted timestamps header")
	rootCmd.AddCommand(getCmd)
}
