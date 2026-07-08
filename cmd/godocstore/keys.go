// cmd/godocstore/keys.go
package main

import "sort"

// keyPathLimit / keySampleLimit / keyDepthLimit bound the cost of
// autocomplete extraction: it's a UX nicety, never allowed to make
// /api/keys slow or its response huge on a pathological document.
const (
	keySampleLimit = 20  // documents inspected
	keyPathLimit   = 300 // distinct paths returned
	keyDepthLimit  = 6   // object/array nesting followed
)

// extractPaths walks a decoded JSON value and collects every path seen,
// as plain dotted segments (a, a.b, arr.b — no "$." prefix, no "[0]"
// index) matching what Collection.FindPath expects: a suggestion can
// be used verbatim, and it reads as "any entry" for array fields
// rather than implying index 0 specifically. Arrays are sampled at
// element 0 to discover the shape, but the index itself is never
// encoded in the path.
func extractPaths(prefix string, v any, depth int, set map[string]struct{}) {
	if depth > keyDepthLimit || len(set) >= keyPathLimit {
		return
	}
	switch val := v.(type) {
	case map[string]any:
		for k, sub := range val {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			set[p] = struct{}{}
			extractPaths(p, sub, depth+1, set)
		}
	case []any:
		if len(val) > 0 {
			extractPaths(prefix, val[0], depth+1, set)
		}
	}
}

// sortedKeys returns set's members sorted, truncated to keyPathLimit.
func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) > keyPathLimit {
		out = out[:keyPathLimit]
	}
	return out
}
