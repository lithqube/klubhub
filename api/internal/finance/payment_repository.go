package finance

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PaymentRepository persists Payment rows.
type PaymentRepository struct {
	pool *pgxpool.Pool
}

// NewPaymentRepository returns a PaymentRepository backed by pool.
func NewPaymentRepository(pool *pgxpool.Pool) *PaymentRepository {
	return &PaymentRepository{pool: pool}
}

// Pool exposes the underlying pool for tests.
func (r *PaymentRepository) Pool() *pgxpool.Pool { return r.pool }

// Create inserts a new payment row with status 'pending'.
func (r *PaymentRepository) Create(ctx context.Context, invoiceID uuid.UUID, req CreatePaymentRequest) (*Payment, error) {
	var p Payment
	err := r.pool.QueryRow(ctx, `
		INSERT INTO payments (invoice_id, currency, amount_minor, kind, status, method, reference, received_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, now(), now())
		RETURNING id, invoice_id, currency, amount_minor, kind, status, method, reference, received_at, created_at, updated_at`,
		invoiceID, req.Currency, req.AmountMinor, req.Kind, req.Method, req.Reference, req.ReceivedAt).Scan(
		&p.ID, &p.InvoiceID, &p.Currency, &p.AmountMinor, &p.Kind, &p.Status, &p.Method, &p.Reference, &p.ReceivedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}
	return &p, nil
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

// Update updates a payment's status/method/reference/received_at with optimistic concurrency.
func (r *PaymentRepository) Update(ctx context.Context, id uuid.UUID, req UpdatePaymentRequest) (*Payment, error) {
	var p Payment
	err := r.pool.QueryRow(ctx, `
		UPDATE payments SET
			status       = $2,
			method       = $3,
			reference    = $4,
			received_at  = $5,
			updated_at   = now()
		WHERE id = $1 AND updated_at = $6
		RETURNING id, invoice_id, currency, amount_minor, kind, status, method, reference, received_at, created_at, updated_at`,
		id, req.Status, req.Method, req.Reference, req.ReceivedAt, req.UpdatedAt).Scan(
		&p.ID, &p.InvoiceID, &p.Currency, &p.AmountMinor, &p.Kind, &p.Status, &p.Method, &p.Reference, &p.ReceivedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentConflict
		}
		return nil, fmt.Errorf("update payment: %w", err)
	}
	return &p, nil
}

// SumByInvoice returns the sum of completed payments for an invoice.
func (r *PaymentRepository) SumByInvoice(ctx context.Context, invoiceID uuid.UUID) (int64, error) {
	var sum int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_minor), 0)
		FROM payments WHERE invoice_id = $1 AND status = 'completed'`, invoiceID).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("sum payments: %w", err)
	}
	return sum, nil
}

var _ = uuid.Nil