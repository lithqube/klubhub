package artwork

import (
	"context"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/klubhub/dj/api/internal/tracklist"
)

// trackArtworkUpdater defines the interface for updating track artwork status in the database
type trackArtworkUpdater interface {
	UpdateArtworkStatus(ctx context.Context, trackID uuid.UUID, status, url, source string) error
}

// Service provides the artwork fetch worker pool
type Service struct {
	chain          *FetchChain
	repo           trackArtworkUpdater
	placeholderURL string
}

// NewService creates a new artwork service
func NewService(chain *FetchChain, repo trackArtworkUpdater, placeholderURL string) *Service {
	return &Service{
		chain:          chain,
		repo:           repo,
		placeholderURL: placeholderURL,
	}
}

// FetchAll fetches artwork for all tracks using a goroutine worker pool
// This runs asynchronously and updates each track's artwork status in the database
func (s *Service) FetchAll(ctx context.Context, tracks []tracklist.Track) {
	const workers = 4

	if len(tracks) == 0 {
		return
	}

	// Create buffered channel for jobs
	jobs := make(chan tracklist.Track, len(tracks))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go s.worker(ctx, jobs, &wg)
	}

	// Send all jobs
	for _, t := range tracks {
		jobs <- t
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()
}

// worker processes artwork fetch jobs from the channel
func (s *Service) worker(ctx context.Context, jobs <-chan tracklist.Track, wg *sync.WaitGroup) {
	defer wg.Done()

	for t := range jobs {
		// Skip tracks with manual artwork source (TRKL-14)
		// Manual overrides must never be replaced by API re-fetch
		if t.ArtworkSource == tracklist.ArtworkManual {
			continue
		}

		// Fetch artwork
		result := s.chain.Fetch(ctx, t.Title, t.Artist)

		// Determine status based on source
		var status string
		switch result.Source {
		case "cache", "spotify", "discogs", "musicbrainz":
			status = tracklist.ArtworkFetched
		case "placeholder":
			status = tracklist.ArtworkPlaceholder
		default:
			status = tracklist.ArtworkPlaceholder
		}

		// Update database
		if err := s.repo.UpdateArtworkStatus(ctx, t.ID, status, result.URL, result.Source); err != nil {
			// Log error but don't panic - continue processing other tracks
			log.Printf("failed to update artwork for track %s: %v", t.ID, err)
			continue
		}
	}
}
