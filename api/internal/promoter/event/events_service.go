package event

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/klubhub/dj/api/internal/platform/envelope"
)

// EventInput creates or replaces an event's details. Times are absolute
// instants; the client resolves "ends 07:00" to the next day (overnight).
type EventInput struct {
	Title             string     `json:"title"`
	Slug              string     `json:"slug"`
	StartsAt          time.Time  `json:"starts_at"`
	EndsAt            time.Time  `json:"ends_at"`
	DoorsAt           *time.Time `json:"doors_at"`
	Timezone          string     `json:"timezone"`
	VenueID           *uuid.UUID `json:"venue_id"`
	City              string     `json:"city"`
	LocationMode      string     `json:"location_mode"`
	LocationRevealAt  *time.Time `json:"location_reveal_at"`
	Visibility        string     `json:"visibility"`
	PublishAt         *time.Time `json:"publish_at"`
	MinAge            *int       `json:"min_age"`
	Genres            []string   `json:"genres"`
	DescriptionMD     string     `json:"description_md"`
	CostText          string     `json:"cost_text"`
	ExternalTicketURL *string    `json:"external_ticket_url"`
	Capacity          *int       `json:"capacity"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,80}$`)

func (in *EventInput) validate() error {
	in.Title = strings.TrimSpace(in.Title)
	in.City = strings.TrimSpace(in.City)
	if in.LocationMode == "" {
		in.LocationMode = LocationVenue
	}
	if in.Visibility == "" {
		in.Visibility = "public"
	}
	switch {
	case in.Title == "" || len(in.Title) > 200:
		return invalid("title", "required, at most 200 characters")
	case in.StartsAt.IsZero() || in.EndsAt.IsZero():
		return invalid("starts_at", "start and end are required")
	case !in.EndsAt.After(in.StartsAt):
		return invalid("ends_at", "must be after the start")
	case in.EndsAt.Sub(in.StartsAt) > 96*time.Hour:
		return invalid("ends_at", "an event lasts at most 4 days")
	case in.DoorsAt != nil && in.DoorsAt.After(in.StartsAt):
		return invalid("doors_at", "doors open before or at the start")
	case !slices.Contains([]string{LocationVenue, LocationCityOnly, LocationSecret}, in.LocationMode):
		return invalid("location_mode", "venue, city_only or secret")
	case in.VenueID == nil && in.City == "":
		return invalid("city", "choose a venue or enter a city")
	case in.LocationMode != LocationSecret && in.LocationRevealAt != nil:
		return invalid("location_reveal_at", "only secret locations have a reveal time")
	case !slices.Contains([]string{"public", "unlisted", "private"}, in.Visibility):
		return invalid("visibility", "public, unlisted or private")
	case in.MinAge != nil && (*in.MinAge < 0 || *in.MinAge > 30):
		return invalid("min_age", "0 to 30")
	case len(in.Genres) > 12:
		return invalid("genres", "at most 12")
	case len(in.DescriptionMD) > 20000:
		return invalid("description_md", "at most 20000 characters")
	case len(in.CostText) > 200:
		return invalid("cost_text", "at most 200 characters")
	case in.Capacity != nil && *in.Capacity <= 0:
		return invalid("capacity", "must be positive")
	case in.Slug != "" && !slugRe.MatchString(in.Slug):
		return invalid("slug", "lowercase letters, digits and dashes")
	}
	if in.ExternalTicketURL != nil {
		u, err := url.Parse(*in.ExternalTicketURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return invalid("external_ticket_url", "an https:// link")
		}
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" {
		return invalid("timezone", "unknown IANA timezone")
	}
	for i, g := range in.Genres {
		in.Genres[i] = strings.TrimSpace(strings.ToLower(g))
	}
	return nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify turns a title into a url-safe slug.
func Slugify(title string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(envelope.NormalizeName(title), "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if len(s) < 2 {
		s = "event-" + s
	}
	return s
}

// Summary is an events-list row.
type Summary struct {
	Event
	VenueName    *string `json:"venue_name"`
	ActCount     int     `json:"act_count"`
	StageCount   int     `json:"stage_count"`
	UntimedCount int     `json:"untimed_count"`
	// ErrorCount and WarningCount summarise timetable issues (upcoming and drafts only).
	ErrorCount   int `json:"error_count"`
	WarningCount int `json:"warning_count"`
}

// VenueRef is the venue as shown on an event.
type VenueRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	City string    `json:"city"`
}

