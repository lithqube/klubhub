package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/events"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/audience"
)

var (
	ErrInvalid     = errors.New("marketing: invalid campaign")
	ErrNotFound    = errors.New("marketing: not found")
	ErrAlreadySent = errors.New("marketing: campaign already sent")
)

// CampaignInput is one campaign: content aimed at a saved audience segment.
type CampaignInput struct {
	SegmentID uuid.UUID `json:"segment_id"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
}

// CampaignService creates campaigns and fans a send out into marketing_sends.
type CampaignService struct {
	db   *tenantdb.DB
	keys *envelope.Keyring
	gate *Gate
	now  func() time.Time
	// clickBase is the public origin for tracking links; empty sends links unwrapped.
	clickBase string
}

// NewCampaignService builds the service; now defaults to time.Now.
func NewCampaignService(db *tenantdb.DB, keys *envelope.Keyring, gate *Gate, now func() time.Time) *CampaignService {
	if now == nil {
		now = time.Now
	}
	return &CampaignService{db: db, keys: keys, gate: gate, now: now}
}

func tenantOf(ctx context.Context) uuid.UUID {
	id, _ := tenantdb.TenantFromContext(ctx)
	return id
}

// Create stores a draft aimed at an existing segment.
func (s *CampaignService) Create(ctx context.Context, in CampaignInput) (uuid.UUID, error) {
	subject, body := strings.TrimSpace(in.Subject), strings.TrimSpace(in.Body)
	if subject == "" || len(subject) > 200 || body == "" || len(body) > 200000 {
		return uuid.Nil, ErrInvalid
	}
	tenant, id := tenantOf(ctx), uuid.Must(uuid.NewV7())
	err := s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		// Inserting from the segment row also proves it belongs to this tenant.
		tag, err := tx.Exec(ctx, `INSERT INTO marketing_campaigns (id, tenant_id, segment_id, subject, body)
			SELECT $1, $2, id, $4, $5 FROM audience_segments WHERE id = $3 AND tenant_id = $2`,
			id, tenant, in.SegmentID, subject, body)
		if err == nil && tag.RowsAffected() == 0 {
			err = ErrInvalid
		}
		return err
	})
	return id, err
}

// Send enqueues the campaign for every active, consented, non-suppressed
// contact in its segment and returns how many sends it queued. Refused
// outright unless the operator attested the provider permits marketing.
func (s *CampaignService) Send(ctx context.Context, id uuid.UUID) (queued int, err error) {
	if !s.gate.AllowsMarketing() {
		return 0, ErrNotPermitted
	}
	tenant := tenantOf(ctx)
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var subject, body, status string
		var f audience.SegmentFilter
		err := tx.QueryRow(ctx, `SELECT c.subject, c.body, c.status, s.filter FROM marketing_campaigns c
			JOIN audience_segments s ON s.id = c.segment_id
			WHERE c.id = $1 AND c.tenant_id = $2 FOR UPDATE OF c`, id, tenant).Scan(&subject, &body, &status, &f)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if status != "draft" {
			return ErrAlreadySent
		}
		// Recipients: active + lawful basis "consent" + not on the suppression list.
		q := `SELECT c.id, c.email_enc FROM audience_contacts c
			WHERE c.tenant_id = $1 AND c.status = 'active' AND c.consent_basis = 'consent' AND c.email_enc IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM marketing_suppressions s WHERE s.tenant_id = c.tenant_id AND s.email_bidx = c.email_bidx)`
		args := []any{tenant}
		for col, val := range map[string]string{"status": f.Status, "source": f.Source} {
			if val != "" {
				args = append(args, val)
				q += ` AND c.` + col + ` = $` + strconv.Itoa(len(args))
			}
		}
		if f.SinceDays != nil {
			args = append(args, s.now().AddDate(0, 0, -*f.SinceDays))
			q += ` AND c.created_at >= $` + strconv.Itoa(len(args))
		}
		type recipient struct {
			ID  uuid.UUID
			Enc []byte
		}
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		rs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[recipient])
		if err != nil {
			return err
		}
		dek, err := s.keys.Current(ctx, tx, tenant)
		if err != nil {
			return err
		}
		var links []string
		if s.clickBase != "" {
			links = campaignLinks(body)
		}
		for i, l := range links {
			if _, err := tx.Exec(ctx, `INSERT INTO marketing_campaign_links (tenant_id, campaign_id, position, url) VALUES ($1, $2, $3, $4)`,
				tenant, id, i, l); err != nil {
				return err
			}
		}
		for _, r := range rs {
			email, err := s.keys.OpenString(ctx, tx, tenant, envelope.Field{Table: "audience_contacts", Column: "email_enc", RowID: r.ID}, r.Enc)
			if err != nil {
				return err
			}
			sid := uuid.Must(uuid.NewV7())
			rendered := body
			if len(links) > 0 {
				rendered = s.wrapLinks(dek, tenant, id, sid, body, links)
			}
			var enc [3][]byte
			for i, v := range []struct{ col, val string }{{"to_email_enc", email}, {"subject_enc", subject}, {"body_enc", rendered}} {
				if enc[i], err = envelope.SealString(dek, tenant, envelope.Field{Table: "marketing_sends", Column: v.col, RowID: sid}, v.val); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO marketing_sends (id, tenant_id, contact_id, to_email_enc, to_email_bidx, subject_enc, body_enc)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`, sid, tenant, r.ID, enc[0], EmailIndex(dek, email), enc[1], enc[2]); err != nil {
				return err
			}
			queued++
		}
		if _, err := tx.Exec(ctx, `UPDATE marketing_campaigns SET status = 'sent', sent_at = $3 WHERE id = $1 AND tenant_id = $2`,
			id, tenant, s.now()); err != nil {
			return err
		}
		subj, ev, err := events.New(tenant, "marketing", "campaign_sent", map[string]uuid.UUID{"campaign_id": id}, s.now())
		if err != nil {
			return err
		}
		return events.Enqueue(ctx, tx, subj, ev)
	})
	return queued, err
}

