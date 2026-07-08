package docstore

import "encoding/json"

// Kept in one place so the codec can be swapped (e.g. for a faster JSON
// library) without touching store logic.
func jsonMarshal(v any) ([]byte, error)     { return json.Marshal(v) }
func jsonUnmarshal(b []byte, out any) error { return json.Unmarshal(b, out) }
