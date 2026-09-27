package guest

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"
)

// SafeCell neutralises spreadsheet formula injection (OWASP "CSV
// injection"): a cell that starts with = + - @ or a tab / carriage return
// is prefixed with an apostrophe so Excel, Numbers and Sheets show it as
// text instead of evaluating it. Quoting is left to encoding/csv.
func SafeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// ExportRow is one guest in the CSV export.
type ExportRow struct {
	Name       string
	PlusN      int
	Status     string
	List       string
	Allocation string
	Email      string
	Phone      string
	Note       string
}

// ExportHeader is the CSV header row.
var ExportHeader = []string{"name", "plus_n", "status", "list", "allocation", "email", "phone", "note"}

// WriteCSV writes the header and rows, every cell injection-safe.
func WriteCSV(w io.Writer, rows []ExportRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(ExportHeader); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{r.Name, strconv.Itoa(r.PlusN), r.Status, r.List, r.Allocation, r.Email, r.Phone, r.Note}
		for i := range rec {
			rec[i] = SafeCell(rec[i])
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
