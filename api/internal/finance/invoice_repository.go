package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// InvoiceRepository persists Invoice and InvoiceLine rows.
type InvoiceRepository struct {
	pool *pgxpool.Pool
}

// NewInvoiceRepository returns an InvoiceRepository backed by pool.
func NewInvoiceRepository(pool *pgxpool.Pool) *InvoiceRepository {
	return &InvoiceRepository{pool: pool}
}

// Pool exposes the underlying pool for tests.
func (r *InvoiceRepository) Pool() *pgxpool.Pool { return r.pool }

// CreateDraft inserts a draft invoice with lines derived from the gig's
// fee. Returns the created invoice with its assigned number (sequence
// advanced atomically). The caller must hold no DB locks; this function
// runs its own transaction.
func (r *InvoiceRepository) CreateDraft(ctx context.Context, gig *GigFeeInfo, req CreateInvoiceRequest, profile *BillingProfile) (*Invoice, error) {
	// Validate currency is 3 uppercase letters.
	if len(req.Currency) != 3 {
		return nil, fmt.Errorf("%w: currency must be 3 letters", ErrInvoiceValidation)
	}
	for _, ch := range req.Currency {
		if ch < 'A' || ch > 'Z' {
			return nil, fmt.Errorf("%w: currency must be uppercase A-Z", ErrInvoiceValidation)
		}
	}

	prefix := req.NumberPrefix
	if prefix == "" {
		prefix = DefaultNumberingConfig().Prefix
	}

	prefix = strings.ToUpper(prefix)

	// Build the invoice row. We allocate the sequence number inside a
	// transaction so we can also write the lines atomically.
	inv := &Invoice{}
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		number, seq, err := allocateInvoiceNumber(ctx, tx, prefix, req.Currency)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO invoices (gig_id, billing_profile, invoice_number, number_prefix, number_seq,
			                      currency, subtotal_minor, tax_rate_bps, tax_minor, total_minor,
			                      status, due_at, updated_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'draft', $11, now(), now())
			RETURNING `+invoiceColumns,
			gig.ID, profileJSON(profile), number, prefix, seq, req.Currency,
			gig.FeeMinor, gig.TaxRateBps, gig.TaxMinor, gig.TotalMinor, req.DueAt).Scan(inv.scanTargets()...)
		if err != nil {
			return fmt.Errorf("insert invoice: %w", err)
		}

		// Insert line(s). For now: one line for the gig fee.
		_, err = tx.Exec(ctx, `
			INSERT INTO invoice_lines (invoice_id, sort_order, description, quantity, unit_minor, tax_bps)
			VALUES ($1, 0, $2, 1, $3, $4)`, inv.ID, gig.LineDescription, gig.FeeMinor, gig.TaxRateBps)
		if err != nil {
			return fmt.Errorf("insert invoice line: %w", err)
		}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrInvoiceConflict
		}
		return nil, err
	}
	return inv, nil
}

// invoiceColumns is the RETURNING/SELECT list matching Invoice.scanTargets.
const invoiceColumns = `id, gig_id, billing_profile, invoice_number, number_prefix, number_seq,
	currency, subtotal_minor, tax_rate_bps, tax_minor, total_minor,
	status, issued_at, due_at, paid_at, payment_ref, internal_notes,
	updated_at, created_at`

// scanTargets returns pointers to inv's fields in invoiceColumns order.
func (inv *Invoice) scanTargets() []any {
	return []any{
		&inv.ID, &inv.GigID, &inv.BillingProfile, &inv.InvoiceNumber, &inv.NumberPrefix, &inv.NumberSeq,
		&inv.Currency, &inv.SubtotalMinor, &inv.TaxRateBps, &inv.TaxMinor, &inv.TotalMinor,
		&inv.Status, &inv.IssuedAt, &inv.DueAt, &inv.PaidAt, &inv.PaymentRef, &inv.InternalNotes,
		&inv.UpdatedAt, &inv.CreatedAt,
	}
}

// allocateInvoiceNumber reserves the next sequence for (prefix, currency).
// A transaction-scoped advisory lock serialises concurrent allocations for
// the same series, so MAX()+1 can't hand the same number to two callers
// under READ COMMITTED. The lock is released when tx commits or rolls back.
func allocateInvoiceNumber(ctx context.Context, tx pgx.Tx, prefix, currency string) (string, int64, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"invoice_number:"+prefix+":"+currency); err != nil {
		return "", 0, fmt.Errorf("lock invoice series: %w", err)
	}
	var seq int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(number_seq), 0) + 1
		FROM invoices WHERE number_prefix = $1 AND currency = $2`, prefix, currency).Scan(&seq); err != nil {
		return "", 0, fmt.Errorf("next invoice sequence: %w", err)
	}
	return FormatInvoiceNumber(prefix, currency, seq), seq, nil
}

