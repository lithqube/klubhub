package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/klubhub/dj/api/internal/finance/tax"
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

// dbtx is satisfied by both *pgxpool.Pool and pgx.Tx.
type dbtx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// paymentTotalsLateral aggregates an invoice's payments in the same query
// as the invoice row (no N+1): received = completed money in − completed
// refunds; pending = pending money in.
const paymentTotalsLateral = `
	LEFT JOIN LATERAL (
		SELECT (SUM(CASE WHEN p.kind = 'refund' THEN -p.amount_minor ELSE p.amount_minor END)
		           FILTER (WHERE p.status = 'completed'))::BIGINT AS received,
		       (SUM(p.amount_minor) FILTER (WHERE p.kind <> 'refund' AND p.status = 'pending'))::BIGINT AS pending
		FROM payments p WHERE p.invoice_id = i.id
	) pay ON true`

// invoiceSelect is the SELECT matching Invoice.scanTargets.
const invoiceSelect = `
	SELECT i.id, i.kind, i.gig_id, i.credits_invoice_id, i.replaced_by_invoice_id,
	       i.invoice_number, i.number_prefix, i.number_seq, i.currency, i.status,
	       to_char(i.supply_date, 'YYYY-MM-DD'), i.issued_at, i.due_at, i.paid_at,
	       i.payment_ref, i.internal_notes, i.customer, i.billing_profile,
	       i.vat_treatment, i.tax_rate_bps, i.tax_note,
	       i.subtotal_minor, i.tax_minor, i.total_minor,
	       i.withholding_rate_bps, i.withholding_minor, i.net_payable_minor, i.tax_breakdown,
	       COALESCE(pay.received, 0), COALESCE(pay.pending, 0),
	       i.updated_at, i.created_at
	FROM invoices i` + paymentTotalsLateral

func (inv *Invoice) scanTargets() []any {
	return []any{
		&inv.ID, &inv.Kind, &inv.GigID, &inv.CreditsInvoiceID, &inv.ReplacedByInvoiceID,
		&inv.InvoiceNumber, &inv.NumberPrefix, &inv.NumberSeq, &inv.Currency, &inv.Status,
		&inv.SupplyDate, &inv.IssuedAt, &inv.DueAt, &inv.PaidAt,
		&inv.PaymentRef, &inv.InternalNotes, &inv.Customer, &inv.BillingProfile,
		&inv.VATTreatment, &inv.TaxRateBps, &inv.TaxNote,
		&inv.SubtotalMinor, &inv.TaxMinor, &inv.TotalMinor,
		&inv.WithholdingRateBps, &inv.WithholdingMinor, &inv.NetPayableMinor, &inv.TaxBreakdown,
		&inv.ReceivedMinor, &inv.PendingMinor,
		&inv.UpdatedAt, &inv.CreatedAt,
	}
}

