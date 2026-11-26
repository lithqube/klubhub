package release

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Platform is a store or service a release can be linked to (REL-04).
type Platform string

const (
	PlatformBeatport   Platform = "beatport"
	PlatformBandcamp   Platform = "bandcamp"
	PlatformSpotify    Platform = "spotify"
	PlatformSoundcloud Platform = "soundcloud"
)

// platforms is the set of known platforms, in the order the UI shows them.
var platforms = []Platform{PlatformBeatport, PlatformBandcamp, PlatformSpotify, PlatformSoundcloud}

// PlatformLink is one manual platform URL of a release.
type PlatformLink struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ReleaseID uuid.UUID `json:"release_id" db:"release_id"`
	Platform  Platform  `json:"platform" db:"platform"`
	URL       string    `json:"url" db:"url"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// KnownPlatforms returns the platforms a release can be linked to.
func KnownPlatforms() []Platform {
	out := make([]Platform, len(platforms))
	copy(out, platforms)
	return out
}

// ValidPlatform reports whether p is one of the known platforms.
func ValidPlatform(p Platform) bool {
	for _, known := range platforms {
		if known == p {
			return true
		}
	}
	return false
}

// SetPlatformLink stores a link for one platform of a release, replacing the
// link that platform had before.
func (s *Service) SetPlatformLink(ctx context.Context, releaseID uuid.UUID, platform Platform, rawURL string) (*PlatformLink, error) {
	if _, err := s.repo.GetByID(ctx, releaseID); err != nil {
		return nil, err
	}
	if !ValidPlatform(platform) {
		return nil, errors.Join(ErrInvalid, errors.New("unknown platform"))
	}
	link, err := s.validateURL(rawURL)
	if err != nil {
		return nil, err
	}
	link.ReleaseID = releaseID
	link.Platform = platform
	if err := s.repo.UpsertPlatformLink(ctx, link); err != nil {
		return nil, err
	}
	return link, nil
}

// RemovePlatformLink removes the link of one platform.
func (s *Service) RemovePlatformLink(ctx context.Context, releaseID uuid.UUID, platform Platform) error {
	if !ValidPlatform(platform) {
		return errors.Join(ErrInvalid, errors.New("unknown platform"))
	}
	if _, err := s.repo.GetByID(ctx, releaseID); err != nil {
		return err
	}
	return s.repo.DeletePlatformLink(ctx, releaseID, platform)
}

// ListPlatformLinks returns the links of a release.
func (s *Service) ListPlatformLinks(ctx context.Context, releaseID uuid.UUID) ([]*PlatformLink, error) {
	if _, err := s.repo.GetByID(ctx, releaseID); err != nil {
		return nil, err
	}
	return s.repo.ListPlatformLinks(ctx, releaseID)
}

// validateURL checks that the URL is an absolute http(s) URL.
func (s *Service) validateURL(raw string) (*PlatformLink, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.Join(ErrInvalid, errors.New("url is required"))
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return nil, errors.Join(ErrInvalid, errors.New("url must be an absolute http(s) URL"))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.Join(ErrInvalid, errors.New("url must be http or https"))
	}
	return &PlatformLink{URL: trimmed}, nil
}