// profileJSON marshals a BillingProfile to JSONB for the invoice snapshot.
func profileJSON(p *BillingProfile) []byte {
	if p == nil {
		return []byte("{}")
	}
	b, _ := json.Marshal(p)
	return b
}

// GigFeeInfo is the subset of gig data needed to create an invoice.
// Passed from the service layer.
type GigFeeInfo struct {
	ID              uuid.UUID
	Currency        string // gig fee currency; empty means "unknown, trust the request"
	FeeMinor        int64
	TaxRateBps      int64
	TaxMinor        int64
	TotalMinor      int64
	LineDescription string
}

// GetByID returns an invoice by ID with its lines.
func (r *InvoiceRepository) GetByID(ctx context.Context, id uuid.UUID) (*Invoice, []*InvoiceLine, error) {
	var inv Invoice
	err := r.pool.QueryRow(ctx, `
		SELECT id, gig_id, billing_profile, invoice_number, number_prefix, number_seq,
		       currency, subtotal_minor, tax_rate_bps, tax_minor, total_minor,
		       status, issued_at, due_at, paid_at, payment_ref, internal_notes,
		       updated_at, created_at
		FROM invoices WHERE id = $1`, id).Scan(
		&inv.ID, &inv.GigID, &inv.BillingProfile, &inv.InvoiceNumber, &inv.NumberPrefix, &inv.NumberSeq,
		&inv.Currency, &inv.SubtotalMinor, &inv.TaxRateBps, &inv.TaxMinor, &inv.TotalMinor,
		&inv.Status, &inv.IssuedAt, &inv.DueAt, &inv.PaidAt, &inv.PaymentRef, &inv.InternalNotes,
		&inv.UpdatedAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrInvoiceNotFound
		}
		return nil, nil, fmt.Errorf("get invoice: %w", err)
	}

	lines, err := r.getLines(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return &inv, lines, nil
}

