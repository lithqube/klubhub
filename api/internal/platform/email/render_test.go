package email

import (
	"strings"
	"testing"
)

func TestRender_SubstitutesKnownKeys(t *testing.T) {
	got := Render("Hello {{name}}, you are booked in {{city}} on {{date}}.", map[string]string{
		"name": "Lina",
		"city": "Berlin",
		"date": "2026-11-01",
	})
	want := "Hello Lina, you are booked in Berlin on 2026-11-01."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_UnknownKeyRendersEmpty(t *testing.T) {
	got := Render("Hi {{name}} ({{missing}})", map[string]string{"name": "Bo"})
	if want := "Hi Bo ()"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_NoVarsIsIdentity(t *testing.T) {
	text := "Hello {{name}}\nsecond line"
	if got := Render(text, nil); got != text {
		t.Errorf("got %q, want %q", got, text)
	}
	if got := Render(text, map[string]string{}); got != text {
		t.Errorf("got %q, want %q", got, text)
	}
}

// TestRender_StripsCRLFFromValues is the SPEC §7 v2.1 header-injection
// defense: substituted values are untrusted (contact names, notes), so CR/LF
// is flattened before replacement — a value like "a\r\nBcc: x@y" can never
// smuggle an extra header into a subject built by Render.
func TestRender_StripsCRLFFromValues(t *testing.T) {
	got := Render("Subject: {{subject}}", map[string]string{
		"subject": "hi\r\nBcc: attacker@evil.com",
	})
	if want := "Subject: hi  Bcc: attacker@evil.com"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("rendered output still contains CR/LF: %q", got)
	}

	// Bare LF and bare CR are flattened too.
	if got := Render("{{v}}", map[string]string{"v": "a\nb\rc"}); got != "a b c" {
		t.Errorf("got %q", got)
	}
}

// TestRender_TemplateNewlinesUntouched verifies only the substituted values
// are sanitised; the template's own line breaks survive.
func TestRender_TemplateNewlinesUntouched(t *testing.T) {
	got := Render("line1\n{{v}}\nline3", map[string]string{"v": "x\r\ny"})
	if want := "line1\nx  y\nline3"; got != want { // CR and LF each flatten to a space
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_RepeatedKey(t *testing.T) {
	got := Render("{{a}}-{{a}}", map[string]string{"a": "z"})
	if got != "z-z" {
		t.Errorf("got %q", got)
	}
}
