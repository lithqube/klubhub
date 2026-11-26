package marketing

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
)

// Click tracking (P3.2-8b). With a click base set, Send rewrites every
// http(s) link of the body to GET /api/v1/marketing/c/{tenantID}/{token}.
// The token names key version, campaign, send (the recipient) and link
// position, sealed with an HMAC under the tenant key: a tampered or foreign
// token records nothing and answers 404. The redirect target always comes
// from marketing_campaign_links, so the route is never an open redirect.

var linkRe = regexp.MustCompile(`https?://[^\s<>"']+`)

const (
	clickPayloadLen = 4 + 16 + 16 + 2
	clickMACLen     = 16
	maxTrackedLinks = 1 << 16
)

// WithClickTracking makes Send wrap campaign links under base, the public
// origin recipients reach the API on.
func (s *CampaignService) WithClickTracking(base string) *CampaignService {
	s.clickBase = strings.TrimRight(base, "/")
	return s
}

// splitLink drops sentence punctuation that the pattern swallows after a URL.
func splitLink(m string) (link, tail string) {
	link = strings.TrimRight(m, ".,;:!?)")
	return link, m[len(link):]
}

// campaignLinks returns the distinct links of body in order of appearance.
func campaignLinks(body string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range linkRe.FindAllString(body, -1) {
		if l, _ := splitLink(m); l != "" && !seen[l] && len(out) < maxTrackedLinks {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

func clickMAC(dek *envelope.DEK, payload []byte) []byte {
	return dek.BlindIndex("marketing_clicks", "token", string(payload))[:clickMACLen]
}

// wrapLinks renders body for one send with each known link replaced by its
// tracking URL.
func (s *CampaignService) wrapLinks(dek *envelope.DEK, tenant, campaign, send uuid.UUID, body string, links []string) string {
	pos := make(map[string]int, len(links))
	for i, l := range links {
		pos[l] = i
	}
	return linkRe.ReplaceAllStringFunc(body, func(m string) string {
		l, tail := splitLink(m)
		i, ok := pos[l]
		if !ok {
			return m
		}
		p := binary.BigEndian.AppendUint32(make([]byte, 0, clickPayloadLen+clickMACLen), dek.Version())
		p = append(append(p, campaign[:]...), send[:]...)
		p = binary.BigEndian.AppendUint16(p, uint16(i))
		p = append(p, clickMAC(dek, p)...)
		return s.clickBase + "/api/v1/marketing/c/" + tenant.String() + "/" + base64.RawURLEncoding.EncodeToString(p) + tail
	})
}

// Click verifies a tracking token, records the click and returns the
// original link. ErrNotFound for anything that does not verify.
func (s *CampaignService) Click(ctx context.Context, tenant uuid.UUID, token string) (target string, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != clickPayloadLen+clickMACLen {
		return "", ErrNotFound
	}
	p, mac := raw[:clickPayloadLen], raw[clickPayloadLen:]
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		dek, err := s.keys.Version(ctx, tx, tenant, binary.BigEndian.Uint32(p))
		if errors.Is(err, envelope.ErrNoKey) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if !hmac.Equal(mac, clickMAC(dek, p)) {
			return ErrNotFound
		}
		campaign, send, position := uuid.UUID(p[4:20]), uuid.UUID(p[20:36]), int(binary.BigEndian.Uint16(p[36:]))
		err = tx.QueryRow(ctx, `SELECT url FROM marketing_campaign_links WHERE tenant_id = $1 AND campaign_id = $2 AND position = $3`,
			tenant, campaign, position).Scan(&target)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		// An erased recipient's send is gone: still redirect, record nothing.
		_, err = tx.Exec(ctx, `INSERT INTO marketing_clicks (id, tenant_id, campaign_id, position, send_id, clicked_at)
			SELECT $1, $2, $3, $4, id, $6 FROM marketing_sends WHERE id = $5 AND tenant_id = $2`,
			uuid.Must(uuid.NewV7()), tenant, campaign, position, send, s.now())
		return err
	})
	return target, err
}

func (h *Handler) mountClicks(r chi.Router, e *authz.Engine, reg *authz.Registry, onDeny authz.Denied) {
	// Public: the recipient's mail client holds no session; the token is the credential.
	e.Handle(r, reg, authz.Route{Method: http.MethodGet, Pattern: "/api/v1/marketing/c/{tenantID}/{token}", Public: true}, h.click, onDeny)
}

func (h *Handler) click(w http.ResponseWriter, r *http.Request) {
	tenant, err := uuid.Parse(chi.URLParam(r, "tenantID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	target, err := h.campaigns.Click(r.Context(), tenant, chi.URLParam(r, "token"))
	switch {
	case errors.Is(err, ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal")
	default:
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, target, http.StatusFound)
	}
}
