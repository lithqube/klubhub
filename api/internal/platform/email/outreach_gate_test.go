package email_test

import (
	"testing"

	"github.com/klubhub/dj/api/internal/platform/email"
)

// The gate is the fail-closed guard that stops the app from sending cold
// B2B outreach through a provider whose terms prohibit it (Plunk's ToS §5
// bans "one-time cold outreach" outright). The default must refuse: a
// deployment that never sets the attestation must not send.
func TestOutreachGate_FailsClosedByDefault(t *testing.T) {
	cases := []struct {
		name string
		gate *email.OutreachGate
		want bool
	}{
		{"nil gate (no config wired)", nil, false},
		{"zero value", &email.OutreachGate{}, false},
		{"explicit false", email.NewOutreachGate(false), false},
		{"explicit true (attested)", email.NewOutreachGate(true), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.gate.AllowsOutreach(); got != tc.want {
				t.Fatalf("AllowsOutreach() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A nil *OutreachGate must not panic when the config block is skipped (the
// receiver is a pointer, so the method is called on a nil interface value).
func TestOutreachGate_NilReceiverIsSafe(t *testing.T) {
	var g *email.OutreachGate
	if g.AllowsOutreach() {
		t.Fatal("nil gate must refuse outreach")
	}
}