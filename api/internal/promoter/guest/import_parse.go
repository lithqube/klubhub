package guest

// Attendee import (P2.2): pure CSV parsing, column mapping presets, row
// normalisation and masking. No database here; import_service.go applies
// the mapped rows. apps/promoter/app/utils/attendeeImport.ts mirrors this
// file for the dev mocks and the mapping UI; the Go side is the source of
// truth for real imports.

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Import presets: the platform an export comes from. "generic" takes an
// explicit column mapping from the client.
const (
	PresetRA      = "ra"
	PresetDICE    = "dice"
	PresetShotgun = "shotgun"
	PresetPretix  = "pretix"
	PresetLuma    = "luma"
	PresetGeneric = "generic"
)

// Presets in UI order.
var Presets = []string{PresetRA, PresetDICE, PresetShotgun, PresetPretix, PresetLuma, PresetGeneric}

// Canonical import fields a column can map to.
const (
	FieldOrderRef      = "order_ref"
	FieldTicketRef     = "ticket_ref"
	FieldSecret        = "secret"
	FieldName          = "name"
	FieldFirstName     = "first_name"
	FieldLastName      = "last_name"
	FieldEmail         = "email"
	FieldBuyerName     = "buyer_name"
	FieldBuyerEmail    = "buyer_email"
	FieldTicketType    = "ticket_type"
	FieldTicketTypeRef = "ticket_type_ref"
	FieldStatus        = "status"
)

// ImportFields in mapping-UI order.
var ImportFields = []string{
	FieldOrderRef, FieldTicketRef, FieldSecret, FieldName, FieldFirstName, FieldLastName, FieldEmail,
	FieldBuyerName, FieldBuyerEmail, FieldTicketType, FieldTicketTypeRef, FieldStatus,
}

// Ticket (order position) statuses.
const (
	TicketValid     = "valid"
	TicketPending   = "pending"
	TicketCancelled = "cancelled"
	TicketRefunded  = "refunded"
)

// Import limits. MaxImportBytes matches nuxt-security's default upload
// cap, so the proxied and the direct path refuse the same files.
const (
	MaxImportBytes       = 8 << 20
	MaxImportRows        = 20_000
	MaxTicketsPerEvent   = 20_000
	maxRefLen            = 120
	maxSecretLen         = 512
	maxTicketTypeLen     = 120
	DefaultTicketType    = "General admission"
	maxRejectedListed    = 200
	previewRows          = 10
	ticketRefOrdinalMark = "#"
)

