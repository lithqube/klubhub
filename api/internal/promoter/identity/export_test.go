package identity

import (
	"time"

	"github.com/klubhub/dj/api/internal/platform/auth"
)

// TOTPCodeForTest exposes the current TOTP code to the external test package.
func TOTPCodeForTest(secret []byte, t time.Time) string { return auth.TOTPCode(secret, t) }
