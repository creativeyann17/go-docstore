// cmd/godocstore/put_cmd.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var putCmd = &cobra.Command{
	Use:   "put <collection> <id> [file]",
	Short: "Create or replace a document from a JSON file (or stdin)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		var raw []byte
		var err error
		if len(args) == 3 {
			raw, err = os.ReadFile(args[2])
		} else {
			raw, err = io.ReadAll(os.Stdin)
		}
		if err != nil {
			return err
		}
		if !json.Valid(raw) {
			return fmt.Errorf("input is not valid JSON")
		}

		s, err := openStoreCreate()
		if err != nil {
			return err
		}
		defer s.Close()
		c, err := s.Collection(args[0])
		if err != nil {
			return err
		}
		if err := c.Put(args[1], raw); err != nil {
			return err
		}
		fmt.Printf("put %s/%s\n", args[0], args[1])
		return nil
	},
}

func init() { rootCmd.AddCommand(putCmd) }
