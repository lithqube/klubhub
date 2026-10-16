package bookings

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Open slots are unfilled positions on the tenant's events (SPEC §6.1) —
// separate from lineup_entries, which are *billed* acts. CRUD is scoped to
// the tenant's events: RLS makes a cross-tenant event invisible, so the FK on
// event_id fails and the insert answers ErrNotFound (identical for "does not
// exist" and "not yours" — no existence leak).

// ListOpenSlots returns the slots of one event.
func (s *Service) ListOpenSlots(ctx context.Context, eventID uuid.UUID) ([]*OpenSlot, error) {
	var out []*OpenSlot
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		slots, err := listOpenSlots(ctx, tx, tenantOf(ctx), eventID)
		if err != nil {
			return err
		}
		for i := range slots {
			out = append(out, &slots[i])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateOpenSlot validates and stores a new slot on the tenant's event.
func (s *Service) CreateOpenSlot(ctx context.Context, eventID uuid.UUID, in OpenSlotInput) (*OpenSlot, error) {
	if err := validateOpenSlot(&in); err != nil {
		return nil, err
	}
	var out *OpenSlot
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		slot, err := insertOpenSlot(ctx, tx, tenantOf(ctx), eventID, &in)
		if err != nil {
			return err
		}
		out = &slot
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteOpenSlot removes a slot. Sent-log rows keep their history via
// ON DELETE SET NULL.
func (s *Service) DeleteOpenSlot(ctx context.Context, slotID uuid.UUID) error {
	return s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		return deleteOpenSlot(ctx, tx, tenantOf(ctx), slotID)
	})
}

// MatchSlot returns the talent contacts matching one open slot (SPEC §8):
// the event's city (case-insensitive, trimmed); genre overlap required only
// when BOTH the event and the contact carry genres; suppressed addresses
// excluded by blind index; email-less contacts included with
// has_email=false (OFFER-10). Ranking: genre-overlap count desc, then most
// recently added.
func (s *Service) MatchSlot(ctx context.Context, slotID uuid.UUID) ([]*TalentMatch, error) {
	var out []*TalentMatch
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		slot, err := getOpenSlot(ctx, tx, tenant, slotID)
		if err != nil {
			return err
		}
		ev, err := getEventForOffer(ctx, tx, tenant, slot.EventID)
		if err != nil {
			return err
		}
		rows, err := matchTalentForSlot(ctx, tx, tenant, ev.genres, ev.city)
		if err != nil {
			return err
		}
		for _, r := range rows {
			c, err := s.unsealContact(ctx, tx, r.talentRow)
			if err != nil {
				return err
			}
			overlap := r.overlapGenres
			if overlap == nil {
				overlap = []string{}
			}
			out = append(out, &TalentMatch{Contact: c, OverlapGenres: overlap})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
