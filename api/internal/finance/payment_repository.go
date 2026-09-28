package finance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klubhub/dj/api/internal/gig"
	"github.com/shopspring/decimal"
)

// PaymentRepository persists Payment rows.
type PaymentRepository struct {
	pool         *pgxpool.Pool
	transitions  gig.PaymentTransitionProcessor
}

// NewPaymentRepository returns a PaymentRepository backed by pool. No
// transition processor is wired — callers that need FIN-03/04/05 hook
// integration must use NewPaymentRepositoryWithTransitionProcessor.
func NewPaymentRepository(pool *pgxpool.Pool) *PaymentRepository {
	return &PaymentRepository{pool: pool}
}

// NewPaymentRepositoryWithTransitionProcessor wires the shared
// gig-payment transition processor so the invoice-driven sync path
// participates in FIN-03/04/05 alongside the manual gig-update path.
func NewPaymentRepositoryWithTransitionProcessor(pool *pgxpool.Pool, processor gig.PaymentTransitionProcessor) *PaymentRepository {
	return &PaymentRepository{pool: pool, transitions: processor}
}

// Pool exposes the underlying pool for tests.
func (r *PaymentRepository) Pool() *pgxpool.Pool { return r.pool }

// Create inserts a new payment row with status 'pending'. The parent
// invoice is locked for the duration of the transaction so the balance
// check and the insert are atomic with respect to other payment writes.
func (r *PaymentRepository) Create(ctx context.Context, invoiceID uuid.UUID, req CreatePaymentRequest) (*Payment, error) {
	var p Payment
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		status, kind, currency, err := lockInvoiceForPayment(ctx, tx, invoiceID)
		if err != nil {
			return err
		}
		// Credit notes are never payable; only issued/paid invoices take money.
		if kind != InvoiceKindInvoice || (status != InvoiceStatusIssued && status != InvoiceStatusPaid) {
			return ErrPaymentInvoiceState
		}
		if currency != req.Currency {
			return PaymentValidationErrors{{"currency", "must match invoice currency " + currency}}
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO payments (invoice_id, currency, amount_minor, kind, status, method, reference, received_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, now(), now())
			RETURNING `+paymentColumns,
			invoiceID, req.Currency, req.AmountMinor, req.Kind, req.Method, req.Reference, req.ReceivedAt).Scan(p.scanTargets()...)
		if err != nil {
			return fmt.Errorf("create payment: %w", err)
		}
		if err := checkPaymentBalance(ctx, tx, invoiceID); err != nil {
			return err
		}
		return syncGigPaymentStatusWith(ctx, tx, invoiceID, r.transitions)
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// paymentColumns is the RETURNING/SELECT list matching Payment.scanTargets.
const paymentColumns = `id, invoice_id, currency, amount_minor, kind, status, method, reference, received_at, created_at, updated_at`

func (p *Payment) scanTargets() []any {
	return []any{&p.ID, &p.InvoiceID, &p.Currency, &p.AmountMinor, &p.Kind, &p.Status,
		&p.Method, &p.Reference, &p.ReceivedAt, &p.CreatedAt, &p.UpdatedAt}
}

// lockInvoiceForPayment takes a row lock on the invoice and returns its
// status, kind and currency. Every payment write goes through this lock,
// which serialises balance checks per invoice.
func lockInvoiceForPayment(ctx context.Context, tx pgx.Tx, invoiceID uuid.UUID) (InvoiceStatus, InvoiceKind, string, error) {
	var status InvoiceStatus
	var kind InvoiceKind
	var currency string
	err := tx.QueryRow(ctx, `SELECT status, kind, currency FROM invoices WHERE id = $1 FOR UPDATE`, invoiceID).
		Scan(&status, &kind, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrPaymentInvoiceNotFound
	}
	if err != nil {
		return "", "", "", fmt.Errorf("lock invoice: %w", err)
	}
	return status, kind, currency, nil
}

// checkPaymentBalance enforces the invariants from 014_payments.sql after a
// write, inside the same transaction, against the invoice's NET PAYABLE
// (total − withholding; the withheld part is paid to the tax office, not
// to the DJ):
//   - pending + completed money in (deposits, payments) minus completed
//     refunds never exceeds net payable, so an invoice can't be
//     over-collected even while payments are still pending;
//   - completed refunds never exceed completed money in.
func checkPaymentBalance(ctx context.Context, tx pgx.Tx, invoiceID uuid.UUID) error {
	var total, committedIn, completedIn, refunded int64
	err := tx.QueryRow(ctx, `
		SELECT i.net_payable_minor,
		       COALESCE(SUM(p.amount_minor) FILTER (WHERE p.kind <> 'refund' AND p.status IN ('pending','completed')), 0),
		       COALESCE(SUM(p.amount_minor) FILTER (WHERE p.kind <> 'refund' AND p.status = 'completed'), 0),
		       COALESCE(SUM(p.amount_minor) FILTER (WHERE p.kind = 'refund' AND p.status = 'completed'), 0)
		FROM invoices i
		LEFT JOIN payments p ON p.invoice_id = i.id
		WHERE i.id = $1
		GROUP BY i.net_payable_minor`, invoiceID).Scan(&total, &committedIn, &completedIn, &refunded)
	if err != nil {
		return fmt.Errorf("check payment balance: %w", err)
	}
	if refunded > completedIn {
		return ErrPaymentExceedsBalance
	}
	if committedIn-refunded > total {
		return ErrPaymentExceedsBalance
	}
	return nil
}

// syncGigPaymentStatus derives gigs.payment_status from the invoice balance
// in the caller's transaction: 'paid' when received ≥ net payable,
// 'deposit_paid' when received > 0, else 'unpaid' (received = completed
// deposits + payments − completed refunds). When a transition processor
// is wired, the before/after snapshots of the gig are run through it so
// the invoice-driven sync path participates in FIN-03/04/05. The gig's
// updated_at (its concurrency token) only moves when the status actually
// changes; if the processor wires a different updated_at in the same tx
// the gig-write happens after the sync so the processor sees the new
// status as the "after" snapshot.
func syncGigPaymentStatus(ctx context.Context, tx pgx.Tx, invoiceID uuid.UUID) error {
	return syncGigPaymentStatusWith(ctx, tx, invoiceID, nil)
}

// syncGigPaymentStatusWith is the same as syncGigPaymentStatus but lets
// the caller pass in an optional processor for FIN-03/04/05 integration.
func syncGigPaymentStatusWith(ctx context.Context, tx pgx.Tx, invoiceID uuid.UUID, processor gig.PaymentTransitionProcessor) error {
	// Compute the target status from the current balance. Read the gig
	// before/after under the same transaction so the transition
	// processor sees a consistent snapshot.
	type gigRow struct {
		id        uuid.UUID
		date      time.Time
		eventName string
		venue     string
		fee       decimal.Decimal
		currency  string
		before    gig.PaymentStatus
	}
	var before gigRow
	if err := tx.QueryRow(ctx, `SELECT g.id, g.date, g.event_name, g.venue, g.fee_amount, g.fee_currency, g.payment_status
		FROM gigs g JOIN invoices i ON i.gig_id = g.id WHERE i.id = $1`, invoiceID).
		Scan(&before.id, &before.date, &before.eventName, &before.venue, &before.fee, &before.currency, &before.before); err != nil {
		return fmt.Errorf("load gig for payment sync: %w", err)
	}

	var target gig.PaymentStatus
	if err := tx.QueryRow(ctx, `WITH bal AS (
		SELECT i.net_payable_minor AS net,
		       COALESCE(SUM(CASE WHEN p.kind = 'refund' THEN -p.amount_minor ELSE p.amount_minor END)
		                FILTER (WHERE p.status = 'completed'), 0) AS received
		FROM invoices i
		LEFT JOIN payments p ON p.invoice_id = i.id
		WHERE i.id = $1
		GROUP BY i.net_payable_minor
	) SELECT (CASE WHEN received >= net THEN 'paid'
	               WHEN received > 0    THEN 'deposit_paid'
	               ELSE 'unpaid' END)::payment_status FROM bal`, invoiceID).Scan(&target); err != nil {
		return fmt.Errorf("compute gig payment target: %w", err)
	}

	if target != before.before {
		if _, err := tx.Exec(ctx, `UPDATE gigs SET payment_status=$2, updated_at=now()
			WHERE id=$1`, before.id, target); err != nil {
			return fmt.Errorf("update gig payment status: %w", err)
		}
	}

	if processor == nil || target == before.before {
		return nil
	}

	afterSnap := gig.PaymentSnapshot{
		GigID:         before.id,
		Date:          before.date,
		EventName:     before.eventName,
		Venue:         before.venue,
		FeeAmount:     before.fee,
		FeeCurrency:   before.currency,
		PaymentStatus: target,
	}
	beforeSnap := afterSnap
	beforeSnap.PaymentStatus = before.before
	if _, err := processor.ProcessGigPaymentTransition(ctx, tx, beforeSnap, afterSnap); err != nil {
		return fmt.Errorf("gig payment transition hook: %w", err)
	}
	return nil
}

// GetByID returns a payment by ID.
func (r *PaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*Payment, error) {
	var p Payment
	err := r.pool.QueryRow(ctx, `
		SELECT id, invoice_id, currency, amount_minor, kind, status, method, reference, received_at, created_at, updated_at
		FROM payments WHERE id = $1`, id).Scan(
		&p.ID, &p.InvoiceID, &p.Currency, &p.AmountMinor, &p.Kind, &p.Status, &p.Method, &p.Reference, &p.ReceivedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get payment: %w", err)
	}
	return &p, nil
}

// ListByInvoice returns all payments for an invoice, newest first.
func (r *PaymentRepository) ListByInvoice(ctx context.Context, invoiceID uuid.UUID) ([]*Payment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, invoice_id, currency, amount_minor, kind, status, method, reference, received_at, created_at, updated_at
		FROM payments WHERE invoice_id = $1 ORDER BY created_at DESC`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()

	var payments []*Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.InvoiceID, &p.Currency, &p.AmountMinor, &p.Kind, &p.Status, &p.Method, &p.Reference, &p.ReceivedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		payments = append(payments, &p)
	}
	return payments, rows.Err()
}

