package social

import (
	"strings"
	"testing"
)

// TestSanitizeTransportError_StripsAccessTokenQueryParam verifies that an
// error string containing "?access_token=secret" loses the value while
// keeping non-secret URL context intact.
func TestSanitizeTransportError_StripsAccessTokenQueryParam(t *testing.T) {
	in := `unexpected status 400: https://graph.instagram.com/v22.0/refresh_access_token?grant_type=ig_refresh_token&access_token=SECRET_TOKEN_ABC`
	out := SanitizeTransportError(in)
	if strings.Contains(out, "SECRET_TOKEN_ABC") {
		t.Errorf("secret must be redacted, got: %s", out)
	}
	if !strings.Contains(out, "graph.instagram.com") {
		t.Errorf("URL host must be preserved, got: %s", out)
	}
}

// TestSanitizeTransportError_StripsClientSecretInBody verifies that error
// strings containing client_secret in body form are also sanitized.
func TestSanitizeTransportError_StripsClientSecretInBody(t *testing.T) {
	in := `short token exchange: parse error client_secret=shhh-not-telling status 400`
	out := SanitizeTransportError(in)
	if strings.Contains(out, "shhh-not-telling") {
		t.Errorf("client_secret must be redacted, got: %s", out)
	}
}

// TestSanitizeTransportError_PreservesGenericMessage ensures non-secret
// error text passes through unchanged.
func TestSanitizeTransportError_PreservesGenericMessage(t *testing.T) {
	in := "rate limited by instagram; retry after 15m"
	out := SanitizeTransportError(in)
	if out != in {
		t.Errorf("expected unchanged, got: %s", out)
	}
}

// TestSanitizeTransportError_StripsURLFragment verifies a credential that
// appears in URL fragment form (rare but seen in error chains) is
// redacted.
func TestSanitizeTransportError_StripsURLFragment(t *testing.T) {
	in := "exchange failed: https://api.instagram.com/oauth/access_token?client_id=APP123&client_secret=SECRET123&grant_type=authorization_code#token=XYZ"
	out := SanitizeTransportError(in)
	for _, secret := range []string{"APP123", "SECRET123", "XYZ"} {
		if strings.Contains(out, secret) {
			t.Errorf("expected %q to be redacted from %q", secret, out)
		}
	}
}
