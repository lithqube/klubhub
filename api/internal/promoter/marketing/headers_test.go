package marketing

import (
	"bufio"
	"errors"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"testing"

	"github.com/klubhub/dj/api/internal/platform/email"
)

// smtpSink accepts one SMTP session and returns the parsed message it got.
func smtpSink(t *testing.T) (host string, port int, msg <-chan *mail.Message) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	out := make(chan *mail.Message, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		say := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		say("220 sink")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				say("250-sink")
				say("250 AUTH PLAIN") // DefaultSMTPSender always authenticates
			case strings.HasPrefix(cmd, "AUTH"):
				say("235 ok")
			case cmd == "DATA":
				say("354 go")
				m, err := mail.ReadMessage(newDotReader(r))
				if err == nil {
					out <- m
				}
				say("250 ok")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	a := ln.Addr().(*net.TCPAddr)
	return a.IP.String(), a.Port, out
}

func newDotReader(r *bufio.Reader) *strings.Reader {
	var b strings.Builder
	for {
		l, err := r.ReadString('\n')
		if err != nil || l == ".\r\n" {
			break
		}
		b.WriteString(strings.TrimPrefix(l, "."))
	}
	return strings.NewReader(b.String())
}

func TestMarketingSendCarriesOneClickHeaders(t *testing.T) {
	host, port, got := smtpSink(t)
	const target = "https://mail.example.com/unsubscribe/abc123"
	h, err := UnsubscribeHeaders(target)
	if err != nil {
		t.Fatal(err)
	}
	raw := "From: a@example.com\r\nTo: b@example.com\r\nSubject: hi\r\n"
	for k, v := range h {
		raw += k + ": " + v[0] + "\r\n"
	}
	raw += "\r\nbody\r\n"
	if err := smtp.SendMail(net.JoinHostPort(host, strconv.Itoa(port)), nil, "a@example.com", []string{"b@example.com"}, []byte(raw)); err != nil {
		t.Fatal(err)
	}
	m := <-got
	if v := m.Header.Get("List-Unsubscribe"); v != "<"+target+">" {
		t.Fatalf("List-Unsubscribe = %q", v)
	}
	if v := m.Header.Get("List-Unsubscribe-Post"); v != "List-Unsubscribe=One-Click" {
		t.Fatalf("List-Unsubscribe-Post = %q", v)
	}
}

func TestTransactionalSendHasNoUnsubscribeHeaders(t *testing.T) {
	host, port, got := smtpSink(t)
	s := email.NewDefaultSMTPSender(email.SMTPConfig{Host: host, Port: port})
	err := s.Send(&email.EmailMessage{FromEmail: "a@example.com", ToEmail: "b@example.com", Subject: "invoice", Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	m := <-got
	for _, k := range []string{"List-Unsubscribe", "List-Unsubscribe-Post"} {
		if m.Header.Get(k) != "" {
			t.Fatalf("transactional mail has %s", k)
		}
	}
}

func TestUnsubscribeHeadersRejectsBadURL(t *testing.T) {
	for _, u := range []string{"", "http://x.example/u", "/relative", "https://x.example/u>\r\nBcc: z@example.com", "https://x.example/<u>", "https://"} {
		if _, err := UnsubscribeHeaders(u); !errors.Is(err, ErrInvalidUnsubscribeURL) {
			t.Errorf("%q: err = %v", u, err)
		}
	}
}