// headerKey folds a column header for tolerant matching: case, spacing,
// punctuation and a leading BOM are ignored ("Order code" == "order_code").
func headerKey(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(h) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Column aliases per preset (already folded with headerKey), most specific
// first. The headers are inferred from each platform's documented or
// commonly seen exports; exports change, so every non-generic preset also
// falls back to commonAliases and the dry run shows what was matched.
var presetAliases = map[string]map[string][]string{
	PresetPretix: {
		FieldOrderRef:      {"ordercode", "order"},
		FieldTicketRef:     {"positionid", "orderpositionid"},
		FieldSecret:        {"ticketsecret", "secret"},
		FieldName:          {"attendeename"},
		FieldFirstName:     {"attendeenamegivenname", "attendeenamefirstname"},
		FieldLastName:      {"attendeenamefamilyname", "attendeenamelastname"},
		FieldEmail:         {"attendeeemail"},
		FieldBuyerName:     {"invoiceaddressname"},
		FieldBuyerEmail:    {"email", "orderemail"},
		FieldTicketType:    {"product", "item"},
		FieldTicketTypeRef: {"productid", "itemid"},
		FieldStatus:        {"status", "orderstatus"},
	},
	PresetRA: {
		FieldOrderRef:   {"orderid", "ordernumber", "orderref", "bookingreference", "bookingref"},
		FieldTicketRef:  {"ticketid", "ticketnumber"},
		FieldSecret:     {"barcode", "ticketbarcode", "ticketcode"},
		FieldName:       {"name", "fullname", "ticketholder", "ticketholdername"},
		FieldFirstName:  {"firstname", "forename"},
		FieldLastName:   {"lastname", "surname"},
		FieldEmail:      {"email", "emailaddress"},
		FieldTicketType: {"tickettype", "ticket", "tickettier", "tier"},
		FieldStatus:     {"status", "ticketstatus"},
	},
	PresetDICE: {
		FieldOrderRef:   {"orderid", "purchaseid", "orderreference", "order"},
		FieldTicketRef:  {"ticketid"},
		FieldSecret:     {"barcode", "ticketcode", "code"},
		FieldName:       {"fanname", "name", "fullname"},
		FieldFirstName:  {"firstname", "fanfirstname"},
		FieldLastName:   {"lastname", "fanlastname"},
		FieldEmail:      {"email", "fanemail"},
		FieldTicketType: {"tickettype", "ticketname", "ticket"},
		FieldStatus:     {"status", "ticketstatus"},
	},
	PresetShotgun: {
		FieldOrderRef:   {"orderid", "ordernumber", "order"},
		FieldTicketRef:  {"ticketid"},
		FieldSecret:     {"barcode", "ticketbarcode", "qrcode"},
		FieldName:       {"name", "fullname", "holdername"},
		FieldFirstName:  {"firstname", "holderfirstname"},
		FieldLastName:   {"lastname", "holderlastname"},
		FieldEmail:      {"email", "holderemail", "contactemail"},
		FieldBuyerEmail: {"buyeremail"},
		FieldTicketType: {"ticket", "tickettitle", "tickettype", "dealtitle", "ticketname"},
		FieldStatus:     {"ticketstatus", "status", "state"},
	},
	PresetLuma: {
		FieldOrderRef:      {"apiid", "guestapiid"},
		FieldSecret:        {"qrcodeurl", "qrcode"},
		FieldName:          {"name"},
		FieldFirstName:     {"firstname"},
		FieldLastName:      {"lastname"},
		FieldEmail:         {"email"},
		FieldTicketType:    {"ticketname", "tickettypename", "tickettype"},
		FieldTicketTypeRef: {"tickettypeid"},
		FieldStatus:        {"approvalstatus", "status"},
	},
}

var commonAliases = map[string][]string{
	FieldOrderRef:      {"orderid", "ordercode", "ordernumber", "orderref", "orderreference", "order", "bookingreference", "purchaseid", "transactionid"},
	FieldTicketRef:     {"ticketid", "ticketnumber", "positionid"},
	FieldSecret:        {"barcode", "ticketbarcode", "secret", "ticketsecret", "qrcode", "ticketcode"},
	FieldName:          {"attendeename", "name", "fullname", "ticketholder", "holdername", "guestname"},
	FieldFirstName:     {"firstname", "givenname", "forename"},
	FieldLastName:      {"lastname", "familyname", "surname"},
	FieldEmail:         {"attendeeemail", "email", "emailaddress"},
	FieldBuyerName:     {"buyername", "purchasername", "customername"},
	FieldBuyerEmail:    {"buyeremail", "purchaseremail", "customeremail"},
	FieldTicketType:    {"tickettype", "ticketname", "ticket", "product", "tier"},
	FieldTicketTypeRef: {"tickettypeid", "productid"},
	FieldStatus:        {"status", "ticketstatus", "orderstatus"},
}

// Table is a parsed CSV: headers and data rows with their source line.
type Table struct {
	Headers []string
	Rows    []TableRow
	// Encoding is "utf-8" or "windows-1252" (legacy Excel exports).
	Encoding string
}

// TableRow is one data row; Line is 1-based in the file (or the JSON array).
type TableRow struct {
	Line  int
	Cells []string
}

// TableFromObjects builds a table from JSON rows (header → value). Columns
// are sorted so the result does not depend on map order.
func TableFromObjects(rows []map[string]string) (*Table, error) {
	if len(rows) == 0 {
		return nil, invalid("rows", "no rows")
	}
	if len(rows) > MaxImportRows {
		return nil, invalid("rows", fmt.Sprintf("at most %d rows per import", MaxImportRows))
	}
	seen := map[string]bool{}
	var headers []string
	for _, r := range rows {
		for k := range r {
			if !seen[k] {
				seen[k] = true
				headers = append(headers, k)
			}
		}
	}
	sort.Strings(headers)
	t := &Table{Headers: headers, Encoding: "utf-8"}
	for i, r := range rows {
		cells := make([]string, len(headers))
		for j, h := range headers {
			cells[j] = r[h]
		}
		t.Rows = append(t.Rows, TableRow{Line: i + 1, Cells: cells})
	}
	return t, nil
}

// ParseCSV reads an export: UTF-8 (a BOM is dropped) or, failing that,
// Windows-1252; `,`, `;` or tab delimited (sniffed from the header line,
// or an Excel "sep=;" first line); quoted fields may span lines. Blank
// rows are skipped.
func ParseCSV(raw []byte) (*Table, error) {
	if len(raw) > MaxImportBytes {
		return nil, invalid("file", fmt.Sprintf("larger than %d MB", MaxImportBytes>>20))
	}
	if bytes.HasPrefix(raw, []byte{0xFF, 0xFE}) || bytes.HasPrefix(raw, []byte{0xFE, 0xFF}) {
		return nil, invalid("file", "UTF-16 text; export or save as CSV (UTF-8)")
	}
	enc := "utf-8"
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(raw) {
		dec, err := charmap.Windows1252.NewDecoder().Bytes(raw)
		if err != nil {
			return nil, invalid("file", "not a text file")
		}
		raw, enc = dec, "windows-1252"
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		return nil, invalid("file", "not a CSV file")
	}
	text := string(raw)
	line := 1
	delim := rune(0)
	if first, rest, ok := strings.Cut(text, "\n"); ok || first != "" {
		f := strings.TrimSpace(first)
		if strings.HasPrefix(strings.ToLower(f), "sep=") && utf8.RuneCountInString(f) == 5 {
			delim, _ = utf8.DecodeRuneInString(f[4:])
			text, line = rest, 2
		}
	}
	if delim == 0 {
		delim = sniffDelimiter(text)
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.ReuseRecord = false
	t := &Table{Encoding: enc}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, invalid("file", fmt.Sprintf("line %d: %v", pe.StartLine+line-1, pe.Err))
			}
			return nil, invalid("file", "could not read the CSV")
		}
		start, _ := r.FieldPos(0)
		if blank(rec) {
			continue
		}
		if t.Headers == nil {
			t.Headers = trimAll(rec)
			continue
		}
		if len(t.Rows) >= MaxImportRows {
			return nil, invalid("file", fmt.Sprintf("at most %d rows per import; split the file", MaxImportRows))
		}
		t.Rows = append(t.Rows, TableRow{Line: start + line - 1, Cells: rec})
	}
	if t.Headers == nil {
		return nil, invalid("file", "the file is empty")
	}
	if len(t.Rows) == 0 {
		return nil, invalid("file", "no rows below the header line")
	}
	return t, nil
}

