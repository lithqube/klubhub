package audience

import "encoding/json"

// filterJSON and parseFilterJSON round-trip SegmentFilter through the
// audience_segments.filter jsonb column. A segment's filter is never
// personal data (§Data model and security) — it is a saved query
// definition — so no encryption applies here, unlike every other field
// touched in this package.
func filterJSON(f SegmentFilter) []byte {
	b, _ := json.Marshal(f) // SegmentFilter has no fields that can fail to marshal
	return b
}

func parseFilterJSON(raw []byte, f *SegmentFilter) error {
	if len(raw) == 0 {
		*f = SegmentFilter{}
		return nil
	}
	return json.Unmarshal(raw, f)
}
