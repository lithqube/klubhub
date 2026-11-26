package marketing_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/authz"
	"github.com/klubhub/dj/api/internal/platform/envelope"
	"github.com/klubhub/dj/api/internal/platform/tenantdb"
	"github.com/klubhub/dj/api/internal/promoter/marketing"
)

var trackRe = regexp.MustCompile(`https://promoter\.test(/api/v1/marketing/c/[0-9a-f-]+/[A-Za-z0-9_-]+)`)

// sentBody opens the one queued send body.
func (f campFixture) sentBody(t *testing.T) string {
	t.Helper()
	var body string
	if err := tenantdb.WithTenant(f.ctx, testDB.App, f.org, func(tx pgx.Tx) error {
		var id uuid.UUID
		var enc []byte
		if err := tx.QueryRow(f.ctx, `SELECT id, body_enc FROM marketing_sends`).Scan(&id, &enc); err != nil {
			return err
		}
		var err error
		body, err = f.keys.OpenString(f.ctx, tx, f.org, envelope.Field{Table: "marketing_sends", Column: "body_enc", RowID: id}, enc)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestClick_RecordsAndRedirects(t *testing.T) {
	f := newCampFixture(t, true)
	f.svc.WithClickTracking("https://promoter.test/")
	f.contact(t, "yes@example.com", "consent", "")
	id, err := f.svc.Create(f.ctx, marketing.CampaignInput{SegmentID: f.segment,
		Subject: "Fri", Body: "Tickets: https://tix.example/fri?a=1. Lineup https://club.example/lineup"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Send(f.ctx, id); err != nil {
		t.Fatal(err)
	}
	body := f.sentBody(t)
	paths := trackRe.FindAllStringSubmatch(body, -1)
	if len(paths) != 2 || strings.Contains(body, "tix.example") || !strings.Contains(body, paths[0][0]+". Lineup") {
		t.Fatalf("links not wrapped: %q", body)
	}

	r := chi.NewRouter()
	marketing.NewHandler(marketing.Checker{}).WithCampaigns(f.svc).Mount(r, (*authz.Engine)(nil), authz.NewRegistry(), nil)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	w := get(paths[0][1])
	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://tix.example/fri?a=1" {
		t.Fatalf("got %d → %q", w.Code, w.Header().Get("Location"))
	}
	if got := count(t, f.org, "marketing_clicks"); got != 1 {
		t.Fatalf("%d clicks, want 1", got)
	}

	// Tampered, truncated, garbage and foreign-tenant tokens: 404, nothing recorded.
	p := paths[1][1]
	last := p[len(p)-1:]
	flip := map[string]string{"A": "B"}[last]
	if flip == "" {
		flip = "A"
	}
	other := newOrg(t).String()
	for _, bad := range []string{
		p[:len(p)-1] + flip,
		p[:len(p)-4],
		"/api/v1/marketing/c/" + f.org.String() + "/not-a-token",
		strings.Replace(p, f.org.String(), other, 1),
		strings.Replace(p, f.org.String(), "nope", 1),
	} {
		if w := get(bad); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", bad, w.Code)
		}
	}
	if got := count(t, f.org, "marketing_clicks"); got != 1 {
		t.Fatalf("%d clicks after bad tokens, want 1", got)
	}
}

func TestSend_WithoutClickBaseLeavesLinks(t *testing.T) {
	f := newCampFixture(t, true)
	f.contact(t, "yes@example.com", "consent", "")
	id, _ := f.svc.Create(f.ctx, marketing.CampaignInput{SegmentID: f.segment, Subject: "Fri", Body: "https://tix.example/fri"})
	if _, err := f.svc.Send(f.ctx, id); err != nil {
		t.Fatal(err)
	}
	if got := f.sentBody(t); got != "https://tix.example/fri" {
		t.Fatalf("body %q, want unchanged", got)
	}
}
