package bookings

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/email"
	"github.com/klubhub/dj/api/internal/promoter/event"
	"github.com/klubhub/dj/api/internal/promoter/identity"
)

// DefaultTemplate is the built-in fallback used when the tenant has no
// default template row — organisations created after the 00017 seed have
// none until they save their own. It mirrors the seeded copy (SPEC §7:
// event name/date/city/venue, slot role/genres/fee, org identity, opt-out
// footer).
const (
	DefaultTemplateName    = "Default booking offer"
	DefaultTemplateSubject = "Booking offer: {{event_name}} on {{date}}"
	DefaultTemplateBody    = `Hi {{dj_name}},

we're {{org_name}} and we'd love to have you play {{event_name}}.

Event: {{event_name}}
Date: {{date}}
City: {{city}}
Venue: {{venue}}
Slot: {{slot_role}}
Genres: {{genres}}
Fee: {{fee}}
{{notes}}

If you're interested, just reply to this email — it lands directly in {{promoter_name}}'s inbox ({{promoter_email}}).

If you'd rather not hear from us, reply "stop" and we'll remove you.

{{org_name}}`
)

// ─── Templates ───────────────────────────────────────────────────────────────

// ListTemplates returns all non-deleted templates, default first.
func (s *Service) ListTemplates(ctx context.Context) ([]*Template, error) {
	var out []*Template
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tpls, err := listTemplates(ctx, tx, tenantOf(ctx))
		if err != nil {
			return err
		}
		for i := range tpls {
			out = append(out, &tpls[i])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateTemplate validates and stores a new template.
func (s *Service) CreateTemplate(ctx context.Context, in TemplateCreate) (*Template, error) {
	if err := validateTemplateCreate(&in); err != nil {
		return nil, err
	}
	var out *Template
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tpl, err := createTemplate(ctx, tx, tenantOf(ctx), &in)
		if err != nil {
			return err
		}
		out = &tpl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateTemplate validates and applies a pointer-patch update with
// optimistic concurrency on the version column.
func (s *Service) UpdateTemplate(ctx context.Context, id uuid.UUID, in TemplateUpdate) (*Template, error) {
	if err := validateTemplateUpdate(&in); err != nil {
		return nil, err
	}
	var out *Template
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tpl, err := updateTemplate(ctx, tx, tenantOf(ctx), id, &in)
		if err != nil {
			return err
		}
		out = &tpl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteTemplate soft-deletes a template.
func (s *Service) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	return s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		return deleteTemplate(ctx, tx, tenantOf(ctx), id)
	})
}

// resolveTemplate loads the requested template or the tenant's default. When
// no default row exists (orgs created after the 00017 seed), the built-in
// DefaultTemplate is used and the request row's template_id stays NULL.
func (s *Service) resolveTemplate(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, id *uuid.UUID) (Template, error) {
	if id != nil {
		return getTemplate(ctx, tx, tenant, *id)
	}
	tpl, err := getDefaultTemplate(ctx, tx, tenant)
	if err == nil {
		return tpl, nil
	}
	if err != ErrNotFound {
		return Template{}, err
	}
	return Template{Name: DefaultTemplateName, Subject: DefaultTemplateSubject, Body: DefaultTemplateBody}, nil
}

// ─── Send ────────────────────────────────────────────────────────────────────

// SendOffer sends one booking-offer email and logs it. The flow:
// validate → load contact (must have an email, not suppressed) → slot
// tenancy check (the slot must be the caller's org's; the route's
// talent.write is the only action gate) → resolve the member's ReplyTo → resolve
// template + render → race-free daily cap + insert the request row in ONE
// transaction → EmailService.Send → sync status onto the row.
//
// The read/render/cap/insert path runs in a single WithTenant transaction:
// the DEK lookup must share the transaction with the reads it protects (the
// audience pattern), and the advisory lock makes check-and-insert atomic.
func (s *Service) SendOffer(ctx context.Context, p authz.Principal, in *SendOfferInput) (*OfferRequest, error) {
	if s.emails == nil {
		return nil, ErrEmailUnavailable
	}
	// Provider attestation gate — fail-closed, checked before any work so a
	// blocked operator gets an actionable error instead of a send that would
	// breach their provider's terms.
	if !s.cfg.AllowsOutreach() {
		return nil, ErrOutreachNotPermitted
	}
	if err := validateSendOffer(in); err != nil {
		return nil, err
	}
	tenant := tenantOf(ctx)
	orgID, err := uuid.Parse(p.OrgID)
	if err != nil || orgID != tenant {
		return nil, ErrForbidden
	}

	var out *OfferRequest
	var memberEmail string
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		// 1. Load the contact; refuse email-less contacts (OFFER-10).
		row, err := getTalentContact(ctx, tx, tenant, in.TalentContactID)
		if err != nil {
			return err
		}
		contact, err := s.unsealContact(ctx, tx, row)
		if err != nil {
			return err
		}
		if !contact.HasEmail {
			return ErrNoContactEmail
		}
		// 2. Suppression check via the contact's blind index.
		suppressed, err := isSuppressedBidx(ctx, tx, tenant, row.emailBidx)
		if err != nil {
			return err
		}
		if suppressed {
			return ErrSuppressed
		}
		// 3. Slot tenancy: the slot must be the tenant's own. RLS makes a
		// cross-tenant slot invisible; ErrForbidden (not ErrNotFound) avoids
		// leaking its existence. There is no per-slot action check: the
		// route's talent.write is the only action gate.
		var slot *OpenSlot
		var ev *eventOffer
		if in.OpenSlotID != nil {
			slotRow, err := getOpenSlot(ctx, tx, tenant, *in.OpenSlotID)
			if err != nil {
				return ErrForbidden
			}
			evRow, err := getEventForOffer(ctx, tx, tenant, slotRow.EventID)
			if err != nil {
				return err
			}
			slot, ev = &slotRow, &evRow
		}
		// 4. ReplyTo = the sending member's email (OFFER-06 v2.1). Orgs have
		// no contact email (00007), so the member's address is where DJ
		// replies must land. Refused 422 when it cannot be resolved (e.g. a
		// device session, whose sub is not "local:<uuid>").
		userID := identity.UserIDFromSub(p.Sub)
		if userID == nil {
			return ErrNoReplyTo
		}
		emailEnc, nameEnc, err := getMemberSealed(ctx, tx, tenant, *userID)
		if err != nil {
			return ErrNoReplyTo
		}
		memberEmail, err = s.open(ctx, tx, "users", "email_enc", *userID, emailEnc)
		if err != nil || memberEmail == "" {
			return ErrNoReplyTo
		}
		memberName, err := s.open(ctx, tx, "users", "display_name_enc", *userID, nameEnc)
		if err != nil {
			return err
		}
		// 5. Org profile (name + timezone for the daily-cap window).
		orgName, orgTZ, err := getOrgProfile(ctx, tx, tenant)
		if err != nil {
			return err
		}
		// 6. Resolve the template and render BEFORE the cap insert so the
		// inserted row carries the final copy (the template is never mutated).
		tpl, err := s.resolveTemplate(ctx, tx, tenant, in.TemplateID)
		if err != nil {
			return err
		}
		subject, body := tpl.Subject, tpl.Body
		if in.Subject != nil {
			subject = *in.Subject
		}
		if in.Body != nil {
			body = *in.Body
		}
		promoterName := memberName
		if promoterName == "" {
			promoterName = s.cfg.FromName
		}
		vars := offerVariables(orgName, promoterName, memberEmail, contact.Name, ev, slot, s.now())
		subject = email.RenderSubject(subject, vars)
		body = email.Render(body, vars)
		// 7. Seal the request row's PII (recipient + rendered copy).
		rowID := uuid.New()
		toEnc, err := seal(dek, tenant, "outreach_requests", "to_email_enc", rowID, contact.Email)
		if err != nil {
			return err
		}
		subjectEnc, err := seal(dek, tenant, "outreach_requests", "subject_enc", rowID, subject)
		if err != nil {
			return err
		}
		bodyEnc, err := seal(dek, tenant, "outreach_requests", "body_enc", rowID, body)
		if err != nil {
			return err
		}
		offer := &offerRow{
			id:              rowID,
			talentContactID: &in.TalentContactID,
			toEmailEnc:      toEnc,
			toEmailBidx:     offerEmailIndex(dek, contact.Email),
			subjectEnc:      subjectEnc,
			bodyEnc:         bodyEnc,
		}
		if slot != nil {
			offer.eventID = &slot.EventID
			offer.openSlotID = &slot.ID
		}
		if tpl.ID != uuid.Nil {
			offer.templateID = &tpl.ID
		}
		// 8. Race-free per-tenant daily cap + insert in this transaction
		// (SPEC §9 v2.1): the window is local midnight in the ORG's timezone;
		// failed attempts count because the row is inserted before the send.
		since := email.LocalMidnightIn(s.now(), orgTZ)
		if err := insertOfferWithDailyCap(ctx, tx, tenant, offer, s.cfg.MaxPerDay, since); err != nil {
			return err
		}
		out = &OfferRequest{
			ID: rowID, EventID: offer.eventID, OpenSlotID: offer.openSlotID,
			TalentContactID: offer.talentContactID, TemplateID: offer.templateID,
			ToEmail: contact.Email, Subject: subject, Body: body,
			Status: "queued", CreatedAt: offer.createdAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 9. Send through the shared outbox (the tenant-scoped repository +
	// EnvelopeCodec seal the outbox row). Send delivers immediately and
	// returns the message with its final status — a delivery failure is
	// msg.Status == failed, not an error.
	msg, err := s.emails.Send(ctx, email.CreateEmailRequest{
		Kind:      EmailKindBookingOffer,
		OwnerType: OwnerTypeBookingOffer,
		OwnerID:   &out.ID,
		FromEmail: s.cfg.FromEmail,
		FromName:  s.cfg.FromName,
		ToEmail:   out.ToEmail,
		Subject:   out.Subject,
		Body:      out.Body,
		ReplyTo:   memberEmail,
	})
	if err != nil {
		// Enqueue/validation failure: no outbox row exists. Mark the request
		// failed so the log does not show it queued forever (the cap already
		// counted it — failed attempts count, SPEC §9 v2.1).
		_ = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			return updateOfferSent(ctx, tx, tenant, out.ID, nil, "failed", nil)
		})
		return nil, err
	}
	// 10. Sync the delivery status onto the row (a second scope: the send is
	// done, the log row is not part of the send transaction). Best-effort: the
	// mail is out, and failing the call here would invite a retry and a
	// duplicate offer; the outbox row (owner-linked) still carries the status.
	status := string(msg.Status)
	_ = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return updateOfferSent(ctx, tx, tenant, out.ID, &msg.ID, status, msg.SentAt)
	})
	out.EmailMessageID = &msg.ID
	out.Status = status
	out.SentAt = msg.SentAt
	out.EmailStatus = status
	if msg.Status == email.EmailStatusFailed {
		out.EmailError = msg.LastError
	}
	return out, nil
}

