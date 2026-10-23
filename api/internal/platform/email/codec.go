package email

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FieldRef identifies one sealed column of one row. The promoter-side
// envelope-backed codec maps it to envelope.Field{Table, Column, RowID}.
type FieldRef struct {
	Table  string
	Column string
	RowID  uuid.UUID
}

// FieldCodec seals and unseals outbox PII columns. The DJ side uses
// IdentityCodec (plaintext passthrough); the promoter side wires an
// envelope-backed implementation so talent PII is never stored in clear.
type FieldCodec interface {
	// Seal encrypts a plaintext value for storage.
	Seal(ctx context.Context, tx pgx.Tx, f FieldRef, plaintext string) ([]byte, error)
	// SealEmail seals an email address and also returns its blind index.
	SealEmail(ctx context.Context, tx pgx.Tx, f FieldRef, email string) (enc, bidx []byte, err error)
	// Open decrypts a sealed value.
	Open(ctx context.Context, tx pgx.Tx, f FieldRef, sealed []byte) (string, error)
}

// FieldSealer is the callback the tenant-scoped repository uses to seal and
// unseal message fields inside its WithTenant transaction scope, so an
// envelope-backed codec resolves the tenant DEK from the same transaction as
// the insert/read (audience pattern). *EmailService implements it; the DJ side
// never wires one (plaintext columns, IdentityCodec).
type FieldSealer interface {
	// SealMessageFields seals the outbox PII fields of msg in place and
	// returns the blind index for to_email (stored in to_email_bidx).
	SealMessageFields(ctx context.Context, tx pgx.Tx, msg *EmailMessage) (toBidx []byte, err error)
	// UnsealMessageFields reverses SealMessageFields.
	UnsealMessageFields(ctx context.Context, tx pgx.Tx, msg *EmailMessage) error
}

// IdentityCodec is the passthrough codec: Seal returns the plaintext bytes,
// Open returns them as a string. Used on the DJ side where the outbox columns
// are plaintext (single-user OSS, no tenant DEK).
type IdentityCodec struct{}

// Seal implements FieldCodec.
func (IdentityCodec) Seal(_ context.Context, _ pgx.Tx, _ FieldRef, plaintext string) ([]byte, error) {
	return []byte(plaintext), nil
}

// SealEmail implements FieldCodec.
func (IdentityCodec) SealEmail(_ context.Context, _ pgx.Tx, _ FieldRef, email string) ([]byte, []byte, error) {
	return []byte(email), nil, nil
}

// Open implements FieldCodec.
func (IdentityCodec) Open(_ context.Context, _ pgx.Tx, _ FieldRef, sealed []byte) (string, error) {
	return string(sealed), nil
}
