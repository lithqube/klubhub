package gig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/contact"
	"github.com/klubhub/dj/api/internal/tracklist"
	"github.com/klubhub/dj/api/internal/venue"
)

type relationshipContractService struct {
	stubServiceIface
	gig        *Gig
	tracklists []*tracklist.Tracklist
	err        error
}

func (s relationshipContractService) GetGig(context.Context, uuid.UUID) (*Gig, error) {
	return s.gig, s.err
}
func (s relationshipContractService) ListGigs(context.Context, GigFilter) ([]*Gig, error) {
	return []*Gig{s.gig}, s.err
}
func (s relationshipContractService) GetGigWithRelations(context.Context, uuid.UUID) (*GigWithRelations, error) {
	return &GigWithRelations{Gig: s.gig, LinkedVenues: []*venue.Venue{}, LinkedContacts: []*contact.Contact{}, LinkedTracklists: []*tracklist.Tracklist{}}, s.err
}
func (s relationshipContractService) ListGigsWithRelations(ctx context.Context, f GigFilter) ([]*GigWithRelations, error) {
	g, err := s.GetGigWithRelations(ctx, uuid.Nil)
	return []*GigWithRelations{g}, err
}
func (s relationshipContractService) Tracklists(context.Context, uuid.UUID) ([]*tracklist.Tracklist, error) {
	return s.tracklists, s.err
}

func TestGigRelationshipContractEmptyListAndGet(t *testing.T) {
	id := uuid.New()
	router := NewHandler(relationshipContractService{gig: &Gig{ID: id, EventName: "Legacy event"}}, "test-secret", nil).Routes()
	for _, path := range []string{"/", "/" + id.String()} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != 200 {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var env map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			payload := env["data"]
			if path == "/" {
				var list []json.RawMessage
				if err := json.Unmarshal(payload, &list); err != nil {
					t.Fatal(err)
				}
				if len(list) != 1 {
					t.Fatalf("list=%s", payload)
				}
				payload = list[0]
			}
			var row map[string]json.RawMessage
			if err := json.Unmarshal(payload, &row); err != nil {
				t.Fatal(err)
			}
			if string(row["event_name"]) != `"Legacy event"` {
				t.Fatalf("base snake_case lost: %s", payload)
			}
			for _, key := range []string{"linkedVenues", "linkedContacts", "linkedTracklists"} {
				if string(row[key]) != "[]" {
					t.Errorf("%s=%s; want []", key, row[key])
				}
			}
		})
	}
}

func TestGigTracklistsContract(t *testing.T) {
	id := uuid.New()
	tl := &tracklist.Tracklist{ID: uuid.New(), Title: "Set", VisibleFields: `[]`}
	for _, tc := range []struct {
		name, path string
		rows       []*tracklist.Tracklist
		err        error
		status     int
	}{
		{"populated", "/" + id.String() + "/tracklists", []*tracklist.Tracklist{tl}, nil, 200},
		{"empty", "/" + id.String() + "/tracklists", nil, nil, 200},
		{"missing", "/" + id.String() + "/tracklists", nil, ErrNotFound, 404},
		{"invalid", "/bad-id/tracklists", nil, nil, 400},
		{"failure", "/" + id.String() + "/tracklists", nil, errors.New("database failed"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewHandler(relationshipContractService{tracklists: tc.rows, err: tc.err}, "test-secret", nil).Routes()
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.status == 200 {
				var rows []*tracklist.Tracklist
				if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
					t.Fatal(err)
				}
				if rows == nil || len(rows) != len(tc.rows) {
					t.Fatalf("rows=%s", rec.Body.String())
				}
				if len(rows) > 0 && rows[0].ID != tl.ID {
					t.Fatalf("wrong tracklist: %+v", rows[0])
				}
			}
		})
	}
}
