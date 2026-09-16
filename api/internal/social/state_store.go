package social

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ErrOAuthStateInvalid is returned when the state token is unknown,
// already consumed, or expired.
var ErrOAuthStateInvalid = errors.New("oauth state invalid")

// stateEntry holds one pending OAuth state record.
type stateEntry struct {
	bind      string
	createdAt time.Time
}

// StateStore keeps short-lived OAuth CSRF state tokens in memory. It is
// intentionally minimal: no persistence, no cross-process replication.
// Suitable for a single-user self-hosted deployment where the API
// process is the only authority.
//
// Each entry pairs a `state` token (returned to the caller to put in
// the OAuth URL) with a `bind` token (set as an HttpOnly cookie). On
// callback the handler verifies both match and consumes the entry, so
// replay is impossible even if the state URL is leaked alone.
//
// State is single-use: Consume deletes the entry atomically.
type StateStore struct {
	mu       sync.Mutex
	entries  map[string]stateEntry
	ttl      time.Duration
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewStateStore creates a StateStore with the given TTL. A background
// goroutine evicts expired entries every ttl/2 (minimum 1s).
func NewStateStore(ttl time.Duration) *StateStore {
	s := &StateStore{
		entries: make(map[string]stateEntry),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
		s.ttl = ttl
	}
	// Evict at half-TTL, but no more than once per second to avoid
	// burning CPU on tight TTLs.
	evictEvery := ttl / 2
	if evictEvery < time.Second {
		evictEvery = time.Second
	}
	go s.evictLoop(evictEvery)
	return s
}

// newStateStore is the package-internal alias used by tests.
func newStateStore(ttl time.Duration) *StateStore { return NewStateStore(ttl) }

// Issue generates a fresh (state, bind) pair, stores it under state,
// and returns both tokens. The bind is a 128-bit random hex string; it
// must be set as an HttpOnly cookie on the originating response.
func (s *StateStore) Issue() (state string, bind string, err error) {
	state, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	bind, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	s.mu.Lock()
	s.entries[state] = stateEntry{bind: bind, createdAt: now}
	s.mu.Unlock()
	return state, bind, nil
}

// Consume atomically looks up and deletes the entry for the given
// state, returning the bind token it was paired with. ok is false if
// the state is unknown, already consumed, or expired.
func (s *StateStore) Consume(state string) (bind string, ok bool) {
	if state == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, found := s.entries[state]
	if !found {
		return "", false
	}
	if time.Since(entry.createdAt) > s.ttl {
		delete(s.entries, state)
		return "", false
	}
	delete(s.entries, state)
	return entry.bind, true
}

// Stop shuts down the background eviction goroutine. Idempotent.
func (s *StateStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *StateStore) evictLoop(every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.evictExpired()
		}
	}
}

func (s *StateStore) evictExpired() {
	cutoff := time.Now().Add(-s.ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.entries {
		if e.createdAt.Before(cutoff) {
			delete(s.entries, k)
		}
	}
}

// randomToken returns a hex-encoded random token of n random bytes
// (so n=32 yields 64 hex chars / 256 bits).
func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