// Detail is an event with its stages, lineup and current issues.
type Detail struct {
	Event
	Venue  *VenueRef     `json:"venue"`
	Stages []Stage       `json:"stages"`
	Lineup []LineupEntry `json:"lineup"`
	Issues []Issue       `json:"issues"`
}

const eventCols = `e.id, e.title, e.slug, e.status, e.visibility, e.publish_at, e.starts_at, e.ends_at, e.doors_at, e.timezone,
	e.venue_id, e.city, e.location_mode, e.location_reveal_at, e.min_age, e.genres, e.description_md, e.cost_text,
	e.external_ticket_url, e.capacity, e.version, e.created_at, e.updated_at`

func scanEvent(row pgx.Row, extra ...any) (Event, error) {
	var e Event
	var minAge *int16
	dest := append([]any{&e.ID, &e.Title, &e.Slug, &e.Status, &e.Visibility, &e.PublishAt, &e.StartsAt, &e.EndsAt, &e.DoorsAt,
		&e.Timezone, &e.VenueID, &e.City, &e.LocationMode, &e.LocationRevealAt, &minAge, &e.Genres, &e.DescriptionMD,
		&e.CostText, &e.ExternalTicketURL, &e.Capacity, &e.Version, &e.CreatedAt, &e.UpdatedAt}, extra...)
	err := row.Scan(dest...)
	if minAge != nil {
		v := int(*minAge)
		e.MinAge = &v
	}
	return e, err
}

// ListEvents returns upcoming, drafts or past events.
func (s *Service) ListEvents(ctx context.Context, view string) ([]Summary, error) {
	where := map[string]string{
		"upcoming": `e.status <> 'draft' AND e.ends_at > $1 ORDER BY e.starts_at`,
		"drafts":   `e.status = 'draft' AND $1::timestamptz IS NOT NULL ORDER BY e.starts_at`,
		"past":     `e.status <> 'draft' AND e.ends_at <= $1 ORDER BY e.starts_at DESC LIMIT 200`,
	}[view]
	if where == "" {
		return nil, invalid("view", "upcoming, drafts or past")
	}
	out := []Summary{}
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+eventCols+`, v.name,
		  (SELECT count(*) FROM lineup_entries l WHERE l.event_id = e.id),
		  (SELECT count(*) FROM event_stages st WHERE st.event_id = e.id),
		  (SELECT count(*) FROM lineup_entries l WHERE l.event_id = e.id AND l.set_start IS NULL)
		  FROM events e LEFT JOIN venues v ON v.id = e.venue_id WHERE `+where, s.now())
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Summary, error) {
			var x Summary
			var err error
			x.Event, err = scanEvent(r, &x.VenueName, &x.ActCount, &x.StageCount, &x.UntimedCount)
			return x, err
		})
		if err != nil || view == "past" || len(out) == 0 {
			return err
		}
		ids := make([]uuid.UUID, len(out))
		for i := range out {
			ids[i] = out[i].ID
		}
		stages, lineup, err := timetablesFor(ctx, tx, ids)
		if err != nil {
			return err
		}
		for i := range out {
			for _, is := range ValidateTimetable(out[i].Event, stages[out[i].ID], lineup[out[i].ID]) {
				if is.Severity == SeverityError {
					out[i].ErrorCount++
				} else {
					out[i].WarningCount++
				}
			}
		}
		return nil
	})
	if out == nil {
		out = []Summary{}
	}
	return out, err
}

