package marketing_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/promoter/marketing"
)

func TestGate_FailsClosedByDefault(t *testing.T) {
	cases := []struct {
		name string
		gate *marketing.Gate
		want bool
	}{
		{"nil gate", nil, false},
		{"zero value", &marketing.Gate{}, false},
		{"explicit false", marketing.NewGate(false), false},
		{"attested", marketing.NewGate(true), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.gate.AllowsMarketing(); got != tc.want {
				t.Fatalf("AllowsMarketing() = %v, want %v", got, tc.want)
			}
		})
	}
}

type fakeProvider struct{ sent int }

func (f *fakeProvider) SendMarketing(context.Context, *marketing.Message) (string, error) {
	f.sent++
	return "id-1", nil
}
func (f *fakeProvider) HandleWebhook(*http.Request) ([]marketing.WebhookEvent, error) {
	return []marketing.WebhookEvent{{Type: marketing.EventBounce}}, nil
}

func TestGated_RefusesSendWithoutAttestation(t *testing.T) {
	fp := &fakeProvider{}
	p := marketing.Gated(marketing.NewGate(false), fp)
	if _, err := p.SendMarketing(context.Background(), &marketing.Message{}); !errors.Is(err, marketing.ErrNotPermitted) {
		t.Fatalf("err = %v, want ErrNotPermitted", err)
	}
	if fp.sent != 0 {
		t.Fatal("provider must not be called when gate refuses")
	}
	// Webhooks still flow so suppressions are recorded.
	if evs, err := p.HandleWebhook(httptest.NewRequest(http.MethodPost, "/", nil)); err != nil || len(evs) != 1 {
		t.Fatalf("webhook = %v, %v", evs, err)
	}
}

func TestGated_SendsWhenAttested(t *testing.T) {
	fp := &fakeProvider{}
	p := marketing.Gated(marketing.NewGate(true), fp)
	id, err := p.SendMarketing(context.Background(), &marketing.Message{})
	if err != nil || id != "id-1" || fp.sent != 1 {
		t.Fatalf("id=%q err=%v sent=%d", id, err, fp.sent)
	}
}

func TestGate_Require(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) })
	for _, tc := range []struct {
		gate *marketing.Gate
		code int
	}{{nil, 422}, {marketing.NewGate(false), 422}, {marketing.NewGate(true), 202}} {
		rec := httptest.NewRecorder()
		tc.gate.Require(next).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/send", nil))
		if rec.Code != tc.code {
			t.Fatalf("code = %d, want %d", rec.Code, tc.code)
		}
		if tc.code == 422 && !strings.Contains(rec.Body.String(), "marketing_not_permitted") {
			t.Fatalf("body = %q", rec.Body.String())
		}
	}
}
