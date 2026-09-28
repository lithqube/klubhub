package gig

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

// PaymentTransitionTx is the narrow transaction surface used by payment
// transition processors. *pgxpool.Pool and pgx.Tx both satisfy it.
type PaymentTransitionTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// PaymentSnapshot contains only the gig fields finance needs to process a
// payment transition. Keeping this contract in gig avoids a package cycle.
type PaymentSnapshot struct {
	GigID         uuid.UUID
	Date          time.Time
	EventName     string
	Venue         string
	FeeAmount     decimal.Decimal
	FeeCurrency   string
	PaymentStatus PaymentStatus
}

func PaymentSnapshotFromGig(g *Gig) PaymentSnapshot {
	if g == nil {
		return PaymentSnapshot{}
	}
	return PaymentSnapshot{
		GigID: g.ID, Date: g.Date, EventName: g.EventName, Venue: g.Venue,
		FeeAmount: g.FeeAmount, FeeCurrency: g.FeeCurrency, PaymentStatus: g.PaymentStatus,
	}
}

// PaymentReconciliationMetadata is backward-compatible response metadata. The
// finance reconciliation GET remains the durable source after refresh.
type PaymentReconciliationMetadata struct {
	ID             uuid.UUID `json:"id"`
	EntryID        uuid.UUID `json:"entry_id"`
	Reason         string    `json:"reason"`
	AllowedActions []string  `json:"allowed_actions"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// PaymentTransitionProcessor runs inside the same database transaction as the
// gig/payment-status mutation. Returning an error rolls the status change back.
type PaymentTransitionProcessor interface {
	ProcessGigPaymentTransition(context.Context, PaymentTransitionTx, PaymentSnapshot, PaymentSnapshot) (*PaymentReconciliationMetadata, error)
}
