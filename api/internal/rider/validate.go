package rider

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Input limits. Generous for a real rider (20,000 characters is several
// pages of A4) but bounded, so one paste cannot create a multi-megabyte row
// or a thousand-page PDF. The same limits are enforced as CHECK constraints
// in the migration; this layer returns a readable 422 before the database
// would reject the write.
const (
	// MaxNameChars is the longest template name, in characters (not bytes).
	MaxNameChars = 200
	// MaxSectionChars is the longest single rider section, in characters.
	MaxSectionChars = 20000
	// MaxRequestBytes caps a JSON request body: four maximum-size sections
	// of 4-byte characters (~320 KB) plus headroom.
	MaxRequestBytes = 512 << 10
)

// checkText rejects text Postgres cannot store or that exceeds max
// characters. field names the input in the error message.
func checkText(field, s string, max int) error {
	if strings.ContainsRune(s, 0) {
		// Postgres TEXT cannot hold NUL; unchecked it surfaced as a 500.
		return fmt.Errorf("%w: %s contains a NUL character", ErrInvalidInput, field)
	}
	if n := utf8.RuneCountInString(s); n > max {
		return fmt.Errorf("%w: %s is too long (%d characters; the limit is %d)", ErrInvalidInput, field, n, max)
	}
	return nil
}

func checkSections(v RiderSectionValues) error {
	for _, f := range []struct{ name, val string }{
		{"technical", v.Technical}, {"hospitality", v.Hospitality},
		{"backline", v.Backline}, {"otherNotes", v.OtherNotes},
	} {
		if err := checkText(f.name, f.val, MaxSectionChars); err != nil {
			return err
		}
	}
	return nil
}

// checkPatch validates only the sections a partial update actually sets.
func checkPatch(p RiderSectionPatch) error {
	for _, f := range []struct {
		name string
		val  *string
	}{
		{"technical", p.Technical}, {"hospitality", p.Hospitality},
		{"backline", p.Backline}, {"otherNotes", p.OtherNotes},
	} {
		if f.val == nil {
			continue
		}
		if err := checkText(f.name, *f.val, MaxSectionChars); err != nil {
			return err
		}
	}
	return nil
}
