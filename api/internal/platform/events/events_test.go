package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSubjectsFollowTheTaxonomy(t *testing.T) {
	tenant := uuid.New()
	s, err := TenantSubject(tenant, "guest", "checked_in")
	if err != nil || s != "tenant."+tenant.String()+".guest.checked_in" {
		t.Fatalf("%q %v", s, err)
	}
	for _, bad := range [][2]string{{"Guest", "x"}, {"guest", "checked-in"}, {"guest", ""}, {"g.x", "y"}, {"guest", ">"}} {
		if _, err := TenantSubject(tenant, bad[0], bad[1]); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	if _, err := TenantSubject(uuid.Nil, "guest", "x"); err == nil {
		t.Error("nil tenant accepted")
	}
}

func TestEventPayloadCarriesIdentifiersOnly(t *testing.T) {
	tenant, guest := uuid.New(), uuid.New()
	_, ev, err := New(tenant, "guest", "checked_in", map[string]uuid.UUID{"guest_id": guest}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(ev)
	var generic map[string]any
	_ = json.Unmarshal(b, &generic)
	for k := range generic {
		if !map[string]bool{"id": true, "type": true, "tenant_id": true, "occurred_at": true, "refs": true, "schema": true}[k] {
			t.Fatalf("unexpected payload field %q", k)
		}
	}
	if !strings.Contains(string(b), guest.String()) {
		t.Fatal("ref missing")
	}
	if _, _, err := New(tenant, "guest", "x", map[string]uuid.UUID{"Name": guest}, time.Now()); err == nil {
		t.Fatal("ref names must be lowercase identifiers")
	}
}
