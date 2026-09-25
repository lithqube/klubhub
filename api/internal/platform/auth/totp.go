package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP is defined over HMAC-SHA-1; authenticator apps expect it.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"time"
)

const (
	totpPeriod = 30
	totpDigits = 6
)

// NewTOTPSecret returns a random 160-bit TOTP secret. Store it sealed
// (envelope, `personal` class), never in plaintext.
func NewTOTPSecret() ([]byte, error) {
	s := make([]byte, 20)
	_, err := rand.Read(s)
	return s, err
}

func totpCode(secret []byte, t time.Time, digits int) string {
	return hotp(secret, uint64(t.Unix()/totpPeriod), digits)
}

func hotp(secret []byte, counter uint64, digits int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for range digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, v%mod)
}

// VerifyTOTP accepts the code for the current step or one step either side.
// lastStep is the last accepted step for this user; codes at or before it
// are rejected so a code cannot be replayed. Returns the matched step.
func VerifyTOTP(secret []byte, code string, now time.Time, lastStep int64) (int64, bool) {
	cur := now.Unix() / totpPeriod
	for _, step := range []int64{cur - 1, cur, cur + 1} {
		if step <= lastStep || step < 0 {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotp(secret, uint64(step), totpDigits)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// TOTPProvisioningURI is the otpauth:// URI shown as a QR code at enrolment.
func TOTPProvisioningURI(secret []byte, issuer, account string) string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {enc}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}
