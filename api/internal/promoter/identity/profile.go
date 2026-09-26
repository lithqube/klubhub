package identity

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/klubhub/dj/api/internal/platform/audit"
	"github.com/klubhub/dj/api/internal/platform/authz"
)

// Profile is the collective's public face in exports (P1.5). Every field is
// public by design; the UI says so and contacts have no place here.
type Profile struct {
	Bio           string  `json:"bio"`
	WebsiteURL    *string `json:"website_url"`
	InstagramURL  *string `json:"instagram_url"`
	SoundcloudURL *string `json:"soundcloud_url"`
	RAURL         *string `json:"ra_url"`
	AccentColor   *string `json:"accent_color"`
}

const profileCols = `bio, website_url, instagram_url, soundcloud_url, ra_url, accent_color`

func (p *Profile) scanDest() []any {
	return []any{&p.Bio, &p.WebsiteURL, &p.InstagramURL, &p.SoundcloudURL, &p.RAURL, &p.AccentColor}
}

// ProfileError names the field a profile update got wrong.
type ProfileError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

func (e *ProfileError) Error() string { return "identity: invalid " + e.Field + ": " + e.Problem }

var accentRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// httpsURL trims and checks a public link; empty clears it.
func httpsURL(field string, v *string) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	s := strings.TrimSpace(*v)
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(s) > 300 {
		return nil, &ProfileError{Field: field, Problem: "an https:// link, at most 300 characters"}
	}
	return &s, nil
}

func (p *Profile) normalise() error {
	p.Bio = strings.TrimSpace(p.Bio)
	if len([]rune(p.Bio)) > 2000 {
		return &ProfileError{Field: "bio", Problem: "at most 2000 characters"}
	}
	var err error
	for _, f := range []struct {
		name string
		v    **string
	}{{"website_url", &p.WebsiteURL}, {"instagram_url", &p.InstagramURL}, {"soundcloud_url", &p.SoundcloudURL}, {"ra_url", &p.RAURL}} {
		if *f.v, err = httpsURL(f.name, *f.v); err != nil {
			return err
		}
	}
	if p.AccentColor != nil {
		c := strings.ToLower(strings.TrimSpace(*p.AccentColor))
		switch {
		case c == "":
			p.AccentColor = nil
		case !accentRe.MatchString(c):
			return &ProfileError{Field: "accent_color", Problem: "a hex colour like #96f8ff"}
		default:
			p.AccentColor = &c
		}
	}
	return nil
}

// UpdateProfile replaces the collective profile and records it in the audit log.
func (s *Service) UpdateProfile(ctx context.Context, p authz.Principal, in Profile) (Organization, error) {
	tenant, err := uuid.Parse(p.OrgID)
	if err != nil {
		return Organization{}, ErrInvalidInput
	}
	if err := in.normalise(); err != nil {
		return Organization{}, err
	}
	err = s.db.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE organizations SET bio = $2, website_url = $3, instagram_url = $4, soundcloud_url = $5,
		  ra_url = $6, accent_color = $7, updated_at = now() WHERE id = $1`,
			tenant, in.Bio, in.WebsiteURL, in.InstagramURL, in.SoundcloudURL, in.RAURL, in.AccentColor); err != nil {
			return err
		}
		return audit.Record(ctx, tx, tenant, audit.Entry{ActorID: p.Sub, Action: "org.update", Resource: "org:" + tenant.String(), Allowed: true})
	})
	if err != nil {
		return Organization{}, err
	}
	return s.Org(ctx, p)
}
