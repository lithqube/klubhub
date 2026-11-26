package marketing_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/klubhub/dj/api/internal/promoter/marketing"
)

// smtpSink is a minimal local SMTP server: it accepts every command and
// hands each DATA payload to got.
type smtpSink struct {
	ln  net.Listener
	got chan string
}

func newSMTPSink(t *testing.T) *smtpSink {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &smtpSink{ln: ln, got: make(chan string, 4)}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *smtpSink) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	reply := func(l string) { _, _ = c.Write([]byte(l + "\r\n")) }
	reply("220 sink ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
		case strings.HasPrefix(cmd, "DATA"):
			reply("354 go ahead")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			s.got <- b.String()
			reply("250 queued")
		case strings.HasPrefix(cmd, "QUIT"):
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

func (s *smtpSink) config() marketing.SMTPConfig {
	host, port, _ := net.SplitHostPort(s.ln.Addr().String())
	p, _ := strconv.Atoi(port)
	return marketing.SMTPConfig{Host: host, Port: p, Timeout: 2 * time.Second}
}

func testMessage() *marketing.Message {
	return &marketing.Message{
		To: "fan@example.com", FromEmail: "news@club.example", FromName: "Club",
		Subject: "Friday lineup", Text: "See you Friday", HTML: "<p>See you Friday</p>",
		UnsubscribeURL: "https://club.example/u/abc",
	}
}

func TestSMTPProvider_SendArrivesAtSink(t *testing.T) {
	sink := newSMTPSink(t)
	id, err := marketing.NewSMTPProvider(sink.config()).SendMarketing(context.Background(), testMessage())
	if err != nil || id != "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	select {
	case data := <-sink.got:
		for _, want := range []string{
			"To: <fan@example.com>", "Subject: Friday lineup",
			"List-Unsubscribe: <https://club.example/u/abc>",
			"List-Unsubscribe-Post: List-Unsubscribe=One-Click",
			"multipart/alternative", "See you Friday", "<p>See you Friday</p>",
		} {
			if !strings.Contains(data, want) {
				t.Errorf("message missing %q:\n%s", want, data)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("message never reached the sink")
	}
}

func TestSMTPProvider_RejectsHeaderInjection(t *testing.T) {
	sink := newSMTPSink(t)
	msg := testMessage()
	msg.To = "fan@example.com\r\nBcc: victim@example.com"
	if _, err := marketing.NewSMTPProvider(sink.config()).SendMarketing(context.Background(), msg); err == nil {
		t.Fatal("want error for injected recipient")
	}
}

func TestSMTPProvider_GateWithoutAttestation(t *testing.T) {
	sink := newSMTPSink(t)
	p := marketing.Gated(marketing.NewGate(false), marketing.NewSMTPProvider(sink.config()))
	if _, err := p.SendMarketing(context.Background(), testMessage()); !errors.Is(err, marketing.ErrNotPermitted) {
		t.Fatalf("err = %v, want ErrNotPermitted", err)
	}
	rec := httptest.NewRecorder()
	marketing.NewGate(false).Require(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "marketing_not_permitted") {
		t.Fatalf("route = %d %q", rec.Code, rec.Body.String())
	}

	attested := marketing.Gated(marketing.NewGate(true), marketing.NewSMTPProvider(sink.config()))
	if _, err := attested.SendMarketing(context.Background(), testMessage()); err != nil {
		t.Fatalf("attested send: %v", err)
	}
	select {
	case <-sink.got: // only the attested send reached the relay
	case <-time.After(2 * time.Second):
		t.Fatal("attested send never arrived")
	}
	if len(sink.got) != 0 {
		t.Fatal("unattested send reached the relay")
	}
}

func TestSMTPProvider_DeadRelayTimesOut(t *testing.T) {
	// Accepts the TCP connection but never sends a greeting.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(port)
	cfg := marketing.SMTPConfig{Host: host, Port: p, Timeout: 200 * time.Millisecond}

	start := time.Now()
	_, err = marketing.NewSMTPProvider(cfg).SendMarketing(context.Background(), testMessage())
	if err == nil {
		t.Fatal("want error from a silent relay")
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("took %v, want about the 200ms timeout", took)
	}
}

func TestSMTPProvider_HasNoWebhooks(t *testing.T) {
	evs, err := marketing.NewSMTPProvider(marketing.SMTPConfig{}).HandleWebhook(httptest.NewRequest(http.MethodPost, "/", nil))
	if err == nil || evs != nil {
		t.Fatalf("evs=%v err=%v, want fail closed", evs, err)
	}
}
