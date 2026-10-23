package email

import (
	"bytes"
	"fmt"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
)

// SMTPConfig holds SMTP credentials for delivery.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// EmailSender is the abstraction over delivery transports.
// Implementations: DefaultSMTPSender (SMTP), PlunkSender (REST).
type EmailSender interface {
	Send(msg *EmailMessage) error
}

// DefaultSMTPSender is the SMTP implementation (kept for local dev / fallback).
type DefaultSMTPSender struct {
	cfg SMTPConfig
}

func NewDefaultSMTPSender(cfg SMTPConfig) *DefaultSMTPSender {
	return &DefaultSMTPSender{cfg: cfg}
}

func (s *DefaultSMTPSender) Send(msg *EmailMessage) error {
	if len(msg.AttachmentIDs) > 0 {
		return fmt.Errorf("%w: attachment_ids are not supported; no email was sent", ErrEmailValidation)
	}
	// Defense in depth: ValidateCreateEmailRequest already rejects header
	// injection (CR/LF/control chars) and validates addresses at the API
	// boundary, but this sender must not trust that every caller went
	// through that path. Re-encode headers safely here too.
	if err := rejectHeaderInjection(msg.Subject); err != nil {
		return fmt.Errorf("smtp: subject: %w", err)
	}
	if err := rejectHeaderInjection(msg.FromName); err != nil {
		return fmt.Errorf("smtp: from name: %w", err)
	}

	from := formatAddress(msg.FromName, msg.FromEmail)
	if _, err := mail.ParseAddress(msg.FromEmail); err != nil {
		return fmt.Errorf("smtp: invalid from address: %w", err)
	}
	to, err := formatAddressList([]string{msg.ToEmail})
	if err != nil {
		return fmt.Errorf("smtp: invalid to address: %w", err)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", to)
	if msg.ReplyTo != "" {
		replyTo, err := formatAddressList([]string{msg.ReplyTo})
		if err != nil {
			return fmt.Errorf("smtp: invalid reply-to address: %w", err)
		}
		fmt.Fprintf(&buf, "Reply-To: %s\r\n", replyTo)
	}
	if len(msg.CCEmails) > 0 {
		cc, err := formatAddressList(msg.CCEmails)
		if err != nil {
			return fmt.Errorf("smtp: invalid cc address: %w", err)
		}
		fmt.Fprintf(&buf, "Cc: %s\r\n", cc)
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", msg.Subject))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	if msg.BodyHTML != "" {
		fmt.Fprintf(&buf, "Content-Type: text/html; charset=UTF-8\r\n")
	} else {
		fmt.Fprintf(&buf, "Content-Type: text/plain; charset=UTF-8\r\n")
	}
	fmt.Fprintf(&buf, "\r\n")
	if msg.BodyHTML != "" {
		fmt.Fprintf(&buf, "%s\r\n", msg.BodyHTML)
	} else {
		fmt.Fprintf(&buf, "%s\r\n", msg.Body)
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	recipients := append([]string{msg.ToEmail}, msg.CCEmails...)
	recipients = append(recipients, msg.BCCEmails...)
	return smtp.SendMail(addr, auth, msg.FromEmail, recipients, buf.Bytes())
}

// formatAddress renders a name + email pair as a single RFC 5322 address
// header value via net/mail, which handles quoting/escaping so a crafted
// name cannot break out of the header.
func formatAddress(name, email string) string {
	return (&mail.Address{Name: name, Address: email}).String()
}

// formatAddressList validates and formats a list of plain email addresses
// as a comma-separated RFC 5322 header value.
func formatAddressList(addrs []string) (string, error) {
	formatted := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if _, err := mail.ParseAddress(a); err != nil {
			return "", fmt.Errorf("invalid address %q: %w", a, err)
		}
		formatted = append(formatted, formatAddress("", a))
	}
	return strings.Join(formatted, ", "), nil
}
