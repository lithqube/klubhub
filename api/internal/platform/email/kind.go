package email

import "sync"

// EmailKind enumerates the kinds of transactional emails the system sends.
// The registry is open: each consumer package registers its own kinds in
// init() (finance registers invoice_*/agreement_*, outreach registers
// booking_request/booking_offer). Unknown kinds are rejected at enqueue.
type EmailKind string

var (
	kindMu  sync.RWMutex
	kindReg = map[EmailKind]struct{}{}
)

// RegisterKind adds a kind to the open registry. Consumers call it from init().
func RegisterKind(k EmailKind) {
	kindMu.Lock()
	kindReg[k] = struct{}{}
	kindMu.Unlock()
}

// IsValid reports whether the kind has been registered.
func (k EmailKind) IsValid() bool {
	kindMu.RLock()
	defer kindMu.RUnlock()
	_, ok := kindReg[k]
	return ok
}
