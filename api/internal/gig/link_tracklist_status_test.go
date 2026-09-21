package gig

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/tracklist"
)

// linkErrService returns a fixed error from LinkTracklist.
type linkErrService struct {
	mockGigServiceForTest
	err error
}

func (m *linkErrService) LinkTracklist(ctx context.Context, gigID, tracklistID uuid.UUID) error {
	return m.err
}

func TestHandleLinkTracklist_ErrorStatuses(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"linked", nil, http.StatusNoContent},
		{"unknown gig", ErrNotFound, http.StatusNotFound},
		{"unknown tracklist", tracklist.ErrNotFound, http.StatusNotFound},
		{"wrapped gig not found", errors.Join(errors.New("link"), ErrNotFound), http.StatusNotFound},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(&linkErrService{err: tt.err}, "test-secret-value-0123456789abcdef")
			url := "/" + uuid.NewString() + "/tracklists/" + uuid.NewString()
			req := httptest.NewRequest(http.MethodPost, url, nil)
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}
