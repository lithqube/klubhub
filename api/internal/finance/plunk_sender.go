package finance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxPlunkResponseBytes bounds how much of a Plunk API response body we
// will read. Without a limit, a misbehaving or malicious endpoint (e.g. a
// misconfigured self-hosted PLUNK_BASE_URL) could stream an unbounded
// response and exhaust memory.
const maxPlunkResponseBytes = 1 << 20 // 1 MiB

// PlunkSender is the EmailSender implementation that delivers transactional
// emails through Plunk (https://github.com/useplunk/plunk) — the open-source
// email platform built on AWS SES that we already use for the GitHub page.
//
// Plunk exposes a public REST API:
//
//	POST {BaseURL}/api/v1/{ProjectID}/emails
//	Authorization: Bearer {APIKey}
//	Content-Type: application/json
//	Body: {"from":{"email":"...","name":"..."}, "to":["..."],"subject":"...","body":"...","bodyHtml":"..."}
//
// Idempotency-Key header is supported (recommended for retry safety).
type PlunkSender struct {
	cfg    PlunkConfig
	client *http.Client
}

// NewPlunkSender creates a PlunkSender.
func NewPlunkSender(cfg PlunkConfig) *PlunkSender {
	return &PlunkSender{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// plunkRequest is the JSON payload we send to the Plunk REST API.
type plunkRequest struct {
	From     plunkContact `json:"from"`
	To       []string     `json:"to"`
	CC       []string     `json:"cc,omitempty"`
	BCC      []string     `json:"bcc,omitempty"`
	Subject  string       `json:"subject"`
	Body     string       `json:"body,omitempty"`
	BodyHTML string       `json:"bodyHtml,omitempty"`
}

type plunkContact struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// plunkResponse is what Plunk returns on success.
type plunkResponse struct {
	Success bool   `json:"success"`
	EmailID string `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
}

// Send posts the email to Plunk's public REST API. Returns an error wrapping
// any transport or non-200 response.
func (p *PlunkSender) Send(msg *EmailMessage) error {
	fromEmail := msg.FromEmail
	if fromEmail == "" {
		fromEmail = p.cfg.FromEmail
	}
	fromName := msg.FromName
	if fromName == "" {
		fromName = p.cfg.FromName
	}
	if fromEmail == "" {
		return fmt.Errorf("plunk: from email not set")
	}

	req := plunkRequest{
		From:     plunkContact{Email: fromEmail, Name: fromName},
		To:       []string{msg.ToEmail},
		CC:       msg.CCEmails,
		BCC:      msg.BCCEmails,
		Subject:  msg.Subject,
		Body:     msg.Body,
		BodyHTML: msg.BodyHTML,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("plunk: marshal: %w", err)
	}

	base := p.cfg.BaseURL
	if base == "" {
		base = "https://app.useplunk.com"
	}
	base = strings.TrimRight(base, "/")
	url := fmt.Sprintf("%s/api/v1/%s/emails", base, p.cfg.ProjectID)

	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("plunk: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	// Idempotency-Key helps retry-safety against accidental duplicate sends
	httpReq.Header.Set("Idempotency-Key", msg.ID.String())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("plunk: send: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxPlunkResponseBytes))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("plunk: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var pres plunkResponse
	if err := json.Unmarshal(body, &pres); err == nil && !pres.Success {
		return fmt.Errorf("plunk: success=false: %s", pres.Message)
	}
	return nil
}