// WithCampaigns enables the campaign routes.
func (h *Handler) WithCampaigns(s *CampaignService) *Handler {
	h.campaigns = s
	return h
}

func (h *Handler) mountCampaigns(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	if h.campaigns == nil {
		return
	}
	route := func(pattern, action string, fn http.HandlerFunc) {
		e.Handle(r, reg, authz.Route{Method: http.MethodPost, Pattern: pattern, Action: action, ResourceType: "campaign"}, fn, onDeny)
	}
	route("/api/v1/marketing/campaigns", "campaign.write", h.createCampaign)
	route("/api/v1/marketing/campaigns/{campaignID}/send", "campaign.send", h.sendCampaign)
	h.mountClicks(r, e, reg, onDeny)
}

func (h *Handler) createCampaign(w http.ResponseWriter, r *http.Request) {
	var in CampaignInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil {
		writeErr(w, http.StatusBadRequest, "invalid_body")
		return
	}
	id, err := h.campaigns.Create(r.Context(), in)
	if err != nil {
		campaignErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "draft"})
}

func (h *Handler) sendCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	n, err := h.campaigns.Send(r.Context(), id)
	if err != nil {
		campaignErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]int{"queued": n})
}

func campaignErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotPermitted):
		writeErr(w, http.StatusUnprocessableEntity, "marketing_not_permitted")
	case errors.Is(err, ErrInvalid):
		writeErr(w, http.StatusUnprocessableEntity, "invalid")
	case errors.Is(err, ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found")
	case errors.Is(err, ErrAlreadySent):
		writeErr(w, http.StatusConflict, "already_sent")
	default:
		writeErr(w, http.StatusInternalServerError, "internal")
	}
}
