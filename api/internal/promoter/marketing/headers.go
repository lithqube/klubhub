package marketing

import (
	"errors"
	"fmt"
	"net/textproto"
	"net/url"
	"strings"
)

// ErrInvalidUnsubscribeURL is returned for a target that RFC 8058 cannot use.
var ErrInvalidUnsubscribeURL = errors.New("marketing: invalid unsubscribe url")

// UnsubscribeHeaders returns the RFC 8058 one-click headers for a marketing
// send. Only marketing mail gets them; transactional senders never call this.
// The target is the provider-hosted unsubscribe URL (no native /u/:token page
// in OSS). It must be an absolute https URL with no control characters or
// angle brackets, which would break out of the header value.
func UnsubscribeHeaders(rawURL string) (textproto.MIMEHeader, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" ||
		strings.ContainsAny(rawURL, "<>\r\n\x00 ") {
		return nil, fmt.Errorf("%w: need an absolute https url", ErrInvalidUnsubscribeURL)
	}
	h := textproto.MIMEHeader{}
	h.Set("List-Unsubscribe", "<"+rawURL+">")
	h.Set("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	return h, nil
}
