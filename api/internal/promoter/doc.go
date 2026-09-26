// Package promoter is the root of the KlubHub Promoter backend. Domain
// packages live below it (events, guestlist, door, audience, ...); shared
// infrastructure lives in internal/platform. Domain code receives a pgx.Tx
// from platform/tenantdb and never touches the connection pool directly
// (see dbaccess_guard_test.go).
package promoter
