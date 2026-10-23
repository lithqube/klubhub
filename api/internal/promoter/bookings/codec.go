package bookings

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/email"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
)

// ErrNoKeyring is returned by the codec when it is wired without a keyring.
var ErrNoKeyring = errors.New("bookings: envelope codec has no keyring")

// EnvelopeCodec is the promoter-side email.FieldCodec (SPEC §6.1/§6.8 v2.1):
// the shared EmailService seals the outbox PII columns (to_email, subject,
// body, body_html, reply_to) on write and unseals them for delivery/log, and
// this codec encrypts them with the tenant DEK so talent PII is never stored
// in clear. The DJ side uses email.IdentityCodec (plaintext passthrough).
//
// The tenant-scoped repository calls the codec inside its WithTenant
// transaction scope, so the tenant comes from ctx and the DEK lookup shares
// the transaction with the insert/read (the audience pattern). The codec maps
// the service's logical column names (to_email, subject, …) to the physical
// sealed columns (to_email_enc, subject_enc, …): the envelope field — and
// therefore the AAD — names the physical column, matching the audience
// convention. SealEmail's blind index uses the physical <col>_bidx scope.
type EnvelopeCodec struct {
	Keys *envelope.Keyring
}

var _ email.FieldCodec = EnvelopeCodec{}

func (c EnvelopeCodec) dek(ctx context.Context, tx pgx.Tx) (*envelope.DEK, error) {
	if c.Keys == nil {
		return nil, ErrNoKeyring
	}
	tenant, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return nil, tenantdb.ErrNoTenant
	}
	return c.Keys.Current(ctx, tx, tenant)
}

func envelopeField(f email.FieldRef) envelope.Field {
	return envelope.Field{Table: f.Table, Column: f.Column + "_enc", RowID: f.RowID}
}

// Seal implements email.FieldCodec.
func (c EnvelopeCodec) Seal(ctx context.Context, tx pgx.Tx, f email.FieldRef, plaintext string) ([]byte, error) {
	dek, err := c.dek(ctx, tx)
	if err != nil {
		return nil, err
	}
	tenant, _ := tenantdb.TenantFromContext(ctx)
	return envelope.SealString(dek, tenant, envelopeField(f), plaintext)
}

// SealEmail implements email.FieldCodec: it seals the address and returns its
// blind index under the physical <col>_bidx scope.
func (c EnvelopeCodec) SealEmail(ctx context.Context, tx pgx.Tx, f email.FieldRef, addr string) (enc, bidx []byte, err error) {
	dek, err := c.dek(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	tenant, _ := tenantdb.TenantFromContext(ctx)
	enc, err = envelope.SealString(dek, tenant, envelopeField(f), addr)
	if err != nil {
		return nil, nil, err
	}
	bidx = dek.BlindIndex(f.Table, f.Column+"_bidx", envelope.NormalizeEmail(addr))
	return enc, bidx, nil
}

// Open implements email.FieldCodec.
func (c EnvelopeCodec) Open(ctx context.Context, tx pgx.Tx, f email.FieldRef, sealed []byte) (string, error) {
	if c.Keys == nil {
		return "", ErrNoKeyring
	}
	tenant, ok := tenantdb.TenantFromContext(ctx)
	if !ok {
		return "", tenantdb.ErrNoTenant
	}
	return c.Keys.OpenString(ctx, tx, tenant, envelopeField(f), sealed)
}