// GetEvent returns the event with stages, lineup and issues.
func (s *Service) GetEvent(ctx context.Context, id uuid.UUID) (Detail, error) {
	var d Detail
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		var err error
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

func (s *Service) detail(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Detail, error) {
	ev, err := scanEvent(tx.QueryRow(ctx, `SELECT `+eventCols+` FROM events e WHERE e.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Event: ev}
	if ev.VenueID != nil {
		var v VenueRef
		if err := tx.QueryRow(ctx, `SELECT id, name, city FROM venues WHERE id = $1`, *ev.VenueID).Scan(&v.ID, &v.Name, &v.City); err == nil {
			d.Venue = &v
		}
	}
	stages, lineup, err := timetablesFor(ctx, tx, []uuid.UUID{id})
	if err != nil {
		return Detail{}, err
	}
	d.Stages, d.Lineup = stages[id], lineup[id]
	if d.Stages == nil {
		d.Stages = []Stage{}
	}
	if d.Lineup == nil {
		d.Lineup = []LineupEntry{}
	}
	d.Issues = ValidateTimetable(d.Event, d.Stages, d.Lineup)
	if d.Issues == nil {
		d.Issues = []Issue{}
	}
	return d, nil
}

// timetablesFor loads stages and lineup for several events in two queries.
func timetablesFor(ctx context.Context, tx pgx.Tx, ids []uuid.UUID) (map[uuid.UUID][]Stage, map[uuid.UUID][]LineupEntry, error) {
	stages := make(map[uuid.UUID][]Stage, len(ids))
	lineup := make(map[uuid.UUID][]LineupEntry, len(ids))
	for _, id := range ids {
		stages[id], lineup[id] = []Stage{}, []LineupEntry{}
	}
	rows, err := tx.Query(ctx, `SELECT event_id, id, name, position, curfew_at, changeover_minutes
	  FROM event_stages WHERE event_id = ANY($1) ORDER BY position`, ids)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var ev uuid.UUID
		var x Stage
		var pos, co int16
		if err := rows.Scan(&ev, &x.ID, &x.Name, &pos, &x.CurfewAt, &co); err != nil {
			rows.Close()
			return nil, nil, err
		}
		x.Position, x.ChangeoverMinutes = int(pos), int(co)
		stages[ev] = append(stages[ev], x)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	rows, err = tx.Query(ctx, `SELECT event_id, id, stage_id, display_name, profile_url, billing_order, b2b_group, set_start, set_end
	  FROM lineup_entries WHERE event_id = ANY($1) ORDER BY billing_order, display_name`, ids)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var ev uuid.UUID
		var x LineupEntry
		var order int16
		var group *int16
		if err := rows.Scan(&ev, &x.ID, &x.StageID, &x.DisplayName, &x.ProfileURL, &order, &group, &x.SetStart, &x.SetEnd); err != nil {
			rows.Close()
			return nil, nil, err
		}
		x.BillingOrder = int(order)
		if group != nil {
			g := int(*group)
			x.B2BGroup = &g
		}
		lineup[ev] = append(lineup[ev], x)
	}
	return stages, lineup, rows.Err()
}

// CreateEvent stores a draft; the venue's rooms become its stages.
func (s *Service) CreateEvent(ctx context.Context, in EventInput) (Detail, error) {
	if err := in.validate(); err != nil {
		return Detail{}, err
	}
	id := uuid.Must(uuid.NewV7())
	var d Detail
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		if in.VenueID != nil {
			var city string
			if err := tx.QueryRow(ctx, `SELECT city FROM venues WHERE id = $1 AND archived_at IS NULL`, *in.VenueID).Scan(&city); err != nil {
				return invalid("venue_id", "unknown venue")
			}
			if in.City == "" {
				in.City = city
			}
		}
		slug, err := s.freeSlug(ctx, tx, in.Slug, in.Title, nil)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO events (id, tenant_id, title, slug, visibility, publish_at, starts_at, ends_at, doors_at,
		  timezone, venue_id, city, location_mode, location_reveal_at, min_age, genres, description_md, cost_text, external_ticket_url, capacity)
		  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
			id, tenantOf(ctx), in.Title, slug, in.Visibility, in.PublishAt, in.StartsAt, in.EndsAt, in.DoorsAt, in.Timezone,
			in.VenueID, in.City, in.LocationMode, in.LocationRevealAt, in.MinAge, nonNil(in.Genres), in.DescriptionMD, in.CostText,
			in.ExternalTicketURL, in.Capacity); err != nil {
			return err
		}
		if in.VenueID != nil {
			if _, err := tx.Exec(ctx, `INSERT INTO event_stages (id, tenant_id, event_id, name, position)
			  SELECT gen_random_uuid(), tenant_id, $1, name, position FROM venue_rooms WHERE venue_id = $2`, id, *in.VenueID); err != nil {
				return err
			}
		}
		if err := s.emit(ctx, tx, "event", "created", map[string]uuid.UUID{"event_id": id}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// freeSlug returns the requested or title-derived slug, suffixed -2, -3…
// when taken within the organisation.
func (s *Service) freeSlug(ctx context.Context, tx pgx.Tx, requested, title string, self *uuid.UUID) (string, error) {
	base := requested
	if base == "" {
		base = Slugify(title)
	}
	for i := 1; i < 100; i++ {
		cand := base
		if i > 1 {
			cand = fmt.Sprintf("%s-%d", base, i)
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM events WHERE slug = $1 AND ($2::uuid IS NULL OR id <> $2))`, cand, self).Scan(&taken); err != nil {
			return "", err
		}
		if !taken {
			return cand, nil
		}
	}
	return "", invalid("slug", "no free variant")
}

// UpdateEvent replaces an event's details if version still matches.
func (s *Service) UpdateEvent(ctx context.Context, id uuid.UUID, version int, in EventInput) (Detail, error) {
	if err := in.validate(); err != nil {
		return Detail{}, err
	}
	var d Detail
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		cur, err := s.detail(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur.Version != version {
			return ErrVersionConflict
		}
		if cut := setsOutside(cur.Lineup, in); len(cut) > 0 {
			return &CutsSetsError{Entries: cut}
		}
		if in.VenueID != nil && (cur.VenueID == nil || *cur.VenueID != *in.VenueID) {
			if err := tx.QueryRow(ctx, `SELECT 1 FROM venues WHERE id = $1 AND archived_at IS NULL`, *in.VenueID).Scan(new(int)); err != nil {
				return invalid("venue_id", "unknown venue")
			}
		}
		slug := cur.Slug
		if in.Slug != "" && in.Slug != cur.Slug {
			if slug, err = s.freeSlug(ctx, tx, in.Slug, in.Title, &id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE events SET title=$2, slug=$3, visibility=$4, publish_at=$5, starts_at=$6, ends_at=$7, doors_at=$8,
		  timezone=$9, venue_id=$10, city=$11, location_mode=$12, location_reveal_at=$13, min_age=$14, genres=$15, description_md=$16,
		  cost_text=$17, external_ticket_url=$18, capacity=$19, version = version + 1, updated_at = now() WHERE id = $1`,
			id, in.Title, slug, in.Visibility, in.PublishAt, in.StartsAt, in.EndsAt, in.DoorsAt, in.Timezone, in.VenueID, in.City,
			in.LocationMode, in.LocationRevealAt, in.MinAge, nonNil(in.Genres), in.DescriptionMD, in.CostText, in.ExternalTicketURL,
			in.Capacity); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, "event", "updated", map[string]uuid.UUID{"event_id": id}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

func setsOutside(lineup []LineupEntry, in EventInput) []uuid.UUID {
	start := in.StartsAt
	if in.DoorsAt != nil {
		start = *in.DoorsAt
	}
	var cut []uuid.UUID
	for _, e := range lineup {
		if e.SetStart != nil && (e.SetStart.Before(start) || e.SetEnd.After(in.EndsAt)) {
			cut = append(cut, e.ID)
		}
	}
	return cut
}

var transitions = map[string][]string{
	StatusDraft:     {StatusScheduled, StatusPublished, StatusCancelled},
	StatusScheduled: {StatusDraft, StatusPublished, StatusCancelled, StatusPostponed},
	StatusPublished: {StatusCancelled, StatusPostponed},
	StatusPostponed: {StatusScheduled, StatusPublished, StatusCancelled},
	StatusCancelled: {},
}

// ChangeStatus moves an event through its lifecycle. Scheduling and
// publishing are blocked while the timetable has errors (UX decision 3).
func (s *Service) ChangeStatus(ctx context.Context, id uuid.UUID, version int, to string) (Detail, error) {
	var d Detail
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		cur, err := s.detail(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur.Version != version {
			return ErrVersionConflict
		}
		if !slices.Contains(transitions[cur.Status], to) {
			return ErrBadTransition
		}
		if to == StatusScheduled || to == StatusPublished {
			if HasErrors(cur.Issues) {
				return &BlockedError{Reason: "timetable_errors", Issues: cur.Issues}
			}
			if to == StatusScheduled && (cur.PublishAt == nil || !cur.PublishAt.After(s.now())) {
				return &BlockedError{Reason: "publish_at_required"}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE events SET status = $2, version = version + 1, updated_at = now() WHERE id = $1`, id, to); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, "event", "status_changed", map[string]uuid.UUID{"event_id": id}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

// StageInput creates (ID nil) or keeps a stage.
type StageInput struct {
	ID                *uuid.UUID `json:"id"`
	Name              string     `json:"name"`
	CurfewAt          *time.Time `json:"curfew_at"`
	ChangeoverMinutes int        `json:"changeover_minutes"`
}

// ReplaceStages sets the event's stages in order; sets on removed stages
// become unassigned.
func (s *Service) ReplaceStages(ctx context.Context, id uuid.UUID, version int, stages []StageInput) (Detail, error) {
	for i := range stages {
		stages[i].Name = strings.TrimSpace(stages[i].Name)
		if stages[i].Name == "" || len(stages[i].Name) > 80 {
			return Detail{}, invalid(fmt.Sprintf("stages[%d].name", i), "required, at most 80 characters")
		}
		if stages[i].ChangeoverMinutes < 0 || stages[i].ChangeoverMinutes > 120 {
			return Detail{}, invalid(fmt.Sprintf("stages[%d].changeover_minutes", i), "0 to 120")
		}
	}
	return s.mutateEvent(ctx, id, version, "event", "updated", func(tx pgx.Tx, cur Detail) error {
		existing := map[uuid.UUID]bool{}
		for _, st := range cur.Stages {
			existing[st.ID] = true
		}
		keep := []uuid.UUID{}
		for i, st := range stages {
			sid := uuid.Must(uuid.NewV7())
			if st.ID != nil {
				if !existing[*st.ID] {
					return invalid(fmt.Sprintf("stages[%d].id", i), "not a stage of this event")
				}
				sid = *st.ID
			}
			keep = append(keep, sid)
			if _, err := tx.Exec(ctx, `INSERT INTO event_stages (id, tenant_id, event_id, name, position, curfew_at, changeover_minutes)
			  VALUES ($1, $2, $3, $4, $5, $6, $7)
			  ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, position = EXCLUDED.position,
			    curfew_at = EXCLUDED.curfew_at, changeover_minutes = EXCLUDED.changeover_minutes`,
				sid, tenantOf(ctx), id, st.Name, i, st.CurfewAt, st.ChangeoverMinutes); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `DELETE FROM event_stages WHERE event_id = $1 AND NOT (id = ANY($2))`, id, keep)
		return err
	})
}

// LineupInput creates (ID nil) or keeps a lineup entry; order = billing.
type LineupInput struct {
	ID          *uuid.UUID `json:"id"`
	StageID     *uuid.UUID `json:"stage_id"`
	DisplayName string     `json:"display_name"`
	ProfileURL  *string    `json:"profile_url"`
	B2BGroup    *int       `json:"b2b_group"`
	SetStart    *time.Time `json:"set_start"`
	SetEnd      *time.Time `json:"set_end"`
}

// ReplaceLineup sets the lineup in billing order. Timetables with errors
// are saved (UX decision 3); the returned Detail carries the issues.
func (s *Service) ReplaceLineup(ctx context.Context, id uuid.UUID, version int, lineup []LineupInput) (Detail, error) {
	if len(lineup) > 300 {
		return Detail{}, invalid("lineup", "at most 300 acts")
	}
	for i := range lineup {
		e := &lineup[i]
		e.DisplayName = strings.TrimSpace(e.DisplayName)
		field := func(f string) string { return fmt.Sprintf("lineup[%d].%s", i, f) }
		switch {
		case e.DisplayName == "" || len(e.DisplayName) > 120:
			return Detail{}, invalid(field("display_name"), "required, at most 120 characters")
		case (e.SetStart == nil) != (e.SetEnd == nil):
			return Detail{}, invalid(field("set_end"), "set start and end go together")
		case e.SetStart != nil && !e.SetEnd.After(*e.SetStart):
			return Detail{}, invalid(field("set_end"), "must be after the set start")
		}
		if e.ProfileURL != nil {
			if u, err := url.Parse(*e.ProfileURL); err != nil || u.Scheme != "https" || u.Host == "" {
				return Detail{}, invalid(field("profile_url"), "an https:// link")
			}
		}
	}
	return s.mutateEvent(ctx, id, version, "lineup", "updated", func(tx pgx.Tx, cur Detail) error {
		stages := map[uuid.UUID]bool{}
		for _, st := range cur.Stages {
			stages[st.ID] = true
		}
		existing := map[uuid.UUID]bool{}
		for _, e := range cur.Lineup {
			existing[e.ID] = true
		}
		keep := []uuid.UUID{}
		for i, e := range lineup {
			if e.StageID != nil && !stages[*e.StageID] {
				return invalid(fmt.Sprintf("lineup[%d].stage_id", i), "not a stage of this event")
			}
			lid := uuid.Must(uuid.NewV7())
			if e.ID != nil {
				if !existing[*e.ID] {
					return invalid(fmt.Sprintf("lineup[%d].id", i), "not an act of this event")
				}
				lid = *e.ID
			}
			keep = append(keep, lid)
			if _, err := tx.Exec(ctx, `INSERT INTO lineup_entries (id, tenant_id, event_id, stage_id, display_name, profile_url,
			  billing_order, b2b_group, set_start, set_end) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			  ON CONFLICT (id) DO UPDATE SET stage_id = EXCLUDED.stage_id, display_name = EXCLUDED.display_name,
			    profile_url = EXCLUDED.profile_url, billing_order = EXCLUDED.billing_order, b2b_group = EXCLUDED.b2b_group,
			    set_start = EXCLUDED.set_start, set_end = EXCLUDED.set_end`,
				lid, tenantOf(ctx), id, e.StageID, e.DisplayName, e.ProfileURL, i, e.B2BGroup, e.SetStart, e.SetEnd); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `DELETE FROM lineup_entries WHERE event_id = $1 AND NOT (id = ANY($2))`, id, keep)
		return err
	})
}

// mutateEvent runs fn under an optimistic version check and bumps the
// event version so exports know they are stale.
func (s *Service) mutateEvent(ctx context.Context, id uuid.UUID, version int, aggregate, verb string, fn func(pgx.Tx, Detail) error) (Detail, error) {
	var d Detail
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		cur, err := s.detail(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur.Version != version {
			return ErrVersionConflict
		}
		if err := fn(tx, cur); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE events SET version = version + 1, updated_at = now() WHERE id = $1`, id); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, aggregate, verb, map[string]uuid.UUID{"event_id": id}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return Detail{}, invalid("input", pgErr.ConstraintName)
	}
	return d, err
}

// ExportData is everything the export pack may contain, already filtered by
// the location mode at this moment.
type ExportData struct {
	Event     Event          `json:"event"`
	Lineup    []LineupEntry  `json:"lineup"`
	Stages    []Stage        `json:"stages"`
	Location  PublicLocation `json:"location"`
	Organizer string         `json:"organizer"`
	Profile   Organizer      `json:"organizer_profile"`
	Embargoed bool           `json:"embargoed"`
}

// organizer reads the collective profile for exports.
func organizer(ctx context.Context, tx pgx.Tx) (Organizer, error) {
	var o Organizer
	var website, instagram, soundcloud, ra, accent *string
	err := tx.QueryRow(ctx, `SELECT name, bio, website_url, instagram_url, soundcloud_url, ra_url, accent_color
	  FROM organizations WHERE id = $1`, tenantOf(ctx)).Scan(&o.Name, &o.Bio, &website, &instagram, &soundcloud, &ra, &accent)
	if website != nil {
		o.URL = *website
	}
	for _, l := range []*string{instagram, soundcloud, ra} {
		if l != nil {
			o.SameAs = append(o.SameAs, *l)
		}
	}
	if accent != nil {
		o.AccentColor = *accent
	}
	return o, err
}

// Export assembles export data. Blocked while the timetable has errors.
// The venue address is decrypted only when the location may be public.
func (s *Service) Export(ctx context.Context, id uuid.UUID) (ExportData, error) {
	var out ExportData
	err := s.db.Run(ctx, func(tx pgx.Tx) error {
		d, err := s.detail(ctx, tx, id)
		if err != nil {
			return err
		}
		if HasErrors(d.Issues) {
			return &BlockedError{Reason: "timetable_errors", Issues: d.Issues}
		}
		now := s.now()
		var venue *PublicVenue
		if d.VenueID != nil {
			v := PublicVenue{}
			var country *string
			var addrEnc []byte
			if err := tx.QueryRow(ctx, `SELECT name, city, country, address_enc FROM venues WHERE id = $1`, *d.VenueID).
				Scan(&v.Name, &v.City, &country, &addrEnc); err != nil {
				return err
			}
			if country != nil {
				v.Country = strings.TrimSpace(*country)
			}
			if !LocationAt(d.Event, &v, now).Withheld && addrEnc != nil {
				tenant := tenantOf(ctx)
				dek, err := s.keys.ForSealed(ctx, tx, tenant, addrEnc)
				if err != nil {
					return err
				}
				plain, err := dek.Open(tenant, envelope.Field{Table: "venues", Column: "address_enc", RowID: *d.VenueID}, addrEnc)
				if err != nil {
					return err
				}
				v.Address = string(plain)
			}
			venue = &v
		}
		org, err := organizer(ctx, tx)
		if err != nil {
			return err
		}
		out = ExportData{
			Event: d.Event, Lineup: SortByBilling(d.Lineup), Stages: d.Stages,
			Location: LocationAt(d.Event, venue, now), Organizer: org.Name, Profile: org,
			Embargoed: d.Status == StatusDraft || (d.PublishAt != nil && now.Before(*d.PublishAt)),
		}
		return nil
	})
	return out, err
}
