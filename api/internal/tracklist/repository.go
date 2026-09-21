package tracklist

import (
	"context"
	"errors"
	"strconv"
	"strings"

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
					artwork_source, created_at, updated_at, deleted_at
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
				)`
			for _, track := range tracks {
				_, err := tx.Exec(ctx, trackQuery,
					track.ID, track.TracklistID, track.Position, track.Title, track.Artist, track.Album, track.Genre, track.BPM, track.Rating,
					track.DurationSeconds, track.MusicalKey, track.DateAdded, track.ArtworkStatus, track.ArtworkURL, track.ArtworkSource,
					track.CreatedAt, track.UpdatedAt, track.DeletedAt,
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
		       artwork_source, created_at, updated_at, deleted_at
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