// getLines loads lines for an invoice ordered by sort_order.
func (r *InvoiceRepository) getLines(ctx context.Context, invoiceID uuid.UUID) ([]*InvoiceLine, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, invoice_id, sort_order, description, quantity, unit_minor,
		       tax_bps, line_total_minor, created_at
		FROM invoice_lines WHERE invoice_id = $1 ORDER BY sort_order`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("query lines: %w", err)
	}
	defer rows.Close()

	var lines []*InvoiceLine
	for rows.Next() {
		var l InvoiceLine
		if err := rows.Scan(&l.ID, &l.InvoiceID, &l.SortOrder, &l.Description, &l.Quantity,
			&l.UnitMinor, &l.TaxBps, &l.LineTotalMinor, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan line: %w", err)
		}
		lines = append(lines, &l)
	}
	return lines, rows.Err()
}

// List returns invoices matching the filter, newest first.
func (r *InvoiceRepository) List(ctx context.Context, filter InvoiceFilter) ([]*Invoice, error) {
	where := []string{"1=1"}
	args := []any{}
	argN := 1

	if filter.GigID != nil {
		where = append(where, fmt.Sprintf("gig_id = $%d", argN))
		args = append(args, *filter.GigID)
		argN++
	}
	if filter.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argN))
		args = append(args, string(filter.Status))
		argN++
	}
	if filter.Currency != "" {
		where = append(where, fmt.Sprintf("currency = $%d", argN))
		args = append(args, filter.Currency)
		argN++
	}
	if !filter.From.IsZero() {
		where = append(where, fmt.Sprintf("created_at >= $%d", argN))
		args = append(args, filter.From)
		argN++
	}
	if !filter.To.IsZero() {
		where = append(where, fmt.Sprintf("created_at <= $%d", argN))
		args = append(args, filter.To)
		argN++
	}

	query := fmt.Sprintf(`
		SELECT id, gig_id, billing_profile, invoice_number, number_prefix, number_seq,
		       currency, subtotal_minor, tax_rate_bps, tax_minor, total_minor,
		       status, issued_at, due_at, paid_at, payment_ref, internal_notes,
		       updated_at, created_at
		FROM invoices
		WHERE %s
		ORDER BY created_at DESC
		LIMIT 100`, strings.Join(where, " AND "))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	defer rows.Close()

	var invs []*Invoice
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(&inv.ID, &inv.GigID, &inv.BillingProfile, &inv.InvoiceNumber, &inv.NumberPrefix, &inv.NumberSeq,
			&inv.Currency, &inv.SubtotalMinor, &inv.TaxRateBps, &inv.TaxMinor, &inv.TotalMinor,
			&inv.Status, &inv.IssuedAt, &inv.DueAt, &inv.PaidAt, &inv.PaymentRef, &inv.InternalNotes,
			&inv.UpdatedAt, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan invoice: %w", err)
		}
		invs = append(invs, &inv)
	}
	return invs, rows.Err()
}

// UpdateDraft updates a draft invoice. Only draft status is allowed.
func (r *InvoiceRepository) UpdateDraft(ctx context.Context, id uuid.UUID, req UpdateInvoiceRequest) (*Invoice, error) {
	var inv Invoice
	err := r.pool.QueryRow(ctx, `
		UPDATE invoices SET
			number_prefix  = $2,
			due_at         = $3,
			internal_notes = $4,
			updated_at     = now()
		WHERE id = $1 AND status = 'draft' AND updated_at = $5
		RETURNING id, gig_id, billing_profile, invoice_number, number_prefix, number_seq,
		          currency, subtotal_minor, tax_rate_bps, tax_minor, total_minor,
		          status, issued_at, due_at, paid_at, payment_ref, internal_notes,
		          updated_at, created_at`,
		id, req.NumberPrefix, req.DueAt, req.InternalNotes, req.UpdatedAt).Scan(
		&inv.ID, &inv.GigID, &inv.BillingProfile, &inv.InvoiceNumber, &inv.NumberPrefix, &inv.NumberSeq,
		&inv.Currency, &inv.SubtotalMinor, &inv.TaxRateBps, &inv.TaxMinor, &inv.TotalMinor,
		&inv.Status, &inv.IssuedAt, &inv.DueAt, &inv.PaidAt, &inv.PaymentRef, &inv.InternalNotes,
		&inv.UpdatedAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvoiceConflict // could be not found, wrong status, or stale token
		}
		return nil, fmt.Errorf("update draft: %w", err)
	}
	return &inv, nil
}

// Issue transitions draft → issued. Sets issued_at = now(), stores the
// billing profile snapshot. Returns the issued invoice.
func (r *InvoiceRepository) Issue(ctx context.Context, id uuid.UUID, req IssueInvoiceRequest, profile *BillingProfile) (*Invoice, error) {
	var inv Invoice
	err := r.pool.QueryRow(ctx, `
		UPDATE invoices SET
			billing_profile = $2,
			status          = 'issued',
			issued_at       = now(),
			updated_at      = now()
		WHERE id = $1 AND status = 'draft' AND updated_at = $3
		RETURNING id, gig_id, billing_profile, invoice_number, number_prefix, number_seq,
		          currency, subtotal_minor, tax_rate_bps, tax_minor, total_minor,
		          status, issued_at, due_at, paid_at, payment_ref, internal_notes,
		          updated_at, created_at`,
		id, profileJSON(profile), req.UpdatedAt).Scan(
		&inv.ID, &inv.GigID, &inv.BillingProfile, &inv.InvoiceNumber, &inv.NumberPrefix, &inv.NumberSeq,
		&inv.Currency, &inv.SubtotalMinor, &inv.TaxRateBps, &inv.TaxMinor, &inv.TotalMinor,
		&inv.Status, &inv.IssuedAt, &inv.DueAt, &inv.PaidAt, &inv.PaymentRef, &inv.InternalNotes,
		&inv.UpdatedAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvoiceConflict
		}
		return nil, fmt.Errorf("issue invoice: %w", err)
	}
	return &inv, nil
}

// Pay transitions issued → paid. Sets paid_at and payment_ref.
func (r *InvoiceRepository) Pay(ctx context.Context, id uuid.UUID, req PayInvoiceRequest) (*Invoice, error) {
	var inv Invoice
	err := r.pool.QueryRow(ctx, `
		UPDATE invoices SET
			status      = 'paid',
			paid_at     = $2,
			payment_ref = $3,
			updated_at  = now()
		WHERE id = $1 AND status = 'issued' AND updated_at = $4
		RETURNING id, gig_id, billing_profile, invoice_number, number_prefix, number_seq,
		          currency, subtotal_minor, tax_rate_bps, tax_minor, total_minor,
		          status, issued_at, due_at, paid_at, payment_ref, internal_notes,
		          updated_at, created_at`,
		id, req.PaidAt, req.PaymentRef, req.UpdatedAt).Scan(
		&inv.ID, &inv.GigID, &inv.BillingProfile, &inv.InvoiceNumber, &inv.NumberPrefix, &inv.NumberSeq,
		&inv.Currency, &inv.SubtotalMinor, &inv.TaxRateBps, &inv.TaxMinor, &inv.TotalMinor,
		&inv.Status, &inv.IssuedAt, &inv.DueAt, &inv.PaidAt, &inv.PaymentRef, &inv.InternalNotes,
		&inv.UpdatedAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvoiceConflict
		}
		return nil, fmt.Errorf("pay invoice: %w", err)
	}
	return &inv, nil
}

// Cancel transitions issued/draft → cancelled.
func (r *InvoiceRepository) Cancel(ctx context.Context, id uuid.UUID, req CancelInvoiceRequest) (*Invoice, error) {
	var inv Invoice
	err := r.pool.QueryRow(ctx, `
		UPDATE invoices SET
			status     = 'cancelled',
			updated_at = now()
		WHERE id = $1 AND status IN ('draft', 'issued') AND updated_at = $2
		RETURNING id, gig_id, billing_profile, invoice_number, number_prefix, number_seq,
		          currency, subtotal_minor, tax_rate_bps, tax_minor, total_minor,
		          status, issued_at, due_at, paid_at, payment_ref, internal_notes,
		          updated_at, created_at`,
		id, req.UpdatedAt).Scan(
		&inv.ID, &inv.GigID, &inv.BillingProfile, &inv.InvoiceNumber, &inv.NumberPrefix, &inv.NumberSeq,
		&inv.Currency, &inv.SubtotalMinor, &inv.TaxRateBps, &inv.TaxMinor, &inv.TotalMinor,
		&inv.Status, &inv.IssuedAt, &inv.DueAt, &inv.PaidAt, &inv.PaymentRef, &inv.InternalNotes,
		&inv.UpdatedAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvoiceConflict
		}
		return nil, fmt.Errorf("cancel invoice: %w", err)
	}
	return &inv, nil
}

// Correct creates a new draft invoice copying the original (with updated
// amounts if provided) and transitions the original to 'corrected'.
// For Wave 2 we just create a new draft with same gig/currency and
// mark original corrected; amount edits come in Wave 3.
func (r *InvoiceRepository) Correct(ctx context.Context, id uuid.UUID, req CorrectInvoiceRequest, profile *BillingProfile) (*Invoice, error) {
	newInv := &Invoice{}
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Lock the original so its status/token check and the transition
		// to 'corrected' can't interleave with a concurrent pay/cancel.
		var original Invoice
		err := tx.QueryRow(ctx, `
			SELECT id, gig_id, currency, number_prefix, subtotal_minor, tax_rate_bps,
			       tax_minor, total_minor, status, updated_at
			FROM invoices WHERE id = $1 FOR UPDATE`, id).Scan(
			&original.ID, &original.GigID, &original.Currency, &original.NumberPrefix,
			&original.SubtotalMinor, &original.TaxRateBps, &original.TaxMinor, &original.TotalMinor,
			&original.Status, &original.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvoiceNotFound
			}
			return fmt.Errorf("get original for correction: %w", err)
		}
		if original.Status != InvoiceStatusIssued && original.Status != InvoiceStatusPaid {
			return ErrInvoiceBadState // only issued/paid can be corrected
		}
		if !req.UpdatedAt.Equal(original.UpdatedAt) {
			return ErrInvoiceConflict
		}

		if _, err := tx.Exec(ctx, `
			UPDATE invoices SET status = 'corrected', updated_at = now()
			WHERE id = $1`, id); err != nil {
			return fmt.Errorf("mark original corrected: %w", err)
		}

		// Create new draft with same prefix/currency/gig, next sequence.
		number, seq, err := allocateInvoiceNumber(ctx, tx, original.NumberPrefix, original.Currency)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO invoices (gig_id, billing_profile, invoice_number, number_prefix, number_seq,
			                      currency, subtotal_minor, tax_rate_bps, tax_minor, total_minor,
			                      status, due_at, updated_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'draft', NULL, now(), now())
			RETURNING `+invoiceColumns,
			original.GigID, profileJSON(profile), number, original.NumberPrefix, seq, original.Currency,
			original.SubtotalMinor, original.TaxRateBps, original.TaxMinor, original.TotalMinor).Scan(newInv.scanTargets()...)
		if err != nil {
			return fmt.Errorf("insert correction draft: %w", err)
		}

		// Copy lines from original.
		if _, err := tx.Exec(ctx, `
			INSERT INTO invoice_lines (invoice_id, sort_order, description, quantity, unit_minor, tax_bps)
			SELECT $1, sort_order, description, quantity, unit_minor, tax_bps
			FROM invoice_lines WHERE invoice_id = $2`, newInv.ID, original.ID); err != nil {
			return fmt.Errorf("copy lines: %w", err)
		}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrInvoiceConflict
		}
		return nil, err
	}
	return newInv, nil
}

