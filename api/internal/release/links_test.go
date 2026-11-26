package release

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSetPlatformLinkValidatesInput(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPlatformLink(context.Background(), rel.ID, Platform("myspace"), "https://example.com"); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown platform: want ErrInvalid, got %v", err)
	}
	for _, bad := range []string{"", "   ", "example.com", "ftp://example.com", "javascript:alert(1)"} {
		if _, err := svc.SetPlatformLink(context.Background(), rel.ID, PlatformBeatport, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("url %q: want ErrInvalid, got %v", bad, err)
		}
	}
	if _, err := svc.SetPlatformLink(context.Background(), uuid.Must(uuid.NewV7()), PlatformBeatport, "https://beatport.com/x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown release: want ErrNotFound, got %v", err)
	}
	link, err := svc.SetPlatformLink(context.Background(), rel.ID, PlatformBeatport, "  https://beatport.com/release/1  ")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://beatport.com/release/1" {
		t.Errorf("url = %q, want trimmed", link.URL)
	}
	if link.Platform != PlatformBeatport {
		t.Errorf("platform = %s, want beatport", link.Platform)
	}
}

func TestSetPlatformLinkReplacesThePreviousOne(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPlatformLink(context.Background(), rel.ID, PlatformBandcamp, "https://bandcamp.com/a"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPlatformLink(context.Background(), rel.ID, PlatformBandcamp, "https://bandcamp.com/b"); err != nil {
		t.Fatal(err)
	}
	links, err := svc.ListPlatformLinks(context.Background(), rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("release has %d bandcamp links, want 1", len(links))
	}
	if links[0].URL != "https://bandcamp.com/b" {
		t.Errorf("url = %q, want the replacement", links[0].URL)
	}
}

func TestRemovePlatformLink(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	rel, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPlatformLink(context.Background(), rel.ID, PlatformSpotify, "https://open.spotify.com/x"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemovePlatformLink(context.Background(), rel.ID, PlatformSpotify); err != nil {
		t.Fatal(err)
	}
	links, err := svc.ListPlatformLinks(context.Background(), rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("release has %d links after remove, want 0", len(links))
	}
	if err := svc.RemovePlatformLink(context.Background(), rel.ID, PlatformSpotify); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing a missing link: want ErrNotFound, got %v", err)
	}
}

func TestLinksBelongToOneRelease(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	first, err := svc.Create(context.Background(), Input{Title: "Night Signal", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(context.Background(), Input{Title: "Blue Hour", ReleaseDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPlatformLink(context.Background(), first.ID, PlatformSoundcloud, "https://soundcloud.com/a"); err != nil {
		t.Fatal(err)
	}
	links, err := svc.ListPlatformLinks(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("second release sees %d links of another release", len(links))
	}
}

func TestKnownPlatforms(t *testing.T) {
	known := KnownPlatforms()
	if len(known) != 4 {
		t.Fatalf("known platforms = %d, want 4", len(known))
	}
	for _, p := range []Platform{PlatformBeatport, PlatformBandcamp, PlatformSpotify, PlatformSoundcloud} {
		if !ValidPlatform(p) {
			t.Errorf("ValidPlatform(%s) = false", p)
		}
	}
	if ValidPlatform(Platform("myspace")) {
		t.Error("ValidPlatform(myspace) = true")
	}
}
