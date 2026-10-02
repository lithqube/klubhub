package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/finance"
	"github.com/klubhub/dj/api/internal/platform/config"
	apphttp "github.com/klubhub/dj/api/internal/platform/http"
	"github.com/rs/zerolog"
)

// Embed the interface for unused operations: any unexpected call panics.
type originEmailRepo struct {
	finance.EmailRepositoryIface
	message *finance.EmailMessage
	creates int
}

func (s *originEmailRepo) Create(_ context.Context, m *finance.EmailMessage) (*finance.EmailMessage, error) {
	s.creates++
	s.message = m
	return m, nil
}
func (s *originEmailRepo) GetByID(_ context.Context, id uuid.UUID) (*finance.EmailMessage, error) {
	if s.message != nil && s.message.ID == id {
		return s.message, nil
	}
	return nil, finance.ErrEmailNotFound
}
func (s *originEmailRepo) UpdateStatus(_ context.Context, _ uuid.UUID, status finance.EmailStatus, e string, sent *time.Time) (*finance.EmailMessage, error) {
	s.message.Status = status
	s.message.LastError = e
	s.message.SentAt = sent
	return s.message, nil
}

type originSender struct{ calls int }

func (s *originSender) Send(_ *finance.EmailMessage) error { s.calls++; return nil }

func originRouter(repo *originEmailRepo, sender *originSender) http.Handler {
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	email := finance.NewEmailHandler(finance.NewEmailService(repo, sender))
	mux := finance.NewMux(nil, nil, nil, nil, nil, nil, email, nil)
	return apphttp.NewRouter(&config.Config{CORSOrigin: "https://dj.example"}, nil, nil, zerolog.Nop(), noop, noop, noop, noop, noop, noop, noop, mux)
}

const originEmailBody = `{"kind":"invoice_issued","from_email":"sender@example.test","to_email":"recipient@example.test","subject":"local fixture","body":"no real email"}`

func TestDJUnsafeMediaEmailContract(t *testing.T) {
	for _, media := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=fixture", "", "application/json; broken", "application/json, text/plain"} {
		t.Run(media, func(t *testing.T) {
			repo, sender := &originEmailRepo{}, &originSender{}
			r := httptest.NewRequest(http.MethodPost, "http://dj.example/api/v1/finance/emails", strings.NewReader(originEmailBody))
			r.Header.Set("Origin", "http://dj.example")
			if media != "" {
				r.Header.Set("Content-Type", media)
			}
			rec := httptest.NewRecorder()
			originRouter(repo, sender).ServeHTTP(rec, r)
			if rec.Code != http.StatusUnsupportedMediaType || repo.creates != 0 || sender.calls != 0 {
				t.Fatalf("status=%d created=%d sent=%d body=%s", rec.Code, repo.creates, sender.calls, rec.Body.String())
			}
		})
	}
}

func TestDJUnsafeOriginEmailContract(t *testing.T) {
	repo, sender := &originEmailRepo{}, &originSender{}
	router := originRouter(repo, sender)
	r := httptest.NewRequest(http.MethodPost, "http://dj.example/api/v1/finance/emails", strings.NewReader(originEmailBody))
	r.Header.Set("Origin", "https://hostile.example")
	r.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("hostile simple POST status=%d body=%s", rec.Code, rec.Body.String())
	}
	if repo.creates != 0 || sender.calls != 0 {
		t.Fatalf("unsafe request created=%d sent=%d", repo.creates, sender.calls)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env["error"] == nil {
		t.Fatalf("missing error envelope: %s", rec.Body.String())
	}
}