// NextNumber previews the next invoice number for a given prefix/currency
// without allocating it. Used by the UI "New Invoice" screen.
func (r *InvoiceRepository) NextNumber(ctx context.Context, prefix, currency string) (string, int64, error) {
	var seq int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(number_seq), 0) + 1
		FROM invoices
		WHERE number_prefix = $1 AND currency = $2`, prefix, currency).Scan(&seq)
	if err != nil {
		return "", 0, fmt.Errorf("next number: %w", err)
	}
	return FormatInvoiceNumber(prefix, currency, seq), seq, nil
}

// Summaries returns per-currency totals for the dashboard.
func (r *InvoiceRepository) Summaries(ctx context.Context) (map[string]CurrencySummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT currency,
		       SUM(CASE WHEN status = 'issued' THEN total_minor ELSE 0 END) AS issued_total,
		       SUM(CASE WHEN status = 'paid'     THEN total_minor ELSE 0 END) AS paid_total,
		       SUM(CASE WHEN status = 'issued'   THEN 1 ELSE 0 END) AS issued_count,
		       SUM(CASE WHEN status = 'paid'     THEN 1 ELSE 0 END) AS paid_count
		FROM invoices
		GROUP BY currency`)
	if err != nil {
		return nil, fmt.Errorf("summaries: %w", err)
	}
	defer rows.Close()

	sums := make(map[string]CurrencySummary)
	for rows.Next() {
		var curr string
		var issuedTotal, paidTotal int64
		var issuedCount, paidCount int64
		if err := rows.Scan(&curr, &issuedTotal, &paidTotal, &issuedCount, &paidCount); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		sums[curr] = CurrencySummary{
			Currency:    curr,
			IssuedTotal: decimal.NewFromInt(issuedTotal).Div(decimal.NewFromInt(100)),
			PaidTotal:   decimal.NewFromInt(paidTotal).Div(decimal.NewFromInt(100)),
			IssuedCount: issuedCount,
			PaidCount:   paidCount,
		}
	}
	return sums, rows.Err()
}

var _ = sql.ErrNoRows
var _ = uuid.Nil
