package docstore

import (
	"fmt"
	"strings"
	"time"
)

// FindPath is a friendlier variant of Find for simple, index-free
// dotted paths (e.g. "login", "address.city", "loginHistory.at" — no
// leading "$." and no array indices; a leading "$." is tolerated and
// stripped for muscle-memory compatibility).
//
// The key difference from Find: if the FIRST path segment turns out to
// be an array, every element is searched — not just index 0. So
// "loginHistory.at" matches a document if ANY entry in its
// loginHistory array has that field equal to value, and a bare
// "tags" matches if value is a member of the tags array (array of
// scalars). A path whose first segment is not an array falls back to a
// plain field match, so "login" behaves exactly as expected.
//
// Only the first segment is treated as a potential array boundary —
// deeper/multiple array hops need Find with a literal SQLite
// json_extract path (e.g. "$.a[2].b[0].c") instead.
func (c *Collection) FindPath(path, op string, value any, limit int) (out []Doc, err error) {
	defer func(start time.Time) {
		c.logOp("findPath", fmt.Sprintf("%s %s %v", path, op, value), start, err)
	}(time.Now())

	sqlOp, ok := map[string]string{
		"=": "=", "!=": "!=", "<": "<", ">": ">", "like": "LIKE",
	}[strings.ToLower(op)]
	if !ok {
		return nil, fmt.Errorf("docstore: invalid op %q (want = != < > like)", op)
	}

	path = strings.TrimPrefix(strings.TrimPrefix(path, "$"), ".")
	if path == "" {
		return nil, fmt.Errorf("docstore: empty path")
	}
	first, rest, hasRest := strings.Cut(path, ".")
	firstJSON := "$." + first

	var q string
	var args []any
	if hasRest {
		scalarPath := "$." + first + "." + rest
		eachPath := "$." + rest
		q = fmt.Sprintf(`SELECT id, doc FROM %s WHERE (
			(json_type(doc, ?) = 'array' AND EXISTS (
				SELECT 1 FROM json_each(doc, ?) je WHERE json_extract(je.value, ?) %s ?
			))
			OR
			(json_type(doc, ?) IS NOT 'array' AND json_extract(doc, ?) %s ?)
		)%s ORDER BY id`, c.table, sqlOp, sqlOp, c.alive())
		args = []any{firstJSON, firstJSON, eachPath, value, firstJSON, scalarPath, value}
	} else {
		q = fmt.Sprintf(`SELECT id, doc FROM %s WHERE (
			(json_type(doc, ?) = 'array' AND EXISTS (
				SELECT 1 FROM json_each(doc, ?) je WHERE je.value %s ?
			))
			OR
			(json_type(doc, ?) IS NOT 'array' AND json_extract(doc, ?) %s ?)
		)%s ORDER BY id`, c.table, sqlOp, sqlOp, c.alive())
		args = []any{firstJSON, firstJSON, value, firstJSON, firstJSON, value}
	}
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := c.store.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d Doc
		var raw string
		if err := rows.Scan(&d.ID, &raw); err != nil {
			return nil, err
		}
		d.Raw = []byte(raw)
		out = append(out, d)
	}
	return out, rows.Err()
}
