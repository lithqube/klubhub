package email

// OutreachGate decides whether the configured email provider may deliver
// unsolicited B2B outreach.
//
// Why this exists
//
// Booking outreach (both directions of Phase 04.10) is cold outreach by
// construction: the recipient is a contact in the operator's own database
// who has never opted in to hear from the sender. That is categorically
// different from transactional mail — an invoice receipt, a password reset,
// an agreement notification — which is what these applications already send.
//
// The distinction matters because most hosted email providers draw the line
// exactly there. Plunk's Terms of Service §5 is titled "NO COLD EMAILING
// POLICY" and prohibits "one-time cold outreach emails" outright, explicitly
// naming the case this feature implements:
//
//	You may NOT use Plunk to: ... Sending emails without documented consent
//	... "One-time" cold outreach emails
//
// Its transactional endpoint (/v1/send) is not an exemption: Plunk's own
// guide states that if you are unsure whether a message is transactional or
// marketing, you must treat it as marketing — which requires explicit opt-in.
//
// This code cannot read a provider's contract, and no API call can report
// one. The only sound source of truth is the operator. So the gate is an
// explicit, opt-in, fail-closed attestation: outreach is refused unless the
// operator has affirmatively declared that their provider permits it.
//
// What this does NOT do
//
// It is not a compliance certificate and it does not transfer legal
// responsibility. It records an operator decision. Enabling it still leaves
// the operator responsible for:
//   - a lawful basis for the sends (e.g. GDPR Art. 6(1)(f) legitimate
//     interest, with a balancing test and an LIA on file)
//   - honouring suppression/opt-out requests (the app enforces its own
//     suppression list; inbound handling is the operator's)
//   - the CAN-SPAM / GDPR / CASL obligations that apply to their recipients
//     in their jurisdictions
//
// Deliberately NOT gated: finance's invoice, receipt, agreement and payment
// email. Those are genuine transactional mail and are unaffected by this
// attestation. If this gate is off and finance email works, that is expected
// behaviour, not a bug.

// OutreachGate is the fail-closed capability check. The zero value refuses
// all outreach, which is the safe default for a new deployment.
type OutreachGate struct {
	// permitted mirrors OUTREACH_PROVIDER_PERMITS_OUTREACH.
	permitted bool
}

// NewOutreachGate returns a gate. permitted must be set from an explicit
// operator setting; do not infer it from whether a provider happens to be
// configured, because "a sender is configured" says nothing about whether
// its terms permit cold outreach.
func NewOutreachGate(permitted bool) *OutreachGate {
	return &OutreachGate{permitted: permitted}
}

// AllowsOutreach reports whether outreach sends may proceed.
func (g *OutreachGate) AllowsOutreach() bool {
	if g == nil {
		return false
	}
	return g.permitted
}