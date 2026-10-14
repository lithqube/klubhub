package finance

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Embedded nil repo interface: any repository call panics.
type panicRepo struct{ EmailRepositoryIface }

type ctxSender struct{}

func (ctxSender) Send(*EmailMessage) error                         { return nil }
func (ctxSender) SendContext(context.Context, *EmailMessage) error { return nil }

func TestStartWorkerRecoversFromTickPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var errs atomic.Int32
	var msg atomic.Value
	done := NewEmailService(panicRepo{}, ctxSender{}).StartWorker(ctx, 5*time.Millisecond, func(err error) {
		msg.Store(err.Error())
		errs.Add(1)
	})
	deadline := time.After(2 * time.Second)
	for errs.Load() < 2 { // survived the first panic and ticked again
		select {
		case <-done:
			t.Fatal("worker exited after panic")
		case <-deadline:
			t.Fatalf("worker did not keep running; errors=%d", errs.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
	if s, _ := msg.Load().(string); !strings.Contains(s, "panic") {
		t.Fatalf("panic not reported: %q", s)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop on cancel")
	}
}
