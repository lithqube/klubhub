package sealed

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

const banColumns = `id, key_version, entry_sealed, expires_at, created_at, updated_at`

func scanBan(r pgx.Row) (BanEntry, error) {
	var e BanEntry
	var blob []byte
	err := r.Scan(&e.ID, &e.KeyVersion, &blob, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt)
	e.EntrySealed = Encode(blob)
	e.ExpiresAt, e.CreatedAt, e.UpdatedAt = e.ExpiresAt.UTC(), e.CreatedAt.UTC(), e.UpdatedAt.UTC()
	return e, err
}

// BanList returns the active key version and the unexpired entries, each
// with the display name of the staff member who added it (created_by_name;
// null when that account is gone or its name does not open). The name is
// opened with the tenant's envelope key, like the recipients list.
func (s *Service) BanList(ctx context.Context) (BanList, error) {
	out := BanList{Entries: []BanEntry{}}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if out.KeyVersion, err = activeVersion(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT b.id, b.key_version, b.entry_sealed, b.expires_at, b.created_at, b.updated_at, u.id, u.display_name_enc
		  FROM ban_entries b LEFT JOIN users u ON u.id = b.created_by
		  WHERE b.expires_at > $1 ORDER BY b.created_at, b.id`, s.clock())
		if err != nil {
			return err
		}
		type banRow struct {
			BanEntry
			by    *uuid.UUID
			nameE []byte
		}
		list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (banRow, error) {
			var e banRow
			var blob []byte
			err := r.Scan(&e.ID, &e.KeyVersion, &blob, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt, &e.by, &e.nameE)
			e.EntrySealed = Encode(blob)
			e.ExpiresAt, e.CreatedAt, e.UpdatedAt = e.ExpiresAt.UTC(), e.CreatedAt.UTC(), e.UpdatedAt.UTC()
			return e, err
		})
		if err != nil {
			return err
		}
		names := map[uuid.UUID]*string{}
		for _, e := range list {
			if e.by != nil {
				name, seen := names[*e.by]
				if !seen {
					// A name that does not open is left out rather than
					// failing the list the door and the managers rely on.
					if n, err := s.open(ctx, tx, tenant, "display_name_enc", *e.by, e.nameE); err == nil && n != "" {
						name = &n
					}
					names[*e.by] = name
				}
				e.CreatedByName = name
			}
			out.Entries = append(out.Entries, e.BanEntry)
		}
		return nil
	})
	return out, err
}

// requireActive refuses writes sealed under anything but the active key
// version (409 not_setup / key_version_stale).
func requireActive(ctx context.Context, tx pgx.Tx, version int) error {
	v, err := activeVersion(ctx, tx)
	if err != nil {
		return err
	}
	if v == nil {
		return conflict("not_setup")
	}
	if *v != version {
		return conflict("key_version_stale")
	}
	return nil
}

func (s *Service) banWritten(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, p authz.Principal, id uuid.UUID, action string, version int) error {
	if err := s.emit(ctx, tx, tenant, "banlist", "updated", map[string]uuid.UUID{"entry_id": id}); err != nil {
		return err
	}
	reason := ""
	if version > 0 {
		reason = fmt.Sprintf("key version %d", version)
	}
	return record(ctx, tx, tenant, p.Sub, action, "ban_entry:"+id.String(), reason)
}

// AddBan stores a new entry under the client's id.
func (s *Service) AddBan(ctx context.Context, p authz.Principal, in BanInput) (BanEntry, error) {
	var out BanEntry
	if in.ID == nil || *in.ID == uuid.Nil {
		return out, invalid("id", "required (client uuid)")
	}
	e, err := in.parse(s.clock())
	if err != nil {
		return out, err
	}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		if err := requireActive(ctx, tx, e.keyVersion); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM ban_entries`).Scan(&n); err != nil {
			return err
		}
		if n >= MaxBanEntries {
			return conflict("ban_list_full")
		}
		now := s.clock()
		out, err = scanBan(tx.QueryRow(ctx, `INSERT INTO ban_entries (id, tenant_id, key_version, entry_sealed, expires_at, created_by, created_at, updated_at)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, $7) ON CONFLICT (id) DO NOTHING RETURNING `+banColumns,
			*in.ID, tenant, e.keyVersion, e.blob, e.expiresAt, createdBy(p), now))
		if errors.Is(err, pgx.ErrNoRows) {
			return conflict("ban_entry_exists")
		}
		if err != nil {
			return err
		}
		return s.banWritten(ctx, tx, tenant, p, out.ID, "banlist.entry_added", e.keyVersion)
	})
	return out, err
}

// UpdateBan replaces an unexpired entry's ciphertext and expiry.
func (s *Service) UpdateBan(ctx context.Context, p authz.Principal, id uuid.UUID, in BanInput) (BanEntry, error) {
	var out BanEntry
	if in.ID != nil && *in.ID != id {
		return out, invalid("id", "must match the URL")
	}
	e, err := in.parse(s.clock())
	if err != nil {
		return out, err
	}
	tenant, err := tenantOf(ctx)
	if err != nil {
		return out, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		if err := requireActive(ctx, tx, e.keyVersion); err != nil {
			return err
		}
		now := s.clock()
		out, err = scanBan(tx.QueryRow(ctx, `UPDATE ban_entries SET key_version = $2, entry_sealed = $3, expires_at = $4, updated_at = $5
		  WHERE id = $1 AND expires_at > $5 RETURNING `+banColumns, id, e.keyVersion, e.blob, e.expiresAt, now))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return s.banWritten(ctx, tx, tenant, p, id, "banlist.entry_updated", e.keyVersion)
	})
	return out, err
}

// DeleteBan removes an entry.
func (s *Service) DeleteBan(ctx context.Context, p authz.Principal, id uuid.UUID) error {
	tenant, err := tenantOf(ctx)
	if err != nil {
		return err
	}
	return s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM ban_entries WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return s.banWritten(ctx, tx, tenant, p, id, "banlist.entry_removed", 0)
	})
}
