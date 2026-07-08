// cmd/godocstore/main.go
package main

import (
	"fmt"
	"os"

	"github.com/creativeyann17/go-docstore"
	"github.com/spf13/cobra"
)

// Overridden at build time via -ldflags (see Makefile).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var dbPath string

var rootCmd = &cobra.Command{
	Use:     "godocstore",
	Short:   "Browse, edit and search go-docstore SQLite databases",
	Long:    "godocstore is the CLI companion to the go-docstore library:\ncollections of JSON documents in a single SQLite file.",
	Version: version,
	// Subcommands open the store themselves; nothing to do here.
	SilenceUsage: true,
}

func main() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "path to the SQLite database file (required)")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// openStore opens --db or fails with a friendly message.
func openStore() (*docstore.Store, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("--db is required (path to the SQLite file)")
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", dbPath, err)
	}
	return docstore.Open(dbPath)
}

// openStoreCreate is like openStore but allows creating a new database
// (used by write commands like put/import).
func openStoreCreate() (*docstore.Store, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("--db is required (path to the SQLite file)")
	}
	return docstore.Open(dbPath)
}
