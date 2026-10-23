package bookings

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Talent contacts are the DJs a promoter knows (SPEC §6.1): B2B relationships
// (past gig, referral, manual) — deliberately NOT the consent-based
// audience_contacts (different purpose/lawful basis, SPEC §9). Name/email/
// phone are sealed with the tenant DEK and blind-indexed; city/country/genres
// are plaintext matching keys (SPEC §8).

// CreateTalentContact adds one contact. Dedupe: on email_bidx conflict →
// ErrDuplicate (the unique index enforces it). Email is optional — a contact
// may be name-only (OFFER-10): email_enc NULL, email_bidx NULL,
// has_email=false.
func (s *Service) CreateTalentContact(ctx context.Context, in TalentContactInput) (*TalentContact, error) {
	if err := validateTalentContact(&in); err != nil {
		return nil, err
	}
	var out TalentContact
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		id := uuid.New()
		nameEnc, err := seal(dek, tenant, "talent_contacts", "name_enc", id, in.Name)
		if err != nil {
			return err
		}
		emailEnc, err := seal(dek, tenant, "talent_contacts", "email_enc", id, in.Email)
		if err != nil {
			return err
		}
		phoneEnc, err := seal(dek, tenant, "talent_contacts", "phone_enc", id, in.Phone)
		if err != nil {
			return err
		}
		createdAt, updatedAt, err := insertTalentContact(ctx, tx, tenant, id,
			nameEnc, emailEnc, phoneEnc, talentNameIndex(dek, in.Name), talentEmailIndex(dek, in.Email), &in)
		if err != nil {
			return err
		}
		out = TalentContact{
			ID: id, Name: in.Name, Email: in.Email, Phone: in.Phone, HasEmail: in.Email != "",
			City: in.City, Country: in.Country, Genres: in.Genres, Source: in.Source, Notes: in.Notes,
			CreatedAt: createdAt, UpdatedAt: updatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTalentContacts returns the talent book matching filter, newest first.
// Every result carries HasEmail so the UI can show email-less contacts
// without offering to send (OFFER-10).
func (s *Service) ListTalentContacts(ctx context.Context, filter TalentFilter) ([]*TalentContact, error) {
	var out []*TalentContact
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		q := talentQuery{city: filter.City, genre: filter.Genre}
		if filter.Q != "" {
			dek, err := s.keys.Current(ctx, tx, tenant)
			if err != nil {
				return err
			}
			q.nameBidx = talentNameIndex(dek, filter.Q)
			q.emailBidx = talentEmailIndex(dek, filter.Q)
		}
		rows, err := listTalentContacts(ctx, tx, tenant, q)
		if err != nil {
			return err
		}
		for _, r := range rows {
			c, err := s.unsealContact(ctx, tx, r)
			if err != nil {
				return err
			}
			out = append(out, &c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetTalentContact loads one contact, unsealed.
func (s *Service) GetTalentContact(ctx context.Context, id uuid.UUID) (*TalentContact, error) {
	var out *TalentContact
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		r, err := getTalentContact(ctx, tx, tenantOf(ctx), id)
		if err != nil {
			return err
		}
		c, err := s.unsealContact(ctx, tx, r)
		if err != nil {
			return err
		}
		out = &c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateTalentContact replaces the editable fields of one contact (full
// replace, audience UpdateContact semantics).
func (s *Service) UpdateTalentContact(ctx context.Context, id uuid.UUID, in TalentContactInput) (*TalentContact, error) {
	if err := validateTalentContact(&in); err != nil {
		return nil, err
	}
	var out TalentContact
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		nameEnc, err := seal(dek, tenant, "talent_contacts", "name_enc", id, in.Name)
		if err != nil {
			return err
		}
		emailEnc, err := seal(dek, tenant, "talent_contacts", "email_enc", id, in.Email)
		if err != nil {
			return err
		}
		phoneEnc, err := seal(dek, tenant, "talent_contacts", "phone_enc", id, in.Phone)
		if err != nil {
			return err
		}
		updatedAt, err := updateTalentContact(ctx, tx, tenant, id,
			nameEnc, emailEnc, phoneEnc, talentNameIndex(dek, in.Name), talentEmailIndex(dek, in.Email), &in)
		if err != nil {
			return err
		}
		out = TalentContact{
			ID: id, Name: in.Name, Email: in.Email, Phone: in.Phone, HasEmail: in.Email != "",
			City: in.City, Country: in.Country, Genres: in.Genres, Source: in.Source, Notes: in.Notes,
			UpdatedAt: updatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTalentContact hard-deletes one contact. The sent log survives via
// ON DELETE SET NULL (00017).
func (s *Service) DeleteTalentContact(ctx context.Context, id uuid.UUID) error {
	return s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		return deleteTalentContact(ctx, tx, tenantOf(ctx), id)
	})
}

// SuppressTalentContact writes a do-not-contact suppression for the contact's
// email, copying the contact's stored blind index (the suppression scope is
// the contact email scope — see insertSuppression). A contact without an
// email cannot be suppressed (there is no address to refuse): 422.
func (s *Service) SuppressTalentContact(ctx context.Context, id uuid.UUID) (*Suppression, error) {
	var out *Suppression
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		r, err := getTalentContact(ctx, tx, tenant, id)
		if err != nil {
			return err
		}
		if r.emailBidx == nil {
			return ErrNoContactEmail
		}
		s, err := insertSuppression(ctx, tx, tenant, r.emailBidx, "marked do-not-contact")
		if err != nil {
			return err
		}
		out = &s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