// ─── Offers (sent log) ───────────────────────────────────────────────────────

// ListOffers returns the sent log, newest first, unsealed, with the live
// delivery status joined from email_messages.
func (s *Service) ListOffers(ctx context.Context) ([]*OfferRequest, error) {
	var out []*OfferRequest
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		rows, err := listOffers(ctx, tx, tenantOf(ctx))
		if err != nil {
			return err
		}
		for _, q := range rows {
			o, err := s.unsealOffer(ctx, tx, q)
			if err != nil {
				return err
			}
			out = append(out, &o)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetOffer returns one sent-log row, unsealed, or ErrNotFound.
func (s *Service) GetOffer(ctx context.Context, id uuid.UUID) (*OfferRequest, error) {
	var out *OfferRequest
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		q, err := getOffer(ctx, tx, tenantOf(ctx), id)
		if err != nil {
			return err
		}
		o, err := s.unsealOffer(ctx, tx, q)
		if err != nil {
			return err
		}
		out = &o
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ─── Suppressions ────────────────────────────────────────────────────────────

// ListSuppressions returns all suppressions. The email itself is never
// stored — only its blind index (SPEC §9).
func (s *Service) ListSuppressions(ctx context.Context) ([]*Suppression, error) {
	var out []*Suppression
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		suppressions, err := listSuppressions(ctx, tx, tenantOf(ctx))
		if err != nil {
			return err
		}
		for i := range suppressions {
			out = append(out, &suppressions[i])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AddSuppression validates, normalizes and stores a suppression. This is the
// endpoint behind the sent log's one-click "Do not contact" — opt-out has an
// owner (SPEC §9).
func (s *Service) AddSuppression(ctx context.Context, in *SuppressionCreate) (*Suppression, error) {
	if err := validateSuppression(in); err != nil {
		return nil, err
	}
	var out *Suppression
	err := s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		tenant := tenantOf(ctx)
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		s, err := insertSuppression(ctx, tx, tenant, suppressionIndex(dek, in.Email), in.Reason)
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

// DeleteSuppression removes a suppression by id.
func (s *Service) DeleteSuppression(ctx context.Context, id uuid.UUID) error {
	return s.db.WithTenant(ctx, tenantOf(ctx), func(tx pgx.Tx) error {
		return deleteSuppression(ctx, tx, tenantOf(ctx), id)
	})
}

// ─── Render helpers (pure — unit-tested) ─────────────────────────────────────

// offerVenueName applies OFFER-09: the offer must never disclose a venue the
// event withholds from the public — {{venue}} renders empty for 'city_only'
// and for 'secret' before location_reveal_at.
func offerVenueName(locationMode string, revealAt *time.Time, venueName string, now time.Time) string {
	switch locationMode {
	case event.LocationCityOnly:
		return ""
	case event.LocationSecret:
		if revealAt == nil || now.Before(*revealAt) {
			return ""
		}
	}
	return venueName
}

// offerVariables builds the {{var}} map for one send (SPEC §7):
// org_*/promoter_* from the org profile + sender identity, dj_* from the
// talent contact, event_*/venue/date/city from the event, and
// genres/fee/slot_role/notes from the open slot (empty without one).
// promoter_phone is always empty — users has no phone column (00003).
func offerVariables(orgName, promoterName, promoterEmail, djName string, ev *eventOffer, slot *OpenSlot, now time.Time) map[string]string {
	vars := map[string]string{
		"org_name":       orgName,
		"promoter_name":  promoterName,
		"promoter_email": promoterEmail,
		"promoter_phone": "",
		"dj_name":        djName,
	}
	if ev != nil {
		vars["event_name"] = ev.title
		vars["date"] = formatEventDate(ev.startsAt, ev.timezone)
		vars["city"] = ev.city
		vars["venue"] = offerVenueName(ev.locationMode, ev.locationRevealAt, ev.venueName, now)
	}
	if slot != nil {
		vars["genres"] = strings.Join(slot.Genres, ", ")
		vars["fee"] = formatFee(slot)
		vars["slot_role"] = slot.Role
		vars["notes"] = slot.Notes
	}
	return vars
}

// formatEventDate renders the event's start in the event's own timezone.
func formatEventDate(startsAt time.Time, tzName string) string {
	loc, err := time.LoadLocation(tzName)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	return startsAt.In(loc).Format("2006-01-02 15:04 MST")
}