// Update updates a payment's status/method/reference/received_at with
// optimistic concurrency. Status changes can move money (failed →
// completed, refund → completed), so the balance is re-checked under the
// invoice lock before commit.
func (r *PaymentRepository) Update(ctx context.Context, id uuid.UUID, req UpdatePaymentRequest) (*Payment, error) {
	var p Payment
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var invoiceID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT invoice_id FROM payments WHERE id = $1`, id).Scan(&invoiceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("get payment invoice: %w", err)
		}
		if _, _, _, err := lockInvoiceForPayment(ctx, tx, invoiceID); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			UPDATE payments SET
				status       = $2,
				method       = $3,
				reference    = $4,
				received_at  = $5,
				updated_at   = now()
			WHERE id = $1 AND updated_at = $6
			RETURNING `+paymentColumns,
			id, req.Status, req.Method, req.Reference, req.ReceivedAt, req.UpdatedAt).Scan(p.scanTargets()...)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPaymentConflict
		}
		if err != nil {
			return fmt.Errorf("update payment: %w", err)
		}
		if err := checkPaymentBalance(ctx, tx, invoiceID); err != nil {
			return err
		}
		return syncGigPaymentStatusWith(ctx, tx, invoiceID, r.transitions)
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SumByInvoice returns the net amount received for an invoice: completed
// deposits and payments minus completed refunds.
func (r *PaymentRepository) SumByInvoice(ctx context.Context, invoiceID uuid.UUID) (int64, error) {
	var sum int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE WHEN kind = 'refund' THEN -amount_minor ELSE amount_minor END), 0)
		FROM payments WHERE invoice_id = $1 AND status = 'completed'`, invoiceID).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("sum payments: %w", err)
	}
	return sum, nil
}
