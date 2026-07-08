// cmd/godocstore/edit_cmd.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit <collection> <id>",
	Short: "Edit a document in $EDITOR (validated, transactional)",
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

		raw, err := c.GetRaw(args[1])
		if err != nil {
			return err
		}

		tmp, err := os.CreateTemp("", "godocstore-*.json")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		tmp.WriteString(pretty(raw) + "\n")
		tmp.Close()

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		ed := exec.Command(editor, tmp.Name())
		ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := ed.Run(); err != nil {
			return fmt.Errorf("%s: %w", editor, err)
		}

		edited, err := os.ReadFile(tmp.Name())
		if err != nil {
			return err
		}
		if !json.Valid(edited) {
			return fmt.Errorf("edited content is not valid JSON — document unchanged")
		}
		// Transactional swap; concurrent writers (the app) can't be lost.
		if err := c.Update(args[1], func([]byte) ([]byte, error) { return edited, nil }); err != nil {
			return err
		}
		fmt.Printf("updated %s/%s\n", args[0], args[1])
		return nil
	},
}

func init() { rootCmd.AddCommand(editCmd) }
