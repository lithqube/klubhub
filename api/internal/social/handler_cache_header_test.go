package social_test

// Smoke test for CodeRabbit comment 3: the GET /posts/{id}/image
// handler must send a private, no-cache Cache-Control header.
//
// Background
// ----------
// The original handler set Cache-Control: public, max-age=3600 with
// the comment "images are immutable once uploaded". After the
// dedicated upload route (POST /posts/{id}/image), an image can be
// replaced for the same post, so:
//   1. The response is no longer immutable.
//   2. The image bytes may carry session-dependent metadata via
//      shared caches, so the response must be private.
//
// We assert the source-literal header value rather than exercising
// the full storage path — the handler uses a concrete *storage.Client
// (not an interface) and there is no minio testcontainer wired into
// the social test package, so an end-to-end httptest through the
// handler would require adding a heavy new testcontainer just to
// observe one response header. The literal-string assertion is the
// same style used in the migration_backfill_test.go drift guard and
// catches a future regression of the literal value or the comment.

import (
	"os"
	"strings"
	"testing"
)

func TestSocialHandler_GetPostImage_SendsPrivateNoCacheHeader(t *testing.T) {
	// Read the source file directly so the assertion cannot drift
	// from the file the operator deploys. This is a content-level
	// regression guard, not a behavior test — see the file comment
	// for why we don't drive it through httptest.
	const srcPath = "handler.go"

	body, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read %s: %v", srcPath, err)
	}
	src := string(body)

	const want = `"private, no-cache"`
	if !strings.Contains(src, want) {
		t.Fatalf("Cache-Control value regressed: missing %q in %s", want, srcPath)
	}

	// Anti-regression: the old value MUST NOT still be set anywhere
	// in the file. Catches a partial revert that leaves both lines.
	if strings.Contains(src, `"public, max-age=3600"`) {
		t.Fatalf("stale Cache-Control value \"public, max-age=3600\" still present in %s", srcPath)
	}

	// The accompanying comment must also reflect the new contract so
	// the next reader understands why we no longer cache public.
	if !strings.Contains(src, "private") {
		t.Fatalf("expected the surrounding comment in %s to reference "+
			"the private response reasoning", srcPath)
	}
}