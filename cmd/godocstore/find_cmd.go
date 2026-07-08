// cmd/godocstore/find_cmd.go
package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	findLimit  int
	findIDs    bool
	findString bool
)

var findCmd = &cobra.Command{
	Use:   "find <collection> <jsonpath> <op> <value>",
	Short: "Search documents by a JSON path (op: = != < > like)",
	Long: `Search documents by comparing a JSON path against a value.

The value is treated as a number when it parses as one (use --string to
force text — e.g. a login that happens to be numeric).

Examples:
  godocstore --db mist.db find users '$.login' = yann
  godocstore --db mist.db find users '$.usedBytes' '>' 1000000
  godocstore --db mist.db find users '$.email' like '%@example.com'`,
	Args: cobra.ExactArgs(4),
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

		var value any = args[3]
		if !findString {
			if n, err := strconv.ParseFloat(args[3], 64); err == nil {
				value = n
			}
		}

		docs, err := c.Find(args[1], args[2], value, findLimit)
		if err != nil {
			return err
		}
		for _, d := range docs {
			if findIDs {
				fmt.Println(d.ID)
			} else {
				fmt.Printf("── %s\n%s\n", d.ID, pretty(d.Raw))
			}
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "%d match(es)\n", len(docs))
		return nil
	},
}

func init() {
	findCmd.Flags().IntVar(&findLimit, "limit", 0, "max results (0 = all)")
	findCmd.Flags().BoolVar(&findIDs, "ids", false, "print ids only")
	findCmd.Flags().BoolVar(&findString, "string", false, "always compare as text (skip numeric coercion)")
	rootCmd.AddCommand(findCmd)
}
