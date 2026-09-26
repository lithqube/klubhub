package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SessionCookie is the session cookie name. The __Host- prefix forces
// Secure, Path=/ and no Domain, so subdomains cannot plant or read it.
const SessionCookie = "__Host-kh_session"

var errBadToken = errors.New("auth: malformed session token")

// SessionToken is a new random session credential. Only Hash is stored.
type SessionToken struct {
	Cookie string // v1.<tenant>.<base64url(32 random bytes)>
	Hash   []byte // sha256 of the random part
}

// NewSessionToken creates a session credential for tenant. The tenant is in
// the cookie so the lookup can run under that tenant's row level security;
// it is not secret.
func NewSessionToken(tenant uuid.UUID) (SessionToken, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return SessionToken{}, err
	}
	sum := sha256.Sum256(raw)
	return SessionToken{
		Cookie: "v1." + tenant.String() + "." + base64.RawURLEncoding.EncodeToString(raw),
		Hash:   sum[:],
	}, nil
}

// ParseSessionToken validates the cookie format and returns the tenant and
// the hash to look up.
func ParseSessionToken(cookie string) (uuid.UUID, []byte, error) {
	parts := strings.Split(cookie, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return uuid.Nil, nil, errBadToken
	}
	tenant, err := uuid.Parse(parts[1])
	if err != nil || tenant == uuid.Nil {
		return uuid.Nil, nil, errBadToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(raw) != 32 {
		return uuid.Nil, nil, errBadToken
	}
	sum := sha256.Sum256(raw)
	return tenant, sum[:], nil
}

// SetSessionCookie writes the session cookie.
func SetSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: value, Path: "/", Expires: expires,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie expires the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}
