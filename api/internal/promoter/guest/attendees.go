package guest

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// tickets lists an event's imported ticket holders for the guest table,
// decrypted. q is an exact lookup through the blind indexes, as for guests.
func (s *Service) tickets(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, q string) ([]Ticket, error) {
	var emailIdx, nameIdx []byte
	if q != "" {
		dek, err := s.keys.Current(ctx, tx, tenantOf(ctx))
		if err != nil {
			return nil, err
		}
		if strings.Contains(q, "@") {
			emailIdx = attendeeEmailIndex(dek, q)
		} else {
			nameIdx = attendeeNameIndex(dek, q)
		}
	}
	rows, err := tx.Query(ctx, `SELECT p.id, p.order_id, o.source, o.external_ref, p.ticket_type_id, t.name, p.attendee_name_enc,
	  p.attendee_email_enc, p.status, p.imported_at, p.purged_at IS NOT NULL,
	  (SELECT min(c.at) FROM checkins c WHERE c.position_id = p.id AND c.direction = 'in' AND c.undone_at IS NULL)
	  FROM order_positions p JOIN orders o ON o.id = p.order_id JOIN ticket_types t ON t.id = p.ticket_type_id
	  WHERE p.event_id = $1 AND ($2::bytea IS NULL OR p.attendee_email_bidx = $2) AND ($3::bytea IS NULL OR p.attendee_name_bidx = $3)
	  ORDER BY o.external_ref, p.external_ref, p.id LIMIT $4`, eventID, emailIdx, nameIdx, MaxTicketsPerEvent)
	if err != nil {
		return nil, err
	}
	type raw struct {
		Ticket
		name, email []byte
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (raw, error) {
		var x raw
		err := r.Scan(&x.ID, &x.OrderID, &x.Source, &x.OrderRef, &x.TicketTypeID, &x.TicketType, &x.name, &x.email, &x.Status, &x.ImportedAt, &x.Purged, &x.FirstInAt)
		x.CheckedIn = x.FirstInAt != nil
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out := make([]Ticket, len(list))
	for i, x := range list {
		out[i] = x.Ticket
		if out[i].Name, err = s.open(ctx, tx, "order_positions", "attendee_name_enc", x.ID, x.name); err != nil {
			return nil, err
		}
		if out[i].Email, err = s.open(ctx, tx, "order_positions", "attendee_email_enc", x.ID, x.email); err != nil {
			return nil, err
		}
	}
	return out, nil
}
