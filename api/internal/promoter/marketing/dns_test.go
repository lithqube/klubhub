package marketing

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// stub answers TXT lookups from a map; a missing name is NXDOMAIN.
type stub map[string][]string

func (s stub) LookupTXT(_ context.Context, name string) ([]string, error) {
	if txt, ok := s[name]; ok {
		return txt, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

// hang never answers until the caller gives up — a dead resolver.
type hang struct{}

func (hang) LookupTXT(ctx context.Context, _ string) ([]string, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCheckKnownGoodDomainPassesAll(t *testing.T) {
	c := Checker{Resolver: stub{
		"example.org":               {"google-site-verification=x", "v=spf1 include:_spf.example.net -all"},
		"s1._domainkey.example.org": {"v=DKIM1; k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC"},
		"_dmarc.example.org":        {"v=DMARC1; p=quarantine; rua=mailto:d@example.org"},
	}, Timeout: time.Second}

	got := c.Check(context.Background(), "example.org", "s1")
	for _, r := range []Record{got.SPF, got.DKIM, got.DMARC} {
		if !r.Pass || r.Error != "" {
			t.Errorf("%s: want pass, got %+v", r.Name, r)
		}
	}
	if got.SPF.Value != "v=spf1 include:_spf.example.net -all" {
		t.Errorf("SPF value = %q", got.SPF.Value)
	}
}

func TestCheckReportsMissingAndInvalidRecords(t *testing.T) {
	c := Checker{Resolver: stub{
		"example.org":               {"v=spf1 -all", "v=spf1 ~all"}, // two SPF records is a permerror
		"s1._domainkey.example.org": {"v=DKIM1; p="},                // revoked key
	}, Timeout: time.Second}

	got := c.Check(context.Background(), "example.org", "s1")
	want := map[string]string{"spf": "multiple_records", "dkim": "invalid_record", "dmarc": "not_found"}
	for _, r := range []Record{got.SPF, got.DKIM, got.DMARC} {
		if r.Pass || r.Error != want[r.Type] {
			t.Errorf("%s: want error %q, got %+v", r.Type, want[r.Type], r)
		}
	}
}

func TestCheckDeadResolverTimesOut(t *testing.T) {
	c := Checker{Resolver: hang{}, Timeout: 50 * time.Millisecond}
	start := time.Now()
	got := c.Check(context.Background(), "example.org", "s1")
	if d := time.Since(start); d > time.Second {
		t.Fatalf("check took %s, want it bounded by the timeout", d)
	}
	for _, r := range []Record{got.SPF, got.DKIM, got.DMARC} {
		if r.Pass || r.Error != "timeout" {
			t.Errorf("%s: want timeout, got %+v", r.Type, r)
		}
	}
}

func TestHandlerUnresolvableDomainAnswersWithTimeout(t *testing.T) {
	h := NewHandler(Checker{Resolver: hang{}, Timeout: 50 * time.Millisecond})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.dnsStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/marketing/dns-status?domain=dead.example&selector=s1", nil))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request hung on a dead resolver")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Domain != "dead.example" || got.SPF.Error != "timeout" || got.DKIM.Error != "timeout" || got.DMARC.Error != "timeout" {
		t.Errorf("got %+v", got)
	}
}

func TestHandlerRejectsBadInput(t *testing.T) {
	h := NewHandler(Checker{Resolver: stub{}, Timeout: time.Second})
	for _, q := range []string{"", "?domain=localhost&selector=s1", "?domain=a..b&selector=s1", "?domain=example.org", "?domain=example.org&selector=a.b", "?domain=ex_ample.org&selector=s1"} {
		rec := httptest.NewRecorder()
		h.dnsStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/marketing/dns-status"+q, nil))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%q: status = %d, want 422", q, rec.Code)
		}
	}
}