// fetchInvoice loads one invoice with its payment balances.
func fetchInvoice(ctx context.Context, q dbtx, id uuid.UUID) (*Invoice, error) {
	var inv Invoice
	err := q.QueryRow(ctx, invoiceSelect+` WHERE i.id = $1`, id).Scan(inv.scanTargets()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvoiceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	inv.finalizeBalances()
	return &inv, nil
}

func jsonb(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// profileJSON marshals a BillingProfile to JSONB for the supplier snapshot.
func profileJSON(p *BillingProfile) []byte {
	if p == nil {
		return []byte("{}")
	}
	return jsonb(p)
}

// CreateDraft inserts an unnumbered draft and its lines.
func (r *InvoiceRepository) CreateDraft(ctx context.Context, in DraftInput) (*Invoice, error) {
	var inv *Invoice
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var id uuid.UUID
		t := in.Totals
		err := tx.QueryRow(ctx, `
			INSERT INTO invoices (kind, gig_id, billing_profile, invoice_number, number_prefix, number_seq,
			                      currency, status, supply_date, due_at,
			                      customer, customer_contact_id, vat_treatment, tax_rate_bps, tax_note,
			                      subtotal_minor, tax_minor, total_minor,
			                      withholding_rate_bps, withholding_minor, net_payable_minor, tax_breakdown)
			VALUES ('invoice', $1, '{}', NULL, $2, NULL, $3, 'draft', NULLIF($4::text, '')::date, $5,
			        $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
			RETURNING id`,
			in.GigID, in.NumberPrefix, in.Currency, in.SupplyDate, in.DueAt,
			jsonb(in.Customer), in.Customer.ContactID, string(in.VATTreatment), in.TaxRateBps, in.TaxNote,
			t.SubtotalMinor, t.TaxMinor, t.TotalMinor,
			in.WithholdingRateBps, t.WithholdingMinor, t.NetPayableMinor, jsonb(t.Breakdown)).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert invoice: %w", err)
		}
		for i, l := range in.Lines {
			rate := in.TaxRateBps
			if i < len(t.LineTaxBps) {
				rate = t.LineTaxBps[i]
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO invoice_lines (invoice_id, sort_order, description, quantity, unit_minor, tax_bps)
				VALUES ($1, $2, $3, $4, $5, $6)`, id, i, l.Description, l.Quantity, l.UnitMinor, rate); err != nil {
				return fmt.Errorf("insert invoice line: %w", err)
			}
		}
		inv, err = fetchInvoice(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// allocateInvoiceNumber reserves the next sequence for (prefix, currency).
// A transaction-scoped advisory lock serialises concurrent allocations for
// the same series, so MAX()+1 can't hand the same number to two callers
// under READ COMMITTED. It must run inside the transaction that marks the
// document issued, so a rolled-back issue consumes no number (gap-free).
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

// GigFeeInfo is the gig data needed to create an invoice draft.
type GigFeeInfo struct {
	ID              uuid.UUID
	Currency        string // gig fee currency; empty means "unknown, trust the request"
	FeeMinor        int64
	LineDescription string
	Date            string // YYYY-MM-DD; default supply date
	// Customer is the pre-filled customer (gig contact → promoter contact →
	// promoter name/email); nil when nothing is known.
	Customer *Party
}

// GetByID returns an invoice by ID with its lines.
func (r *InvoiceRepository) GetByID(ctx context.Context, id uuid.UUID) (*Invoice, []*InvoiceLine, error) {
	inv, err := fetchInvoice(ctx, r.pool, id)
	if err != nil {
		return nil, nil, err
	}
	lines, err := getLines(ctx, r.pool, id)
	if err != nil {
		return nil, nil, err
	}
	return inv, lines, nil
}

// getLines loads lines for an invoice ordered by sort_order.
func getLines(ctx context.Context, q dbtx, invoiceID uuid.UUID) ([]*InvoiceLine, error) {
	rows, err := q.Query(ctx, `
		SELECT id, invoice_id, sort_order, description, quantity, unit_minor,
		       tax_bps, line_total_minor, created_at
		FROM invoice_lines WHERE invoice_id = $1 ORDER BY sort_order, id`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("query lines: %w", err)
	}
	defer rows.Close()

	lines := []*InvoiceLine{}
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

// List returns invoices matching the filter, newest first (max 100), with
// payment balances computed in the same query.
func (r *InvoiceRepository) List(ctx context.Context, filter InvoiceFilter) ([]*Invoice, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if filter.GigID != nil {
		add("i.gig_id = $%d", *filter.GigID)
	}
	if filter.Status != "" {
		add("i.status = $%d", string(filter.Status))
	}
	if filter.Kind != "" {
		add("i.kind = $%d", string(filter.Kind))
	}
	if filter.Currency != "" {
		add("i.currency = $%d", filter.Currency)
	}
	if !filter.From.IsZero() {
		add("i.created_at >= $%d", filter.From)
	}
	if !filter.To.IsZero() {
		add("i.created_at <= $%d", filter.To)
	}

	rows, err := r.pool.Query(ctx, invoiceSelect+`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY i.created_at DESC, i.id
		LIMIT 100`, args...)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	defer rows.Close()

	invs := []*Invoice{}
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(inv.scanTargets()...); err != nil {
			return nil, fmt.Errorf("scan invoice: %w", err)
		}
		inv.finalizeBalances()
		invs = append(invs, &inv)
	}
	return invs, rows.Err()
}

// lockedInvoice is the state read under FOR UPDATE before a transition.
type lockedInvoice struct {
	status    InvoiceStatus
	kind      InvoiceKind
	updatedAt time.Time
}

// lockForTransition row-locks the invoice and classifies failures the same
// way the agreement Sign path does: missing row → ErrInvoiceNotFound, stale
// token → ErrInvoiceConflict. The caller then checks status/kind and
// returns ErrInvoiceBadState, so 409 conflict and 409 bad_state are
// distinguishable and race-free.
func lockForTransition(ctx context.Context, tx pgx.Tx, id uuid.UUID, token time.Time) (lockedInvoice, error) {
	var li lockedInvoice
	err := tx.QueryRow(ctx, `SELECT status, kind, updated_at FROM invoices WHERE id = $1 FOR UPDATE`, id).
		Scan(&li.status, &li.kind, &li.updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return li, ErrInvoiceNotFound
	}
	if err != nil {
		return li, fmt.Errorf("lock invoice: %w", err)
	}
	if !li.updatedAt.Equal(token) {
		return li, ErrInvoiceConflict
	}
	return li, nil
}

// isDraftInvoice reports whether a locked row is an editable draft invoice.
func (li lockedInvoice) isDraftInvoice() bool {
	return li.kind == InvoiceKindInvoice && li.status == InvoiceStatusDraft
}

// UpdateDraft replaces the editable fields of a draft, recomputes totals
// from its lines and rewrites every line's tax_bps.
func (r *InvoiceRepository) UpdateDraft(ctx context.Context, id uuid.UUID, req UpdateInvoiceRequest) (*Invoice, error) {
	var inv *Invoice
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		li, err := lockForTransition(ctx, tx, id, req.UpdatedAt)
		if err != nil {
			return err
		}
		if !li.isDraftInvoice() {
			return ErrInvoiceBadState
		}
		lines, err := getLines(ctx, tx, id)
		if err != nil {
			return err
		}
		taxLines := make([]tax.Line, len(lines))
		for i, l := range lines {
			taxLines[i] = tax.Line{NetMinor: l.LineTotalMinor, TaxBps: l.TaxBps}
		}
		t := tax.ComputeTotals(taxLines, req.TaxRateBps, req.WithholdingRateBps)
		for i, l := range lines {
			if _, err := tx.Exec(ctx, `UPDATE invoice_lines SET tax_bps = $2 WHERE id = $1`, l.ID, t.LineTaxBps[i]); err != nil {
				return fmt.Errorf("update line tax: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE invoices SET
				customer             = $2,
				customer_contact_id  = $3,
				vat_treatment        = $4,
				tax_rate_bps         = $5,
				tax_note             = $6,
				withholding_rate_bps = $7,
				supply_date          = NULLIF($8::text, '')::date,
				due_at               = $9,
				number_prefix        = $10,
				internal_notes       = $11,
				subtotal_minor       = $12,
				tax_minor            = $13,
				total_minor          = $14,
				withholding_minor    = $15,
				net_payable_minor    = $16,
				tax_breakdown        = $17,
				updated_at           = now()
			WHERE id = $1`,
			id, jsonb(req.Customer), req.Customer.ContactID, string(req.VATTreatment), req.TaxRateBps, req.TaxNote,
			req.WithholdingRateBps, req.SupplyDate, req.DueAt.ptr(), req.NumberPrefix, req.InternalNotes,
			t.SubtotalMinor, t.TaxMinor, t.TotalMinor, t.WithholdingMinor, t.NetPayableMinor, jsonb(t.Breakdown)); err != nil {
			return fmt.Errorf("update draft: %w", err)
		}
		inv, err = fetchInvoice(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// IssueCheckFunc validates a locked draft right before it is numbered.
// gigCurrency is the gig's fee currency read in the same transaction (""
// when the gig is gone). It must not do I/O: the transaction holds a
// connection, and concurrent issues would otherwise exhaust the pool.
type IssueCheckFunc func(inv *Invoice, gigCurrency string) []tax.Problem

// Issue transitions draft → issued in one transaction: lock the row, run
// the issue check, allocate the next number of the draft's series, snapshot
// the billing profile. A failed check or rollback consumes no number.
func (r *InvoiceRepository) Issue(ctx context.Context, id uuid.UUID, req IssueInvoiceRequest, profile *BillingProfile, check IssueCheckFunc) (*Invoice, error) {
	var inv *Invoice
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		li, err := lockForTransition(ctx, tx, id, req.UpdatedAt)
		if err != nil {
			return err
		}
		if !li.isDraftInvoice() {
			return ErrInvoiceBadState
		}
		draft, err := fetchInvoice(ctx, tx, id)
		if err != nil {
			return err
		}
		if check != nil {
			var gigCurrency string
			err := tx.QueryRow(ctx, `SELECT upper(fee_currency) FROM gigs WHERE id = $1 AND deleted_at IS NULL`,
				draft.GigID).Scan(&gigCurrency)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("read gig currency: %w", err)
			}
			if problems := check(draft, gigCurrency); len(problems) > 0 {
				return &NotIssuableError{Problems: problems}
			}
		}
		number, seq, err := allocateInvoiceNumber(ctx, tx, draft.NumberPrefix, draft.Currency)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE invoices SET
				invoice_number  = $2,
				number_seq      = $3,
				billing_profile = $4,
				status          = 'issued',
				issued_at       = now(),
				updated_at      = now()
			WHERE id = $1`, id, number, seq, profileJSON(profile)); err != nil {
			return fmt.Errorf("issue invoice: %w", err)
		}
		inv, err = fetchInvoice(ctx, tx, id)
		return err
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrInvoiceConflict
		}
		return nil, err
	}
	return inv, nil
}

// Pay transitions issued → paid. Sets paid_at and payment_ref.
func (r *InvoiceRepository) Pay(ctx context.Context, id uuid.UUID, req PayInvoiceRequest) (*Invoice, error) {
	return r.transition(ctx, id, req.UpdatedAt,
		func(li lockedInvoice) bool { return li.kind == InvoiceKindInvoice && li.status == InvoiceStatusIssued },
		`UPDATE invoices SET status = 'paid', paid_at = $2, payment_ref = $3, updated_at = now() WHERE id = $1`,
		req.PaidAt, req.PaymentRef)
}

// Cancel transitions draft → cancelled. Drafts are unnumbered, so nothing is
// consumed from the series. Issued invoices are reversed by credit note.
func (r *InvoiceRepository) Cancel(ctx context.Context, id uuid.UUID, req CancelInvoiceRequest) (*Invoice, error) {
	return r.transition(ctx, id, req.UpdatedAt, lockedInvoice.isDraftInvoice,
		`UPDATE invoices SET status = 'cancelled', updated_at = now() WHERE id = $1`)
}

// transition runs a single-statement state change under the row lock.
func (r *InvoiceRepository) transition(ctx context.Context, id uuid.UUID, token time.Time,
	allowed func(lockedInvoice) bool, sql string, args ...any) (*Invoice, error) {
	var inv *Invoice
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		li, err := lockForTransition(ctx, tx, id, token)
		if err != nil {
			return err
		}
		if !allowed(li) {
			return ErrInvoiceBadState
		}
		if _, err := tx.Exec(ctx, sql, append([]any{id}, args...)...); err != nil {
			return fmt.Errorf("invoice transition: %w", err)
		}
		inv, err = fetchInvoice(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// copyLines duplicates an invoice's lines onto another invoice.
func copyLines(ctx context.Context, tx pgx.Tx, from, to uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO invoice_lines (invoice_id, sort_order, description, quantity, unit_minor, tax_bps)
		SELECT $1, sort_order, description, quantity, unit_minor, tax_bps
		FROM invoice_lines WHERE invoice_id = $2`, to, from)
	if err != nil {
		return fmt.Errorf("copy lines: %w", err)
	}
	return nil
}

// CreditNote reverses an issued or paid invoice in full. The credit note
// is issued immediately in the CN series of the invoice's currency, copies
// the original's customer, tax treatment, amounts and lines, and snapshots
// the current billing profile (or the original's snapshot if none). The
// original becomes 'credited' — or, when correct is true, 'corrected' with
// a new unnumbered draft copy linked via replaced_by_invoice_id.
func (r *InvoiceRepository) CreditNote(ctx context.Context, id uuid.UUID, req CreditNoteRequest, profile *BillingProfile, correct bool) (*CreditNoteResult, error) {
	var res CreditNoteResult
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		li, err := lockForTransition(ctx, tx, id, req.UpdatedAt)
		if err != nil {
			return err
		}
		if li.kind != InvoiceKindInvoice || (li.status != InvoiceStatusIssued && li.status != InvoiceStatusPaid) {
			return ErrInvoiceBadState
		}
		var currency string
		if err := tx.QueryRow(ctx, `SELECT currency FROM invoices WHERE id = $1`, id).Scan(&currency); err != nil {
			return fmt.Errorf("read original: %w", err)
		}
		number, seq, err := allocateInvoiceNumber(ctx, tx, CreditNotePrefix, currency)
		if err != nil {
			return err
		}
		var snapshot []byte
		if profile != nil {
			snapshot = profileJSON(profile)
		}
		var cnID uuid.UUID
		err = tx.QueryRow(ctx, `
			INSERT INTO invoices (kind, gig_id, credits_invoice_id, billing_profile,
			                      invoice_number, number_prefix, number_seq, currency, status, issued_at,
			                      supply_date, customer, customer_contact_id, vat_treatment, tax_rate_bps, tax_note,
			                      subtotal_minor, tax_minor, total_minor,
			                      withholding_rate_bps, withholding_minor, net_payable_minor, tax_breakdown,
			                      internal_notes)
			SELECT 'credit_note', gig_id, id, COALESCE($2::jsonb, billing_profile),
			       $3, $4, $5, currency, 'issued', now(),
			       supply_date, customer, customer_contact_id, vat_treatment, tax_rate_bps, tax_note,
			       subtotal_minor, tax_minor, total_minor,
			       withholding_rate_bps, withholding_minor, net_payable_minor, tax_breakdown,
			       $6
			FROM invoices WHERE id = $1
			RETURNING id`, id, snapshot, number, CreditNotePrefix, seq, req.Reason).Scan(&cnID)
		if err != nil {
			return fmt.Errorf("insert credit note: %w", err)
		}
		if err := copyLines(ctx, tx, id, cnID); err != nil {
			return err
		}

		if correct {
			var replID uuid.UUID
			err = tx.QueryRow(ctx, `
				INSERT INTO invoices (kind, gig_id, billing_profile, invoice_number, number_prefix, number_seq,
				                      currency, status, supply_date, due_at,
				                      customer, customer_contact_id, vat_treatment, tax_rate_bps, tax_note,
				                      subtotal_minor, tax_minor, total_minor,
				                      withholding_rate_bps, withholding_minor, net_payable_minor, tax_breakdown,
				                      internal_notes)
				SELECT 'invoice', gig_id, '{}', NULL, number_prefix, NULL,
				       currency, 'draft', supply_date, NULL,
				       customer, customer_contact_id, vat_treatment, tax_rate_bps, tax_note,
				       subtotal_minor, tax_minor, total_minor,
				       withholding_rate_bps, withholding_minor, net_payable_minor, tax_breakdown,
				       'Replaces ' || invoice_number
				FROM invoices WHERE id = $1
				RETURNING id`, id).Scan(&replID)
			if err != nil {
				return fmt.Errorf("insert replacement draft: %w", err)
			}
			if err := copyLines(ctx, tx, id, replID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE invoices SET status = 'corrected', replaced_by_invoice_id = $2, updated_at = now()
				WHERE id = $1`, id, replID); err != nil {
				return fmt.Errorf("mark original corrected: %w", err)
			}
			if res.Replacement, err = fetchInvoice(ctx, tx, replID); err != nil {
				return err
			}
		} else if _, err := tx.Exec(ctx, `
			UPDATE invoices SET status = 'credited', updated_at = now() WHERE id = $1`, id); err != nil {
			return fmt.Errorf("mark original credited: %w", err)
		}

		if res.CreditNote, err = fetchInvoice(ctx, tx, cnID); err != nil {
			return err
		}
		res.Original, err = fetchInvoice(ctx, tx, id)
		return err
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrInvoiceConflict
		}
		return nil, err
	}
	return &res, nil
}

// NextNumber previews the next invoice number for a given prefix/currency
// without allocating it.
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

// Summaries returns per-currency dashboard totals in minor units, computed
// in one query (see CurrencySummary for the definitions).
func (r *InvoiceRepository) Summaries(ctx context.Context) (map[string]CurrencySummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT i.currency,
		       COUNT(*) FILTER (WHERE i.status = 'draft'),
		       COUNT(*) FILTER (WHERE i.status = 'issued'),
		       COUNT(*) FILTER (WHERE i.status = 'paid'),
		       COALESCE(SUM(GREATEST(i.net_payable_minor - COALESCE(pay.received, 0) - COALESCE(pay.pending, 0), 0))
		                FILTER (WHERE i.status = 'issued'), 0)::BIGINT,
		       COALESCE(SUM(CASE WHEN i.status = 'paid' THEN i.net_payable_minor
		                         ELSE LEAST(GREATEST(COALESCE(pay.received, 0), 0), i.net_payable_minor) END)
		                FILTER (WHERE i.status IN ('issued', 'paid')), 0)::BIGINT
		FROM invoices i`+paymentTotalsLateral+`
		WHERE i.kind = 'invoice'
		GROUP BY i.currency`)
	if err != nil {
		return nil, fmt.Errorf("summaries: %w", err)
	}
	defer rows.Close()

	sums := make(map[string]CurrencySummary)
	for rows.Next() {
		var s CurrencySummary
		if err := rows.Scan(&s.Currency, &s.DraftCount, &s.IssuedCount, &s.PaidCount, &s.OutstandingMinor, &s.PaidMinor); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		sums[s.Currency] = s
	}
	return sums, rows.Err()
}
