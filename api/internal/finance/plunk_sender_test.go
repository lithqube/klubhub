package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestPlunkSender_SendsCorrectRequest verifies the JSON payload Plunk receives
// matches Plunk's REST API contract:
//
//	POST {base}/v1/send
//	Authorization: Bearer ...
//	Content-Type: application/json
func TestPlunkSender_SendsCorrectRequest(t *testing.T) {
	var captured struct {
		Method  string
		Path    string
		Auth    string
		IdemKey string
		Body    plunkRequest
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Method = r.Method
		captured.Path = r.URL.Path
		captured.Auth = r.Header.Get("Authorization")
		captured.IdemKey = r.Header.Get("Idempotency-Key")
		bodyBytes, _ := io.ReadAll(r.Body)
		json.Unmarshal(bodyBytes, &captured.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.ReplaceAll(runtimePlunkAck, "to@example.com", "client@example.com")))
	}))
	defer server.Close()

	sender := NewPlunkSender(PlunkConfig{
		BaseURL:   server.URL,
		ProjectID: "proj-abc",
		APIKey:    "secret-key",
		FromEmail: "noreply@klubhub.example",
		FromName:  "KlubHub DJ",
	})

	msg := &EmailMessage{
		ID:        uuid.New(),
		FromEmail: "noreply@klubhub.example",
		FromName:  "KlubHub DJ",
		ToEmail:   "client@example.com",

		Subject:  "Invoice INV-0001-EUR",
		Body:     "Hello, your invoice is attached.",
		BodyHTML: "<p>Hello, your invoice is attached.</p>",
	}

	if err := sender.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if captured.Method != http.MethodPost {
		t.Errorf("method: %s", captured.Method)
	}
	if captured.Path != "/v1/send" {
		t.Errorf("path: %s", captured.Path)
	}
	if captured.Auth != "Bearer secret-key" {
		t.Errorf("auth: %s", captured.Auth)
	}
	if captured.IdemKey != msg.ID.String() {
		t.Errorf("idempotency: got %s, want %s", captured.IdemKey, msg.ID.String())
	}
	if captured.Body.From.Email != "noreply@klubhub.example" {
		t.Errorf("from.email: %s", captured.Body.From.Email)
	}
	if captured.Body.From.Name != "KlubHub DJ" {
		t.Errorf("from.name: %s", captured.Body.From.Name)
	}
	if len(captured.Body.To) != 1 || captured.Body.To[0] != "client@example.com" {
		t.Errorf("to: %v", captured.Body.To)
	}
	if len(captured.Body.CC) != 0 {
		t.Errorf("cc: %v", captured.Body.CC)
	}
	if captured.Body.Subject != "Invoice INV-0001-EUR" {
		t.Errorf("subject: %s", captured.Body.Subject)
	}
	if captured.Body.Body != "<p>Hello, your invoice is attached.</p>" {
		t.Errorf("body: %s", captured.Body.Body)
	}
	if captured.Body.BodyHTML != "" {
		t.Errorf("bodyHtml: %s", captured.Body.BodyHTML)
	}
}

// TestPlunkSender_UsesConfigFromWhenMessageEmpty verifies that if FromEmail is
// blank on the message, the sender falls back to PlunkConfig.FromEmail.
func TestPlunkSender_UsesConfigFromWhenMessageEmpty(t *testing.T) {
	var capturedBody plunkRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.ReplaceAll(runtimePlunkAck, "to@example.com", "x@y.com")))
	}))
	defer server.Close()

	sender := NewPlunkSender(PlunkConfig{
		BaseURL:   server.URL,
		ProjectID: "p",
		APIKey:    "k",
		FromEmail: "noreply@klubhub.example",
		FromName:  "KlubHub",
	})

	msg := &EmailMessage{
		ID:      uuid.New(),
		ToEmail: "x@y.com",
		Subject: "x",
		Body:    "y",
		// FromEmail / FromName intentionally blank
	}
	if err := sender.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if capturedBody.From.Email != "noreply@klubhub.example" {
		t.Errorf("from fallback: %s", capturedBody.From.Email)
	}
	if capturedBody.From.Name != "KlubHub" {
		t.Errorf("name fallback: %s", capturedBody.From.Name)
	}
}

