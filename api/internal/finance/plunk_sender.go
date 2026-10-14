package finance

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
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
//	POST {BaseURL}/v1/send
//	Authorization: Bearer {APIKey}
//	Content-Type: application/json
//	Body: {"from":{"email":"...","name":"..."}, "to":["..."],"subject":"...","body":"HTML"}
//
// Idempotency-Key header is supported (recommended for retry safety).
type PlunkSender struct {
	cfg    PlunkConfig
	client *http.Client
}

// NewPlunkSender creates a PlunkSender.
func NewPlunkSender(cfg PlunkConfig) *PlunkSender {
	return &PlunkSender{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
			// Never follow redirects: 307/308 would replay the POST body and
			// Idempotency-Key to another location. A 3xx is a failure.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// plunkRequest is the JSON payload we send to the Plunk REST API.
type plunkRequest struct {
	From    plunkContact `json:"from"`
	To      []string     `json:"to"`
	CC      []string     `json:"cc,omitempty"`
	BCC     []string     `json:"bcc,omitempty"`
	Subject string       `json:"subject"`
	// Reply is a single plain address; Plunk sets it as the Reply-To header.
	Reply    string `json:"reply,omitempty"`
	Body     string `json:"body,omitempty"`
	BodyHTML string `json:"bodyHtml,omitempty"`
	// Attachments follow Plunk's send schema: base64 content, at most 10 files
	// and 10 MB of base64 by default.
	Attachments []plunkAttachment `json:"attachments,omitempty"`
}

type plunkAttachment struct {
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	ContentType string `json:"contentType"`
	Disposition string `json:"disposition"`
}

const (
	maxPlunkAttachments      = 10
	maxPlunkAttachmentBase64 = 10 << 20
)

// plunkFilename keeps a filename inside what Plunk accepts: no quotes or line
// breaks, at most 255 characters.
func plunkFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '"' || r == '\r' || r == '\n' || r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	if len(name) > 255 {
		name = name[:255]
	}
	if name == "" {
		name = "attachment"
	}
	return name
}

type plunkContact struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// Send posts the email to Plunk's public REST API. Returns an error wrapping
// any transport or non-200 response.
func (p *PlunkSender) Send(msg *EmailMessage) error {
	return p.SendContext(context.Background(), msg)
}

// ValidateRequest rejects features absent from the official /v1/send contract.
func (p *PlunkSender) ValidateRequest(req CreateEmailRequest) error {
	if len(req.CCEmails) > 0 || len(req.BCCEmails) > 0 {
		return fmt.Errorf("%w: Plunk cc_emails and bcc_emails are not supported", ErrEmailValidation)
	}
	return nil
}

func (p *PlunkSender) SendContext(ctx context.Context, msg *EmailMessage) error {
	if err := p.ValidateRequest(CreateEmailRequest{CCEmails: msg.CCEmails, BCCEmails: msg.BCCEmails}); err != nil {
		return err
	}
	// Attachments must have been loaded by the service; never send a message
	// that is missing files it claims to carry.
	if len(msg.AttachmentIDs) > 0 && len(msg.Attachments) != len(msg.AttachmentIDs) {
		return fmt.Errorf("%w: attachment content was not loaded; no email was sent", ErrEmailValidation)
	}
	if len(msg.Attachments) > maxPlunkAttachments {
		return fmt.Errorf("%w: at most %d attachments are supported; no email was sent", ErrEmailValidation, maxPlunkAttachments)
	}
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

	reply := ""
	if msg.ReplyTo != "" {
		addr, err := mail.ParseAddress(msg.ReplyTo)
		if err != nil {
			return fmt.Errorf("%w: reply_to is not a valid email address; no email was sent", ErrEmailValidation)
		}
		reply = addr.Address // Plunk takes a bare address, not "Name <addr>"
	}

	req := plunkRequest{
		Reply:   reply,
		From:    plunkContact{Email: fromEmail, Name: fromName},
		To:      []string{msg.ToEmail},
		CC:      msg.CCEmails,
		BCC:     msg.BCCEmails,
		Subject: msg.Subject,
		Body:    msg.Body,
	}

	if msg.BodyHTML != "" {
		req.Body = msg.BodyHTML
	} else {
		req.Body = "<pre>" + html.EscapeString(msg.Body) + "</pre>"
	}
	encoded := 0
	for _, a := range msg.Attachments {
		content := base64.StdEncoding.EncodeToString(a.Content)
		if encoded += len(content); encoded > maxPlunkAttachmentBase64 {
			return fmt.Errorf("%w: attachments are too large; no email was sent", ErrEmailValidation)
		}
		req.Attachments = append(req.Attachments, plunkAttachment{
			Filename: plunkFilename(a.Filename), Content: content, ContentType: a.MimeType, Disposition: "attachment",
		})
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("plunk: marshal: %w", err)
	}

	base := p.cfg.BaseURL
	if base == "" {
		base = "https://api.useplunk.com"
	}
	base = strings.TrimRight(base, "/")
	url := base + "/v1/send"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPlunkResponseBytes+1))
	if err != nil {
		return fmt.Errorf("plunk: read acknowledgement: %w", err)
	}
	if len(body) > maxPlunkResponseBytes {
		return fmt.Errorf("plunk: acknowledgement too large")
	}
	// Official Plunk does not replay receipts. Only a matched duplicate with
	// an explicitly successful original result proves acceptance; in-flight
	// (null), original 5xx and arbitrary conflicts remain failures.
	if resp.StatusCode == http.StatusConflict {
		var duplicate struct {
			Success bool `json:"success"`
			Error   struct {
				Code    string `json:"code"`
				Details struct {
					Key                string `json:"key"`
					OriginalRequest    string `json:"originalRequest"`
					OriginalStatusCode *int   `json:"originalStatusCode"`
				} `json:"details"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &duplicate) == nil && !duplicate.Success && duplicate.Error.Code == "IDEMPOTENCY_KEY_REUSED" {
			d := duplicate.Error.Details
			if d.Key == msg.ID.String() && d.OriginalRequest == "POST /v1/send" && d.OriginalStatusCode != nil && *d.OriginalStatusCode == http.StatusOK {
				return nil
			}
		}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plunk: HTTP %d", resp.StatusCode)
	}
	var ack struct {
		Success bool `json:"success"`
		Data    struct {
			Emails []struct {
				Contact struct {
					Email string `json:"email"`
				} `json:"contact"`
				Email string `json:"email"`
			} `json:"emails"`
			Timestamp time.Time `json:"timestamp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ack); err != nil {
		return fmt.Errorf("plunk: invalid acknowledgement: %w", err)
	}
	if !ack.Success || len(ack.Data.Emails) != 1 || ack.Data.Timestamp.IsZero() {
		return fmt.Errorf("plunk: missing successful email acknowledgement")
	}
	email := ack.Data.Emails[0]
	if id, err := uuid.Parse(email.Email); err != nil || id == uuid.Nil || !strings.EqualFold(email.Contact.Email, msg.ToEmail) {
		return fmt.Errorf("plunk: invalid email receipt")
	}
	return nil
}
