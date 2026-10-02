package tracklist

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides access to tracklist data
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository with the given connection pool
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create saves a tracklist and its tracks in a transaction
func (r *Repository) Create(ctx context.Context, tl *Tracklist, tracks []Track) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		// Insert tracklist
		query := `
			INSERT INTO tracklists (
				id, title, source_format, raw_file_path, preset, visible_fields, 
				bg_mode, bg_value, max_tracks, track_range_start, track_range_end,
				created_at, updated_at, deleted_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
			)`
		_, err := tx.Exec(ctx, query,
			tl.ID, tl.Title, tl.SourceFormat, tl.RawFilePath, tl.Preset, tl.VisibleFields,
			tl.BgMode, tl.BgValue, tl.MaxTracks, tl.TrackRangeStart, tl.TrackRangeEnd,
			tl.CreatedAt, tl.UpdatedAt, tl.DeletedAt,
		)
		if err != nil {
			return err
		}

		// Insert tracks
		if len(tracks) > 0 {
			trackQuery := `
				INSERT INTO tracks (
					id, tracklist_id, position, title, artist, album, genre, bpm, rating, 
					duration_secs, musical_key, date_added, artwork_status, artwork_url, 
					artwork_source, created_at, updated_at, deleted_at,
					hidden_gem, unreleased, media
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
				)`
			for _, track := range tracks {
				_, err := tx.Exec(ctx, trackQuery,
					track.ID, track.TracklistID, track.Position, track.Title, track.Artist, track.Album, track.Genre, track.BPM, track.Rating,
					track.DurationSeconds, track.MusicalKey, track.DateAdded, track.ArtworkStatus, track.ArtworkURL, track.ArtworkSource,
					track.CreatedAt, track.UpdatedAt, track.DeletedAt,
					track.HiddenGem, track.Unreleased, track.Media,
				)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// Get returns a tracklist with its tracks by ID
func (r *Repository) Get(ctx context.Context, id uuid.UUID) (*Tracklist, []Track, error) {
	// Get tracklist
	var tl Tracklist
	err := r.pool.QueryRow(ctx, `
		SELECT id, title, source_format, raw_file_path, preset, visible_fields, 
		       bg_mode, bg_value, max_tracks, track_range_start, track_range_end,
		       created_at, updated_at, deleted_at
		FROM tracklists 
		WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(
		&tl.ID, &tl.Title, &tl.SourceFormat, &tl.RawFilePath, &tl.Preset, &tl.VisibleFields,
		&tl.BgMode, &tl.BgValue, &tl.MaxTracks, &tl.TrackRangeStart, &tl.TrackRangeEnd,
		&tl.CreatedAt, &tl.UpdatedAt, &tl.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	// Get tracks
	var tracks []Track
	rows, err := r.pool.Query(ctx, `
		SELECT id, tracklist_id, position, title, artist, album, genre, bpm, rating, 
		       duration_secs, musical_key, date_added, artwork_status, artwork_url, 
		       artwork_source, created_at, updated_at, deleted_at,
		       hidden_gem, unreleased, media
		FROM tracks
		WHERE tracklist_id = $1 AND deleted_at IS NULL
		ORDER BY position`,
		id,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var t Track
		err := rows.Scan(
			&t.ID, &t.TracklistID, &t.Position, &t.Title, &t.Artist, &t.Album, &t.Genre, &t.BPM, &t.Rating,
			&t.DurationSeconds, &t.MusicalKey, &t.DateAdded, &t.ArtworkStatus, &t.ArtworkURL, &t.ArtworkSource,
			&t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
			&t.HiddenGem, &t.Unreleased, &t.Media,
		)
		if err != nil {
			return nil, nil, err
		}
		tracks = append(tracks, t)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	return &tl, tracks, nil
}

// UpdateTitle renames a live tracklist. ErrNotFound if it does not exist or is deleted.
func (r *Repository) UpdateTitle(ctx context.Context, id uuid.UUID, title string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE tracklists SET title = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`,
		id, title,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LinkedGigs lists the live gigs a tracklist is linked to, newest first.
// ErrNotFound if the tracklist does not exist. The link itself is managed by
// the gig endpoints (POST/DELETE /gigs/{id}/tracklists/{tracklistId}).
func (r *Repository) LinkedGigs(ctx context.Context, id uuid.UUID) ([]LinkedGig, error) {
	exists, err := r.Exists(ctx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT g.id, g.date, g.venue, g.city, g.event_name
		FROM gigs g
		JOIN tracklist_gigs tg ON tg.gig_id = g.id
		WHERE tg.tracklist_id = $1 AND g.deleted_at IS NULL
		ORDER BY g.date DESC, g.id`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	gigs := make([]LinkedGig, 0)
	for rows.Next() {
		var g LinkedGig
		if err := rows.Scan(&g.ID, &g.Date, &g.Venue, &g.City, &g.EventName); err != nil {
			return nil, err
		}
		gigs = append(gigs, g)
	}
	return gigs, rows.Err()
}

// Exists checks whether a tracklist exists (not soft-deleted).
func (r *Repository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM tracklists WHERE id = $1 AND deleted_at IS NULL)`,
		id,
	).Scan(&exists)
	return exists, err
}
func (r *Repository) List(ctx context.Context) ([]Tracklist, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, title, source_format, raw_file_path, preset, visible_fields, 
		       bg_mode, bg_value, max_tracks, track_range_start, track_range_end,
		       created_at, updated_at, deleted_at
		FROM tracklists 
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracklists []Tracklist
	for rows.Next() {
		var tl Tracklist
		err := rows.Scan(
			&tl.ID, &tl.Title, &tl.SourceFormat, &tl.RawFilePath, &tl.Preset, &tl.VisibleFields,
			&tl.BgMode, &tl.BgValue, &tl.MaxTracks, &tl.TrackRangeStart, &tl.TrackRangeEnd,
			&tl.CreatedAt, &tl.UpdatedAt, &tl.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		tracklists = append(tracklists, tl)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tracklists, nil
}

// UpdateTrack updates only non-nil fields of a track
func (r *Repository) UpdateTrack(ctx context.Context, tracklistID, trackID uuid.UUID, req UpdateTrackRequest) error {
	// Build dynamic SET clause for non-nil fields
	setClauses := []string{}
	args := []interface{}{}
	argID := 1

	if req.Title != nil {
		setClauses = append(setClauses, "title = $"+strconv.Itoa(argID))
		args = append(args, *req.Title)
		argID++
	}
	if req.Artist != nil {
		setClauses = append(setClauses, "artist = $"+strconv.Itoa(argID))
		args = append(args, *req.Artist)
		argID++
	}
	if req.Album != nil {
		setClauses = append(setClauses, "album = $"+strconv.Itoa(argID))
		args = append(args, *req.Album)
		argID++
	}
	if req.Genre != nil {
		setClauses = append(setClauses, "genre = $"+strconv.Itoa(argID))
		args = append(args, *req.Genre)
		argID++
	}
	if req.BPM != nil {
		setClauses = append(setClauses, "bpm = $"+strconv.Itoa(argID))
		args = append(args, *req.BPM)
		argID++
	}
	if req.Rating != nil {
		setClauses = append(setClauses, "rating = $"+strconv.Itoa(argID))
		args = append(args, *req.Rating)
		argID++
	}
	if req.MusicalKey != nil {
		setClauses = append(setClauses, "musical_key = $"+strconv.Itoa(argID))
		args = append(args, *req.MusicalKey)
		argID++
	}
	if req.DurationSeconds != nil {
		setClauses = append(setClauses, "duration_secs = $"+strconv.Itoa(argID))
		args = append(args, *req.DurationSeconds)
		argID++
	}
	if req.DateAdded != nil {
		setClauses = append(setClauses, "date_added = $"+strconv.Itoa(argID))
		args = append(args, *req.DateAdded)
		argID++
	}
	if req.HiddenGem != nil {
		setClauses = append(setClauses, "hidden_gem = $"+strconv.Itoa(argID))
		args = append(args, *req.HiddenGem)
		argID++
	}
	if req.Unreleased != nil {
		setClauses = append(setClauses, "unreleased = $"+strconv.Itoa(argID))
		args = append(args, *req.Unreleased)
		argID++
	}
	if req.Media != nil {
		setClauses = append(setClauses, "media = $"+strconv.Itoa(argID))
		args = append(args, *req.Media)
		argID++
	}

	if len(setClauses) == 0 {
		return nil // Nothing to update
	}

	setClause := strings.Join(setClauses, ", ")
	query := `
		UPDATE tracks 
		SET ` + setClause + `, updated_at = NOW()
		WHERE id = $` + strconv.Itoa(argID) + ` AND tracklist_id = $` + strconv.Itoa(argID+1) + ` AND deleted_at IS NULL`

	args = append(args, trackID, tracklistID)

	result, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// lockTracklist takes the tracklist row lock (serialising adds and reorders on
// one tracklist) and returns ErrNotFound if it does not exist or is deleted.
func lockTracklist(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var locked uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM tracklists WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// AddTrack appends a manually entered track after the last one. req must
// already be normalised. Soft-deleted tracks keep their position, so the next
// position is taken over all rows.
func (r *Repository) AddTrack(ctx context.Context, tracklistID uuid.UUID, req CreateTrackRequest) (*Track, error) {
	now := time.Now().UTC()
	t := &Track{
		ID: uuid.New(), TracklistID: tracklistID, Title: req.Title, Artist: req.Artist,
		MusicalKey: req.MusicalKey, DateAdded: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
		// A hand-typed track has nothing to look up, so it skips the artwork fetch.
		ArtworkStatus: ArtworkPlaceholder,
		HiddenGem:     req.HiddenGem, Unreleased: req.Unreleased, Media: req.Media,
		CreatedAt: now, UpdatedAt: now,
	}
	if req.BPM != nil {
		t.BPM = *req.BPM
	}
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockTracklist(ctx, tx, tracklistID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(position), 0) + 1 FROM tracks WHERE tracklist_id = $1`,
			tracklistID).Scan(&t.Position); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO tracks (
				id, tracklist_id, position, title, artist, album, genre, bpm, rating,
				duration_secs, musical_key, date_added, artwork_status, artwork_url,
				artwork_source, created_at, updated_at, hidden_gem, unreleased, media
			) VALUES ($1, $2, $3, $4, $5, '', '', $6, 0, 0, $7, $8, $9, '', '', $10, $10, $11, $12, $13)`,
			t.ID, tracklistID, t.Position, t.Title, t.Artist, t.BPM, t.MusicalKey, t.DateAdded,
			t.ArtworkStatus, now, t.HiddenGem, t.Unreleased, t.Media,
		)
		return err
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ReorderTracks sets the order of a tracklist's live tracks to ids, which must
// be exactly the live track ids (ErrInvalidTrack otherwise). Live tracks end up
// at 1..n; soft-deleted rows are renumbered after them so they never collide.
func (r *Repository) ReorderTracks(ctx context.Context, tracklistID uuid.UUID, ids []uuid.UUID) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockTracklist(ctx, tx, tracklistID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id FROM tracks WHERE tracklist_id = $1 AND deleted_at IS NULL`, tracklistID)
		if err != nil {
			return err
		}
		live := map[uuid.UUID]bool{}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			live[id] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		seen := make(map[uuid.UUID]bool, len(ids))
		strs := make([]string, 0, len(ids))
		for _, id := range ids {
			if !live[id] || seen[id] {
				return ErrInvalidTrack
			}
			seen[id] = true
			strs = append(strs, id.String())
		}
		if len(ids) != len(live) {
			return ErrInvalidTrack
		}

		// Rows trade places mid-update; check uniqueness once, at commit.
		if _, err := tx.Exec(ctx, `SET CONSTRAINTS tracks_tracklist_position_key DEFERRED`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE tracks t SET position = v.pos::int, updated_at = NOW()
			FROM unnest($2::uuid[]) WITH ORDINALITY AS v(id, pos)
			WHERE t.id = v.id AND t.tracklist_id = $1`, tracklistID, strs); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE tracks t SET position = $2 + d.rn::int
			FROM (SELECT id, row_number() OVER (ORDER BY position, id) AS rn
			      FROM tracks WHERE tracklist_id = $1 AND deleted_at IS NOT NULL) d
			WHERE t.id = d.id`, tracklistID, len(ids))
		return err
	})
}

// SoftDelete sets deleted_at on tracklist and all child tracks in a transaction
func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		// Soft delete tracklist
		_, err := tx.Exec(ctx, `
			UPDATE tracklists 
			SET deleted_at = NOW() 
			WHERE id = $1 AND deleted_at IS NULL`,
			id,
		)
		if err != nil {
			return err
		}

		// Soft delete tracks
		_, err = tx.Exec(ctx, `
			UPDATE tracks 
			SET deleted_at = NOW() 
			WHERE tracklist_id = $1 AND deleted_at IS NULL`,
			id,
		)
		if err != nil {
			return err
		}

		// TODO(Phase 4): also UPDATE gigs SET tracklist_id = NULL WHERE tracklist_id = $1 once gigs table exists (TRKL-27)

		return nil
	})
}

// UpdateArtworkStatus updates the artwork status for a track
func (r *Repository) UpdateArtworkStatus(ctx context.Context, trackID uuid.UUID, status, url, source string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE tracks 
		SET artwork_status = $1, artwork_url = $2, artwork_source = $3, updated_at = NOW()
		WHERE id = $4 AND deleted_at IS NULL`,
		status, url, source, trackID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// SaveManualArtwork saves manual artwork for a track
func (r *Repository) SaveManualArtwork(ctx context.Context, trackID uuid.UUID, artworkURL string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE tracks 
		SET artwork_url = $1, artwork_source = 'manual', artwork_status = 'manual', updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL`,
		artworkURL, trackID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
