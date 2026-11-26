package marketing

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"time"
)

// DefaultSMTPTimeout bounds one whole SMTP session when SMTPConfig.Timeout is unset.
const DefaultSMTPTimeout = 30 * time.Second

// ErrNoWebhooks: plain SMTP has no delivery webhooks; bounces come back as mail.
var ErrNoWebhooks = errors.New("marketing: smtp provider has no webhooks")

// SMTPConfig is a tenant's own SMTP relay (provider 'smtp').
type SMTPConfig struct {
	Host     string
	Port     int // 465 is implicit TLS; any other port uses STARTTLS when offered
	Username string
	Password string
	// Timeout bounds dial plus the whole session, so a dead relay errors
	// instead of hanging the queue. Zero means DefaultSMTPTimeout.
	Timeout time.Duration
}

// SMTPProvider delivers marketing mail through a generic SMTP relay. Wrap it
// with Gated: the relay being configured says nothing about its terms.
type SMTPProvider struct{ cfg SMTPConfig }

// NewSMTPProvider builds the adapter for one tenant's relay.
func NewSMTPProvider(cfg SMTPConfig) *SMTPProvider { return &SMTPProvider{cfg: cfg} }

// SendMarketing delivers msg. SMTP has no message id, so it returns "".
func (p *SMTPProvider) SendMarketing(ctx context.Context, msg *Message) (string, error) {
	body, err := buildMIME(msg)
	if err != nil {
		return "", err
	}
	timeout := p.cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultSMTPTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := net.JoinHostPort(p.cfg.Host, strconv.Itoa(p.cfg.Port))
	tlsCfg := &tls.Config{ServerName: p.cfg.Host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	if p.cfg.Port == 465 {
		conn, err = (&tls.Dialer{Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return "", fmt.Errorf("smtp: dial: %w", err)
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() }) // caller cancel aborts mid-session
	defer stop()

	if err := p.session(conn, tlsCfg, msg, body); err != nil {
		return "", fmt.Errorf("smtp: %w", err)
	}
	return "", nil
}

func (p *SMTPProvider) session(conn net.Conn, tlsCfg *tls.Config, msg *Message, body []byte) error {
	c, err := smtp.NewClient(conn, p.cfg.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, isTLS := conn.(*tls.Conn); !isTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return err
			}
		}
	}
	if p.cfg.Username != "" {
		// PlainAuth refuses to send credentials over a non-TLS link to a remote host.
		if err := c.Auth(smtp.PlainAuth("", p.cfg.Username, p.cfg.Password, p.cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(msg.FromEmail); err != nil {
		return err
	}
	if err := c.Rcpt(msg.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// HandleWebhook fails closed: there is no signed SMTP webhook to trust.
func (p *SMTPProvider) HandleWebhook(*http.Request) ([]WebhookEvent, error) {
	return nil, ErrNoWebhooks
}

// buildMIME renders msg as RFC 5322 text/plain + text/html alternative.
// Addresses are parsed (no CR/LF can survive) and the subject is Q-encoded.
func buildMIME(msg *Message) ([]byte, error) {
	from, err := mail.ParseAddress(msg.FromEmail)
	if err != nil {
		return nil, fmt.Errorf("smtp: from address: %w", err)
	}
	from.Name = msg.FromName
	to, err := mail.ParseAddress(msg.To)
	if err != nil || to.Address != msg.To {
		return nil, fmt.Errorf("smtp: to address %q is not a bare address", msg.To)
	}

	var buf bytes.Buffer
	h := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	h("From", from.String())
	h("To", to.String())
	if msg.ReplyTo != "" {
		rt, err := mail.ParseAddress(msg.ReplyTo)
		if err != nil {
			return nil, fmt.Errorf("smtp: reply-to address: %w", err)
		}
		h("Reply-To", rt.String())
	}
	h("Subject", mime.QEncoding.Encode("UTF-8", msg.Subject))
	h("Date", time.Now().Format(time.RFC1123Z))
	h("MIME-Version", "1.0")
	if msg.UnsubscribeURL != "" {
		uh, err := UnsubscribeHeaders(msg.UnsubscribeURL)
		if err != nil {
			return nil, err
		}
		for _, k := range []string{"List-Unsubscribe", "List-Unsubscribe-Post"} {
			h(k, uh.Get(k))
		}
	}

	mw := multipart.NewWriter(&buf)
	h("Content-Type", `multipart/alternative; boundary="`+mw.Boundary()+`"`)
	buf.WriteString("\r\n")
	for _, part := range []struct{ typ, body string }{{"text/plain", msg.Text}, {"text/html", msg.HTML}} {
		if part.body == "" {
			continue
		}
		pw, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.typ + "; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		// Quoted-printable keeps every line under SMTP's 998-octet limit.
		qp := quotedprintable.NewWriter(pw)
		_, _ = qp.Write([]byte(part.body))
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
