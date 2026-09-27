package audience

import (
	"encoding/csv"
	"io"
	"strings"
)

// safeCell neutralises spreadsheet formula injection (OWASP "CSV
// injection"), mirroring guest.SafeCell: a cell starting with = + - @ or a
// tab / carriage return is prefixed with an apostrophe so Excel, Numbers
// and Sheets show it as text instead of evaluating it.
func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// ExportHeader is the CSV header row for a contact export.
var ExportHeader = []string{"name", "email", "phone", "status", "source", "consent_basis", "consent_recorded_at"}

// WriteCSV writes the header and one row per contact, every cell
// injection-safe. It never includes consent_ip or consent_form_text: the
// export is for list management, not a compliance dump.
func WriteCSV(w io.Writer, contacts []Contact) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(ExportHeader); err != nil {
		return err
	}
	for _, c := range contacts {
		row := []string{
			safeCell(c.Name), safeCell(c.Email), safeCell(c.Phone),
			c.Status, c.Source, c.ConsentBasis, c.ConsentRecordedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