// sniffDelimiter picks the most frequent of , ; and tab on the first
// non-empty line, outside quotes. Ties go to the comma.
func sniffDelimiter(text string) rune {
	head := text
	for head != "" {
		l, rest, _ := strings.Cut(head, "\n")
		if strings.TrimSpace(l) != "" {
			head = l
			break
		}
		head = rest
	}
	counts := map[rune]int{}
	quoted := false
	for _, c := range head {
		switch {
		case c == '"':
			quoted = !quoted
		case !quoted && (c == ',' || c == ';' || c == '\t'):
			counts[c]++
		}
	}
	best := ','
	for _, c := range []rune{';', '\t'} {
		if counts[c] > counts[best] {
			best = c
		}
	}
	return best
}

func blank(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func trimAll(rec []string) []string {
	out := make([]string, len(rec))
	for i, c := range rec {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

// MappingError: the export has no column for a required field. Headers are
// returned so the client can offer an explicit (generic) mapping.
type MappingError struct {
	Missing []string `json:"missing"`
	Headers []string `json:"headers"`
	Problem string   `json:"problem"`
}

func (e *MappingError) Error() string { return "guest: import mapping incomplete: " + e.Problem }

// Mapping is the resolved field → column index (and header text).
type Mapping struct {
	Index  map[string]int
	Header map[string]string
}

// ResolveMapping matches a preset's aliases against the headers, or checks
// an explicit field → header mapping for "generic". Each column maps to at
// most one field. It fails when there is no name/email column or no key
// (order number, ticket id or barcode) for idempotent re-imports.
func ResolveMapping(preset string, headers []string, explicit map[string]string) (Mapping, error) {
	m := Mapping{Index: map[string]int{}, Header: map[string]string{}}
	if !slices.Contains(Presets, preset) {
		return m, invalid("preset", strings.Join(Presets, ", "))
	}
	keys := make([]string, len(headers))
	for i, h := range headers {
		keys[i] = headerKey(h)
	}
	used := map[int]bool{}
	take := func(field string, idx int) {
		m.Index[field], m.Header[field] = idx, headers[idx]
		used[idx] = true
	}
	if preset == PresetGeneric {
		for field, h := range explicit {
			if h == "" {
				continue
			}
			if !slices.Contains(ImportFields, field) {
				return m, invalid("mapping", "unknown field "+field)
			}
			idx := slices.Index(keys, headerKey(h))
			if idx < 0 {
				return m, &MappingError{Missing: []string{field}, Headers: headers, Problem: fmt.Sprintf("no column %q in the file", h)}
			}
			if used[idx] {
				return m, invalid("mapping", fmt.Sprintf("column %q is mapped twice", h))
			}
			take(field, idx)
		}
	} else {
		for _, field := range ImportFields {
			for _, alias := range append(slices.Clone(presetAliases[preset][field]), commonAliases[field]...) {
				if idx := slices.Index(keys, alias); idx >= 0 && !used[idx] {
					take(field, idx)
					break
				}
			}
		}
	}
	var missing []string
	if !m.has(FieldName, FieldFirstName, FieldLastName, FieldEmail, FieldBuyerName, FieldBuyerEmail) {
		missing = append(missing, "name or email")
	}
	if !m.has(FieldOrderRef, FieldTicketRef, FieldSecret) {
		missing = append(missing, "order number, ticket id or barcode")
	}
	if len(missing) > 0 {
		return m, &MappingError{Missing: missing, Headers: headers, Problem: "no column for " + strings.Join(missing, " and ")}
	}
	return m, nil
}

func (m Mapping) has(fields ...string) bool {
	for _, f := range fields {
		if _, ok := m.Index[f]; ok {
			return true
		}
	}
	return false
}

func (m Mapping) cell(row TableRow, field string) string {
	i, ok := m.Index[field]
	if !ok || i >= len(row.Cells) {
		return ""
	}
	return strings.Join(strings.Fields(row.Cells[i]), " ")
}

// ImportRow is one export row after mapping and normalisation.
type ImportRow struct {
	Line          int
	OrderRef      string
	TicketRef     string
	Secret        string
	Name          string
	Email         string
	BuyerName     string
	BuyerEmail    string
	TicketType    string
	TicketTypeRef string
	Status        string
}

// MapRow applies the mapping to one row. It returns a reason when the row
// cannot be imported (the dry run lists it by line).
func (m Mapping) MapRow(row TableRow) (ImportRow, string) {
	r := ImportRow{
		Line:          row.Line,
		OrderRef:      m.cell(row, FieldOrderRef),
		TicketRef:     m.cell(row, FieldTicketRef),
		Secret:        strings.TrimSpace(cellRaw(m, row, FieldSecret)),
		Name:          m.cell(row, FieldName),
		Email:         m.cell(row, FieldEmail),
		BuyerName:     m.cell(row, FieldBuyerName),
		BuyerEmail:    m.cell(row, FieldBuyerEmail),
		TicketType:    m.cell(row, FieldTicketType),
		TicketTypeRef: m.cell(row, FieldTicketTypeRef),
	}
	if r.Name == "" {
		r.Name = strings.TrimSpace(m.cell(row, FieldFirstName) + " " + m.cell(row, FieldLastName))
	}
	if r.Email == "" {
		r.Email = r.BuyerEmail
	}
	if r.BuyerEmail == "" {
		r.BuyerEmail = r.Email
	}
	if r.Name == "" {
		r.Name = r.BuyerName
	}
	if r.BuyerName == "" {
		r.BuyerName = r.Name
	}
	for _, e := range []string{r.Email, r.BuyerEmail} {
		if e != "" && !validEmail(e) {
			return r, fmt.Sprintf("%q is not an email address", MaskEmail(e))
		}
	}
	if r.Name == "" {
		r.Name = r.Email
	}
	status, ok := TicketStatus(m.cell(row, FieldStatus))
	if !ok {
		return r, fmt.Sprintf("unknown status %q", m.cell(row, FieldStatus))
	}
	r.Status = status
	if r.TicketType == "" {
		r.TicketType = DefaultTicketType
	}
	switch {
	case r.Name == "":
		return r, "no name or email"
	case r.OrderRef == "" && r.TicketRef == "" && r.Secret == "":
		return r, "no order number, ticket id or barcode"
	case len(r.Name) > maxNameLen || len(r.BuyerName) > maxNameLen:
		return r, fmt.Sprintf("name longer than %d characters", maxNameLen)
	case len(r.Email) > maxEmailLen || len(r.BuyerEmail) > maxEmailLen:
		return r, "email too long"
	case len(r.OrderRef) > maxRefLen || len(r.TicketRef) > maxRefLen || len(r.TicketTypeRef) > maxRefLen:
		return r, fmt.Sprintf("order or ticket id longer than %d characters", maxRefLen)
	case strings.HasPrefix(r.TicketRef, ticketRefOrdinalMark):
		return r, "ticket id must not start with #"
	case len(r.Secret) > maxSecretLen:
		return r, fmt.Sprintf("barcode longer than %d characters", maxSecretLen)
	case len(r.TicketType) > maxTicketTypeLen:
		return r, fmt.Sprintf("ticket type longer than %d characters", maxTicketTypeLen)
	}
	return r, ""
}

// The ticket secret is matched exactly (barcodes are case-sensitive), so it
// is only trimmed, not folded.
func cellRaw(m Mapping, row TableRow, field string) string {
	i, ok := m.Index[field]
	if !ok || i >= len(row.Cells) {
		return ""
	}
	return row.Cells[i]
}

func validEmail(e string) bool {
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e && strings.Contains(e[strings.LastIndex(e, "@"):], ".")
}

var ticketStatuses = map[string]string{
	"": TicketValid, "valid": TicketValid, "paid": TicketValid, "p": TicketValid, "completed": TicketValid, "complete": TicketValid,
	"confirmed": TicketValid, "approved": TicketValid, "active": TicketValid, "issued": TicketValid, "sold": TicketValid,
	"purchased": TicketValid, "going": TicketValid, "ok": TicketValid, "checkedin": TicketValid, "attended": TicketValid,
	"scanned": TicketValid, "used": TicketValid, "free": TicketValid, "registered": TicketValid,
	"pending": TicketPending, "n": TicketPending, "unpaid": TicketPending, "reserved": TicketPending,
	"pendingapproval": TicketPending, "waitlist": TicketPending, "invited": TicketPending, "awaitingpayment": TicketPending,
	"requested": TicketPending,
	"refunded":  TicketRefunded, "r": TicketRefunded, "refund": TicketRefunded, "chargeback": TicketRefunded,
	"cancelled": TicketCancelled, "canceled": TicketCancelled, "c": TicketCancelled, "e": TicketCancelled,
	"expired": TicketCancelled, "void": TicketCancelled, "voided": TicketCancelled, "declined": TicketCancelled,
	"rejected": TicketCancelled, "revoked": TicketCancelled, "transferred": TicketCancelled, "deleted": TicketCancelled,
	"invalid": TicketCancelled, "notgoing": TicketCancelled, "notattending": TicketCancelled,
}

// TicketStatus maps a platform's status text to a ticket status. Empty
// means valid (many exports list only sold tickets). Unknown text is
// refused rather than guessed, so a cancelled ticket never turns valid.
func TicketStatus(raw string) (string, bool) {
	k := headerKey(raw)
	if s, ok := ticketStatuses[k]; ok {
		return s, true
	}
	switch {
	case strings.Contains(k, "refund"):
		return TicketRefunded, true
	case strings.Contains(k, "cancel"):
		return TicketCancelled, true
	}
	return "", false
}

// MaskName shortens a name for the dry-run preview: "John Doe" → "Jo… D…".
func MaskName(name string) string {
	words := strings.Fields(name)
	for i, w := range words {
		keep := 1
		if i == 0 {
			keep = 2
		}
		if utf8.RuneCountInString(w) > keep {
			words[i] = string([]rune(w)[:keep]) + "…"
		}
	}
	return strings.Join(words, " ")
}

// MaskEmail shortens an email for the preview: "mara@label.example" → "m…@l…".
func MaskEmail(email string) string {
	if email == "" {
		return ""
	}
	local, domain, ok := strings.Cut(email, "@")
	first := func(s string) string {
		if s == "" {
			return ""
		}
		return string([]rune(s)[:1]) + "…"
	}
	if !ok {
		return first(local)
	}
	return first(local) + "@" + first(domain)
}