// TestPlunkSender_NoFromErrors verifies both message-from and config-from
// being empty yields a hard error rather than sending an unconfigured message.
func TestPlunkSender_NoFromErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not have been called")
	}))
	defer server.Close()

	sender := NewPlunkSender(PlunkConfig{BaseURL: server.URL, ProjectID: "p", APIKey: "k"})
	msg := &EmailMessage{ID: uuid.New(), ToEmail: "x@y.com", Subject: "x", Body: "y"}
	err := sender.Send(msg)
	if err == nil {
		t.Fatal("expected error when from email missing")
	}
}

// TestPlunkSender_FailsOnNon2xx verifies HTTP error bodies surface as Go errors.
func TestPlunkSender_FailsOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success": false, "message": "invalid from"}`))
	}))
	defer server.Close()

	sender := NewPlunkSender(PlunkConfig{
		BaseURL: server.URL, ProjectID: "p", APIKey: "k",
		FromEmail: "noreply@klubhub.example",
	})
	msg := &EmailMessage{
		ID: uuid.New(), FromEmail: "noreply@klubhub.example",
		ToEmail: "x@y.com", Subject: "x", Body: "y",
	}
	err := sender.Send(msg)
	if err == nil {
		t.Fatal("expected error on 400 response")
	}
}

// TestPlunkSender_RetriesWithIdempotency verifies that reusing the same
// EmailMessage ID produces the same Idempotency-Key (Plunk uses this to
// de-duplicate).
func TestPlunkSender_RetriesWithIdempotency(t *testing.T) {
	var idemKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idemKeys = append(idemKeys, r.Header.Get("Idempotency-Key"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.ReplaceAll(runtimePlunkAck, "to@example.com", "x@y.com")))
	}))
	defer server.Close()

	sender := NewPlunkSender(PlunkConfig{
		BaseURL: server.URL, ProjectID: "p", APIKey: "k",
		FromEmail: "noreply@klubhub.example",
	})
	fixedID := uuid.New()
	msg := &EmailMessage{
		ID:        fixedID,
		FromEmail: "noreply@klubhub.example",
		ToEmail:   "x@y.com",
		Subject:   "x",
		Body:      "y",
	}
	sender.Send(msg)
	sender.Send(msg)
	if idemKeys[0] != idemKeys[1] {
		t.Errorf("idempotency keys should match for retries: %v", idemKeys)
	}
	if idemKeys[0] != fixedID.String() {
		t.Errorf("idempotency key should be msg id: %s", idemKeys[0])
	}
}

// TestEmailService_PlunkWiring verifies EmailService integrates with PlunkSender.
func TestEmailService_PlunkWiring(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.ReplaceAll(runtimePlunkAck, "to@example.com", "client@example.com")))
	}))
	defer server.Close()

	repo := newFakeEmailRepo()
	sender := NewPlunkSender(PlunkConfig{
		BaseURL:   server.URL,
		ProjectID: "test-proj",
		APIKey:    "test-key",
		FromEmail: "noreply@klubhub.example",
		FromName:  "KlubHub",
	})
	svc := NewEmailService(repo, sender)

	ownerID := uuid.New()
	req := CreateEmailRequest{
		Kind:      EmailKindInvoiceIssued,
		OwnerType: "invoice",
		OwnerID:   &ownerID,
		FromEmail: "noreply@klubhub.example",
		FromName:  "KlubHub",
		ToEmail:   "client@example.com",
		Subject:   "Invoice INV-0001-EUR",
		Body:      "Thanks!",
	}
	msg, err := svc.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msg.Status != EmailStatusSent {
		t.Errorf("status: %s", msg.Status)
	}
}

// TestEmailSender_InterfaceMatrix verifies that both SMTP and Plunk satisfy
// the EmailSender interface (compile-time guarantee).
func TestEmailSender_InterfaceMatrix(t *testing.T) {
	var _ EmailSender = (*DefaultSMTPSender)(nil)
	var _ EmailSender = (*PlunkSender)(nil)
}

// silence unused imports in some build configurations
var _ = bytes.NewReader
var _ = time.Now
var _ = errors.New

// Redirects must never be followed: 307/308 would replay the POST body and
// Idempotency-Key to another location.
func TestPlunkSender_DoesNotFollowRedirects(t *testing.T) {
	var targetHits int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
		w.Write([]byte(runtimePlunkAck))
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/send", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	sender := NewPlunkSender(PlunkConfig{BaseURL: server.URL, FromEmail: "noreply@klubhub.example"})
	err := sender.Send(&EmailMessage{ID: uuid.New(), ToEmail: "to@example.com", Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("redirect response must be treated as failure")
	}
	if targetHits != 0 {
		t.Fatalf("redirect was followed (%d hits)", targetHits)
	}
}
