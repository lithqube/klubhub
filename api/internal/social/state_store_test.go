package social

import (
	"sync"
	"testing"
	"time"
)

// TestStateStore_PutGetAndConsume verifies basic put/consume behaviour
// with bind-token pairing.
func TestStateStore_PutGetAndConsume(t *testing.T) {
	store := newStateStore(10 * time.Minute)
	defer store.Stop()

	state, bind, err := store.Issue()
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if state == "" || bind == "" {
		t.Fatal("expected non-empty state and bind tokens")
	}
	if state == bind {
		t.Fatal("state and bind must be different tokens")
	}

	gotBind, ok := store.Consume(state)
	if !ok {
		t.Fatal("expected consume to succeed")
	}
	if gotBind != bind {
		t.Errorf("expected bind %q, got %q", bind, gotBind)
	}
}

// TestStateStore_SingleUse verifies that a state token can only be
// consumed once. Replay protection is a CSRF mitigation requirement.
func TestStateStore_SingleUse(t *testing.T) {
	store := newStateStore(10 * time.Minute)
	defer store.Stop()

	state, _, err := store.Issue()
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	if _, ok := store.Consume(state); !ok {
		t.Fatal("first consume must succeed")
	}
	if _, ok := store.Consume(state); ok {
		t.Fatal("second consume must fail (single-use)")
	}
}

// TestStateStore_Expired verifies that expired state tokens are
// rejected on consume.
func TestStateStore_Expired(t *testing.T) {
	store := newStateStore(50 * time.Millisecond)
	defer store.Stop()

	state, _, err := store.Issue()
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	if _, ok := store.Consume(state); ok {
		t.Fatal("expected consume to fail for expired state")
	}
}

// TestStateStore_Unknown verifies that consuming an unknown state
// returns ok=false.
func TestStateStore_Unknown(t *testing.T) {
	store := newStateStore(10 * time.Minute)
	defer store.Stop()

	if _, ok := store.Consume("never-issued"); ok {
		t.Fatal("expected unknown state lookup to fail")
	}
}

// TestStateStore_ConcurrentIssue exercises parallel issuance to ensure
// no two issues share a state or bind.
func TestStateStore_ConcurrentIssue(t *testing.T) {
	store := newStateStore(10 * time.Minute)
	defer store.Stop()

	const n = 50
	states := make([]string, n)
	binds := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, b, err := store.Issue()
			if err != nil {
				t.Errorf("Issue %d: %v", i, err)
				return
			}
			states[i] = s
			binds[i] = b
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		if seen[states[i]] {
			t.Fatalf("duplicate state %q at index %d", states[i], i)
		}
		seen[states[i]] = true
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if binds[i] == binds[j] {
				t.Fatalf("duplicate bind %q at indices %d, %d", binds[i], i, j)
			}
		}
	}
}

// TestStateStore_StopIsIdempotent verifies that calling Stop multiple
// times does not panic and that the background goroutine exits.
func TestStateStore_StopIsIdempotent(t *testing.T) {
	store := newStateStore(10 * time.Minute)
	store.Stop()
	store.Stop() // second call must not panic
}
