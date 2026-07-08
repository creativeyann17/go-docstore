// cmd/godocstore/serve_cmd.go
package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

var serveAddr string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve a JSON-first debug UI for the database",
	Long: `Serve a small web UI to browse, edit and query the collections —
built for documents (the JSON is the centerpiece), unlike generic
SQL browsers that truncate wide columns.

Binds to loopback by default: the UI has NO auth and edits live data.
Only expose it further behind your own reverse-proxy auth.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStoreCreate()
		if err != nil {
			return err
		}
		defer s.Close()

		if !strings.HasPrefix(serveAddr, "127.0.0.1:") && !strings.HasPrefix(serveAddr, "localhost:") {
			fmt.Println("[warn] binding beyond loopback — this UI has no auth and edits live data")
		}
		fmt.Printf("godocstore UI: http://%s  (db: %s)\n", serveAddr, dbPath)
		return http.ListenAndServe(serveAddr, newServeMux(s))
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", "127.0.0.1:8391", "listen address (loopback by default on purpose)")
	rootCmd.AddCommand(serveCmd)
}
