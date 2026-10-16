package outreach

import "github.com/klubhub/dj/api/internal/platform/email"

// EmailKindBookingRequest is the outbox kind for DJ→promoter booking
// requests. Registered in the open email kind registry so enqueue accepts
// it and unknown kinds are still rejected.
const EmailKindBookingRequest email.EmailKind = "booking_request"

// OwnerTypeOutreachRequest is the email_messages owner_type for sends
// without an EPK attachment (the message is owned by the request row).
const OwnerTypeOutreachRequest = "outreach_request"

func init() { email.RegisterKind(EmailKindBookingRequest) }
