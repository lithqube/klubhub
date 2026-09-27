package report

import (
	"encoding/csv"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/promoter/guest"
)

// ListBackRow is one guest in the list-back CSV. No email or phone.
type ListBackRow struct {
	Name          string
	PlusN         int
	Status        string
	HeadsAdmitted int
	FirstIn       *time.Time
}

// ListBackHeader is the list-back CSV header row.
var ListBackHeader = []string{"name", "plus_n", "status", "arrived", "heads_admitted", "first_in_local"}

// localStamp is the first_in_local format (event timezone).
const localStamp = "2006-01-02 15:04"

// WriteListBack writes the header and rows, every cell injection-safe
// (guest.SafeCell, as in the guest export). first_in_local is the first
// live `in` in loc.
func WriteListBack(w io.Writer, rows []ListBackRow, loc *time.Location) error {
	if loc == nil {
		loc = time.UTC
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(ListBackHeader); err != nil {
		return err
	}
	for _, r := range rows {
		arrived, first := "no", ""
		if r.HeadsAdmitted >= 1 {
			arrived = "yes"
		}
		if r.FirstIn != nil {
			first = r.FirstIn.In(loc).Format(localStamp)
		}
		rec := []string{r.Name, strconv.Itoa(r.PlusN), r.Status, arrived, strconv.Itoa(r.HeadsAdmitted), first}
		for i := range rec {
			rec[i] = guest.SafeCell(rec[i])
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

var nonFileChars = regexp.MustCompile(`[^a-z0-9]+`)

// filename is "<event-slug>-list-back-<label>.csv" with the label folded
// to [a-z0-9-] (it is a public billing name, but still goes into a header),
// or the allocation id's first block when nothing is left.
func filename(slug, label string, id uuid.UUID) string {
	l := strings.Trim(nonFileChars.ReplaceAllString(envelope.NormalizeName(label), "-"), "-")
	if len(l) > 40 {
		l = strings.Trim(l[:40], "-")
	}
	if l == "" {
		l = id.String()[:8]
	}
	return slug + "-list-back-" + l + ".csv"
}
