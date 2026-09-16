package gig

import "testing"

// Plan B.5 / v1.0.0 — additional regression tests for validateICalSecret
// covering edge cases the B.5 base tests didn't reach.

func TestValidateICalSecret_SingleByteDifference(t *testing.T) {
	h := newHandlerWithSecret(t)
	tampered := testICalSecret[:len(testICalSecret)-1] + "X"
	if h.validateICalSecret(tampered) {
		t.Fatalf("validateICalSecret accepted a single-byte tampering: %q", tampered)
	}
}

func TestValidateICalSecret_WhitespacePaddedFails(t *testing.T) {
	h := newHandlerWithSecret(t)
	padded := testICalSecret + " "
	if h.validateICalSecret(padded) {
		t.Fatal("validateICalSecret accepted a whitespace-padded secret")
	}
}

func TestValidateICalSecret_UnicodeFails(t *testing.T) {
	h := newHandlerWithSecret(t)
	if h.validateICalSecret(testICalSecret + "ä") {
		t.Fatal("validateICalSecret accepted a unicode-tampered secret")
	}
}

func TestValidateICalSecret_NewlineInjectionFails(t *testing.T) {
	h := newHandlerWithSecret(t)
	if h.validateICalSecret(testICalSecret + "\r\nX-Injected: yes") {
		t.Fatal("validateICalSecret accepted a CRLF-injected secret")
	}
}

func TestValidateICalSecret_ExtremeLengthsRejected(t *testing.T) {
	h := newHandlerWithSecret(t)
	garbage := make([]byte, 1024)
	for i := range garbage {
		garbage[i] = 'A'
	}
	if h.validateICalSecret(string(garbage)) {
		t.Fatal("validateICalSecret accepted 1KiB of arbitrary bytes")
	}
}

func TestEscapeICalText_AllRFC5545Reserveds(t *testing.T) {
	// RFC 5545 §3.3.11 reserved characters in TEXT values.
	cases := map[string]string{
		`Hello`:          `Hello`,
		`Hello;World`:    `Hello\;World`,
		`Hello,World`:    `Hello\,World`,
		`Hello\World`:    `Hello\\World`,
		"Hello\nWorld":   `Hello\nWorld`,
		`Hello;World,Ok`: `Hello\;World\,Ok`,
		``:               ``,
	}
	for in, want := range cases {
		if got := escapeICalText(in); got != want {
			t.Errorf("escapeICalText(%q) = %q, want %q", in, got, want)
		}
	}
}
