package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/finance"
	"github.com/klubhub/dj/api/internal/platform/config"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	"github.com/rs/zerolog"
)

// Invalid JSON deliberately stops at each real handler's decoder, before an
// unconfigured service can run. The successful email matrix separately drives
// a real service with captured repository/delivery effects and HTTP readback.
func TestDJFinanceRegisteredRoutesOriginContract(t *testing.T) {
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux := finance.NewMux(finance.NewHandler(nil), finance.NewInvoiceHandler(nil), finance.NewPaymentHandler(nil), nil, finance.NewAgreementTemplateHandler(nil), finance.NewAgreementInstanceHandler(nil), finance.NewEmailHandler(nil), finance.NewEntryHandler(nil))
	router := apphttp.NewRouter(&config.Config{CORSOrigin: "https://dj.example"}, nil, nil, zerolog.Nop(), noop, noop, noop, noop, noop, noop, noop, mux)
	const id = "11111111-1111-1111-1111-111111111111"
	for _, route := range []struct{ method, path string }{
		{"PUT", "billing-profile"}, {"POST", "invoices"}, {"POST", "invoices/" + id + "/payments"}, {"PUT", "payments/" + id},
		{"POST", "agreements/templates"}, {"POST", "agreements/instances"}, {"POST", "agreements/instances/" + id + "/sign"}, {"POST", "emails"}, {"POST", "entries"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			for _, origin := range []string{"https://hostile.example", "http://dj.example", ""} {
				r := httptest.NewRequest(route.method, "http://dj.example/api/v1/finance/"+route.path, strings.NewReader("{"))
				r.Header.Set("Content-Type", "application/json")
				if origin != "" {
					r.Header.Set("Origin", origin)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, r)
				want := 400
				if origin == "https://hostile.example" {
					want = 403
				}
				if rec.Code != want {
					t.Fatalf("origin=%q status=%d want=%d body=%s", origin, rec.Code, want, rec.Body.String())
				}
				if want == 400 && !strings.Contains(rec.Body.String(), "invalid JSON") {
					t.Fatalf("did not reach real decoder: %s", rec.Body.String())
				}
			}
		})
	}
}

func TestDJRouterPreservesBodyLimitsContract(t *testing.T) {
	repo, sender := &originEmailRepo{}, &originSender{}
	router := originRouter(repo, sender)
	r := httptest.NewRequest("POST", "http://dj.example/api/v1/finance/emails", strings.NewReader(originEmailBody))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = apphttp.MaxBodyBytes + 1
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	if rec.Code != 413 || repo.creates != 0 || sender.calls != 0 {
		t.Fatalf("outer cap status=%d created=%d sent=%d", rec.Code, repo.creates, sender.calls)
	}
	// Preserve the email handler's existing tighter 1 MiB cap and error envelope.
	body := strings.Replace(originEmailBody, "no real email", strings.Repeat("x", (1<<20)+1), 1)
	r = httptest.NewRequest("POST", "http://dj.example/api/v1/finance/emails", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "invalid JSON body") || repo.creates != 0 || sender.calls != 0 {
		t.Fatalf("inner cap status=%d body=%s", rec.Code, rec.Body.String())
	}
}
