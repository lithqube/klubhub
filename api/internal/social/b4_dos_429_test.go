package social_test

// Regression tests pinning the B.4 fix-subagent findings:
//
// 1. DoS via missing-cookie state consumption.
//    An attacker without the OAuthStateCookieName cookie MUST NOT
//    be able to consume a legitimate user's pending OAuth state.
//    The handler now rejects missing/empty bind cookies before
//    forwarding to the service, and the service layer additionally
//    refuses to consume state when bind == "" (defense-in-depth).
//
// 2. 429 sanitization bypass in the publish worker.
//    When Instagram returns HTTP 429, the worker persists the
//    failure reason via SanitizeTransportError, not verbatim.
//
// These tests pin the structural invariants so future refactors
// cannot reintroduce either bug.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/social"
)

// --- Issue 1: DoS via missing-cookie state consumption ---

// TestOAuthDoS_MissingCookieDoesNotConsumeState asserts that an
// unauthenticated caller with the state string but NO bind cookie
// cannot evict the legitimate user's pending state from the store.
//
// Steps:
//  1. Issue a state via /auth/url (so the store has a real entry).
//  2. Replay the state via /auth/callback with NO cookie.
//  3. Expect 400 with no DB writes and no state eviction.
//  4. Re-issue the same state and use the legitimate cookie path
//     to confirm the store still works.
func TestOAuthDoS_MissingCookieDoesNotConsumeState(t *testing.T) {
	store := social.NewStateStore(10 * time.Minute)
	defer store.Stop()
	repo := &realServiceRepo{}
	router := newRealServiceRouter(repo, store)

	// Step 1: issue via the real handler.
	issueReq := httptest.NewRequest(http.MethodGet, "/auth/url", nil)
	issueW := httptest.NewRecorder()
	router.ServeHTTP(issueW, issueReq)
	if issueW.Code != http.StatusOK {
		t.Fatalf("issue failed: %d %s", issueW.Code, issueW.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(issueW.Body).Decode(&body); err != nil {
		t.Fatalf("decode issue body: %v", err)
	}
	state := extractState(t, body["url"])
	bind := findCookie(t, issueW.Result(), social.OAuthStateCookieName)

	// Step 2: replay WITHOUT cookie. This must be rejected at the
	// handler boundary BEFORE the state is consumed.
	attackReq := httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state="+state, nil)
	// Intentionally do NOT add the bind cookie.
	attackW := httptest.NewRecorder()
	router.ServeHTTP(attackW, attackReq)
	if attackW.Code != http.StatusBadRequest {
		t.Fatalf("attacker without cookie must get 400, got %d body=%s",
			attackW.Code, attackW.Body.String())
	}
	if !strings.Contains(attackW.Body.String(), "oauth state cookie") &&
		!strings.Contains(attackW.Body.String(), "invalid or expired") {
		t.Errorf("expected cookie/state error message, got: %s", attackW.Body.String())
	}

	// Step 3: NO DB writes on the attack path.
	if repo.upsertCalls != 0 {
		t.Errorf("expected zero UpsertAccount calls on attack path, got %d", repo.upsertCalls)
	}

	// Step 4: confirm the legitimate state is STILL pending.
	// The user's callback with the correct cookie must NOT yet be
	// 400 — it will eventually fail because the test environment
	// can't reach api.instagram.com, but the failure code must NOT
	// be 400 (which would indicate the state was already consumed).
	legitReq := httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state="+state, nil)
	legitReq.AddCookie(bind)
	legitW := httptest.NewRecorder()
	router.ServeHTTP(legitW, legitReq)
	if legitW.Code == http.StatusBadRequest {
		t.Fatalf("legitimate user callback was rejected — state was consumed by attacker! body=%s",
			legitW.Body.String())
	}
}

// TestOAuthDoS_EmptyCookieValueDoesNotConsumeState asserts that a
// non-empty cookie NAME with an empty VALUE is treated the same as
// a missing cookie. The store must not be touched.
func TestOAuthDoS_EmptyCookieValueDoesNotConsumeState(t *testing.T) {
	store := social.NewStateStore(10 * time.Minute)
	defer store.Stop()
	repo := &realServiceRepo{}
	router := newRealServiceRouter(repo, store)

	issueReq := httptest.NewRequest(http.MethodGet, "/auth/url", nil)
	issueW := httptest.NewRecorder()
	router.ServeHTTP(issueW, issueReq)
	var body map[string]string
	if err := json.NewDecoder(issueW.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	state := extractState(t, body["url"])

	attackReq := httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state="+state, nil)
	attackReq.AddCookie(&http.Cookie{Name: social.OAuthStateCookieName, Value: ""})
	attackW := httptest.NewRecorder()
	router.ServeHTTP(attackW, attackReq)
	if attackW.Code != http.StatusBadRequest {
		t.Fatalf("empty cookie must be 400, got %d body=%s",
			attackW.Code, attackW.Body.String())
	}
	if repo.upsertCalls != 0 {
		t.Errorf("expected zero writes, got %d", repo.upsertCalls)
	}
}

// TestOAuthDoS_ServiceLayerDefenseInDepth asserts that even if a
// future caller forgets the handler-level check, the service layer
// itself refuses to consume state when bind == "".
func TestOAuthDoS_ServiceLayerDefenseInDepth(t *testing.T) {
	store := social.NewStateStore(10 * time.Minute)
	defer store.Stop()
	repo := &realServiceRepo{}
	svc := social.NewServiceWithState(repo, social.ServiceConfig{
		InstagramAppID:       "test_app_id",
		InstagramRedirectURI: "https://example.com/cb",
		TokenEncryptionKey:   []byte("0123456789abcdef0123456789abcdef"),
	}, store)

	state, bind, err := store.Issue()
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Even with a direct service call, empty bind must not consume state.
	_, err = svc.HandleOAuthCallback(context.Background(), "code", state, "")
	if err == nil {
		t.Fatal("expected ErrOAuthStateInvalid for empty bind")
	}
	if !errors.Is(err, social.ErrOAuthStateInvalid) {
		t.Fatalf("expected ErrOAuthStateInvalid, got %v", err)
	}

	// State must still be consumable by the legitimate bind.
	gotBind, ok := store.Consume(state)
	if !ok {
		t.Fatal("state was consumed despite empty bind — DoS regression")
	}
	if gotBind != bind {
		t.Fatalf("bind mismatch: got %q want %q", gotBind, bind)
	}
}

// --- Issue 2: 429 sanitization bypass in publish worker ---

// 429 sanitization at the worker boundary is pinned by
// TestWorker_RecordRateLimitSanitizesReason in worker_test.go, which
// uses a taintedRateLimitError wrapper around *RateLimitError and
// asserts the credential substring is stripped from the persisted
// reason. That test catches regressions to err.Error() at the call
// site (worker.go:202 historically). This file therefore focuses on
// the DoS-fix coverage.

// keep these imports referenced even if some tests are removed.
var (
	_ = uuid.New
)
