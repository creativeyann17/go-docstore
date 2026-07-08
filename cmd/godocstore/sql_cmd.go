// cmd/godocstore/sql_cmd.go
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var sqlCmd = &cobra.Command{
	Use:   "sql <query>",
	Short: "Run raw SQL against the database (escape hatch)",
	Long: `Run raw SQL. SELECT rows print as JSON lines; other statements
print the number of affected rows. Tables are named c_<collection>.

Example:
  godocstore --db mist.db sql "SELECT id, json_extract(doc,'$.login') AS login FROM c_users"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()
		q := strings.TrimSpace(args[0])

		if !strings.HasPrefix(strings.ToUpper(q), "SELECT") {
			res, err := s.DB().Exec(q)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			fmt.Printf("%d row(s) affected\n", n)
			return nil
		}

		rows, err := s.DB().Query(q)
		if err != nil {
			return err
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			return err
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		count := 0
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
			m := map[string]any{}
			for i, col := range cols {
				if b, ok := vals[i].([]byte); ok {
					m[col] = string(b)
				} else {
					m[col] = vals[i]
				}
			}
			line, _ := json.Marshal(m)
			fmt.Println(string(line))
			count++
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "%d row(s)\n", count)
		return rows.Err()
	},
}

func init() { rootCmd.AddCommand(sqlCmd) }
