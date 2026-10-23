package bookings

import "github.com/klubhub/dj/api/internal/platform/email"

// EmailKindBookingOffer is the outbox kind for promoter→DJ booking offers.
// Registered in the open email kind registry so enqueue accepts it and
// unknown kinds are still rejected.
const EmailKindBookingOffer email.EmailKind = "booking_offer"

// OwnerTypeBookingOffer is the email_messages owner_type for booking offers
// (the message is owned by the outreach_requests row).
const OwnerTypeBookingOffer = "booking_offer"

func init() { email.RegisterKind(EmailKindBookingOffer) }
