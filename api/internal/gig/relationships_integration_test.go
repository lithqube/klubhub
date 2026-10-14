package gig_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/contact"
	"github.com/klubhub/dj/api/internal/gig"
	"github.com/klubhub/dj/api/internal/tracklist"
	"github.com/klubhub/dj/api/internal/venue"
)

// Reuses the existing TestMain's migrated Postgres and lifecycle.
func TestIntegration_GigRelationshipPayloads(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if testPool == nil {
		t.Skip("postgres unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vr := venue.NewRepository(testPool)
	cr := contact.NewRepository(testPool)
	tr := tracklist.NewRepository(testPool)
	svc := gig.NewService(gig.NewRepository(testPool), vr, cr, tr, nil)
	city := "Relations-" + uuid.NewString()
	g, err := svc.CreateGig(ctx, &gig.GigCreate{Date: time.Now().UTC(), Venue: "Legacy venue", City: city, PromoterName: "Legacy promoter", FeeCurrency: "EUR"})
	if err != nil {
		t.Fatal(err)
	}
	assertRelations := func(want int) {
		t.Helper()
		row, err := svc.GetGigWithRelations(ctx, g.ID)
		if err != nil {
			t.Fatal(err)
		}
		if row.LinkedVenues == nil || row.LinkedContacts == nil || row.LinkedTracklists == nil {
			t.Fatalf("nil relationships: %+v", row)
		}
		if len(row.LinkedVenues) != want || len(row.LinkedContacts) != want || len(row.LinkedTracklists) != want {
			t.Fatalf("wrong relations: %+v", row)
		}
		list, err := svc.ListGigsWithRelations(ctx, gig.GigFilter{City: &city})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || len(list[0].LinkedVenues) != want || len(list[0].LinkedContacts) != want || len(list[0].LinkedTracklists) != want {
			t.Fatalf("wrong list: %+v", list)
		}
		router := gig.NewHandler(svc, "test-secret", nil).Routes()
		for _, path := range []string{"/?city=" + city, "/" + g.ID.String()} {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != 200 {
				t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
			}
			var env map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			payload := env["data"]
			if path[1] == '?' {
				var list []json.RawMessage
				if err := json.Unmarshal(payload, &list); err != nil {
					t.Fatal(err)
				}
				if len(list) != 1 {
					t.Fatalf("list: %s", payload)
				}
				payload = list[0]
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(payload, &obj); err != nil {
				t.Fatal(err)
			}
			if string(obj["venue"]) != `"Legacy venue"` || string(obj["promoter_name"]) != `"Legacy promoter"` {
				t.Fatalf("legacy fields: %s", payload)
			}
			for _, key := range []string{"linkedVenues", "linkedContacts", "linkedTracklists"} {
				var rows []map[string]json.RawMessage
				if err := json.Unmarshal(obj[key], &rows); err != nil {
					t.Fatal(err)
				}
				if rows == nil || len(rows) != want {
					t.Fatalf("%s: %s", key, obj[key])
				}
			}
			for _, key := range []string{"LinkedVenues", "linked_venues", "venues"} {
				if _, ok := obj[key]; ok {
					t.Fatalf("unexpected key %s", key)
				}
			}
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+g.ID.String()+"/tracklists", nil))
		if rec.Code != 200 {
			t.Fatalf("tracklists: %d %s", rec.Code, rec.Body.String())
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		if rows == nil || len(rows) != want {
			t.Fatalf("tracklists: %s", rec.Body.String())
		}
	}
	assertRelations(0)
	v, err := vr.Create(ctx, &venue.VenueCreate{Name: "Reusable venue", City: "Full city", Country: "DE", TechContactEmail: "tech@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := cr.Create(ctx, &contact.ContactCreate{Name: "Reusable contact", Email: "promoter@example.com", Type: contact.ContactTypePromoter, PartyFields: contact.PartyFields{City: "Contact city"}})
	if err != nil {
		t.Fatal(err)
	}
	tl := &tracklist.Tracklist{ID: uuid.New(), Title: "Reusable set", SourceFormat: "rekordbox", Preset: "story", VisibleFields: `["title"]`, BgMode: "solid", BgValue: "#000000", MaxTracks: 20, TrackRangeStart: 1, TrackRangeEnd: 20, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := tr.Create(ctx, tl, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkVenue(ctx, g.ID, v.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkContact(ctx, g.ID, c.ID, "promoter"); err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkTracklist(ctx, g.ID, tl.ID); err != nil {
		t.Fatal(err)
	}
	assertRelations(1)
	row, err := svc.GetGigWithRelations(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.LinkedVenues[0].ID != v.ID || row.LinkedVenues[0].TechContactEmail != v.TechContactEmail || row.LinkedContacts[0].ID != c.ID || row.LinkedContacts[0].Email != c.Email || row.LinkedContacts[0].City != c.City || row.LinkedTracklists[0].ID != tl.ID || row.LinkedTracklists[0].SourceFormat != tl.SourceFormat {
		t.Fatalf("not full persisted records: %+v", row)
	}
	detail, err := svc.GetGigDetail(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Venues) != 1 || !detail.Venues[0].IsPrimary || len(detail.Contacts) != 1 || detail.Contacts[0].Role != "promoter" || len(detail.Tracklists) != 1 {
		t.Fatalf("legacy detail changed: %+v", detail)
	}
	for _, table := range []string{"venues", "contacts", "tracklists"} {
		if _, err := testPool.Exec(ctx, "UPDATE "+table+" SET deleted_at=now() WHERE id=$1", map[string]uuid.UUID{"venues": v.ID, "contacts": c.ID, "tracklists": tl.ID}[table]); err != nil {
			t.Fatal(err)
		}
	}
	assertRelations(0)
	if err := svc.DeleteGig(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetGigWithRelations(ctx, g.ID); !errors.Is(err, gig.ErrNotFound) {
		t.Fatalf("deleted gig err=%v", err)
	}
	if _, err := svc.Tracklists(ctx, g.ID); !errors.Is(err, gig.ErrNotFound) {
		t.Fatalf("deleted gig tracklists err=%v", err)
	}
	for _, path := range []string{"/" + g.ID.String(), "/" + g.ID.String() + "/tracklists"} {
		rec := httptest.NewRecorder()
		gig.NewHandler(svc, "test-secret", nil).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 404 {
			t.Fatalf("missing gig %s: %d", path, rec.Code)
		}
	}
	list, err := svc.ListGigsWithRelations(ctx, gig.GigFilter{City: &city})
	if err != nil {
		t.Fatal(err)
	}
	if list == nil || len(list) != 0 {
		t.Fatalf("empty list=%+v", list)
	}
}
