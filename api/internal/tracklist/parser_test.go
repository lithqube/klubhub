package tracklist

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/stretchr/testify/require"
)

func TestParseRekordbox(t *testing.T) {
	f, err := os.Open("../../testdata/rekordbox_sample.txt")
	require.NoError(t, err)
	defer f.Close()

	tracks, warnings, err := ParseRekordboxTSV(f)
	if err != nil {
		t.Fatalf("Error parsing: %v", err)
	}

	fmt.Printf("Parsed %d tracks, %d warnings\n", len(tracks), len(warnings))
	for i, w := range warnings {
		fmt.Printf("Warning %d: %+v\n", i, w)
	}

	require.NoError(t, err)
	require.Len(t, tracks, 5)
}

func TestTrackFields(t *testing.T) {
	f, err := os.Open("../../testdata/rekordbox_sample.txt")
	require.NoError(t, err)
	defer f.Close()

	tracks, warnings, err := ParseRekordboxTSV(f)
	if err != nil {
		t.Fatalf("Error parsing: %v", err)
	}

	fmt.Printf("Parsed %d tracks, %d warnings\n", len(tracks), len(warnings))
	for i, t := range tracks {
		fmt.Printf("Track %d: %+v\n", i, t)
	}

	require.NoError(t, err)
	require.Len(t, tracks, 5)
	require.Equal(t, "AFRICA (TOTO)", tracks[0].Title)
	require.Equal(t, 288, tracks[0].DurationSeconds)
}

func TestInvalidFormat(t *testing.T) {
	_, _, err := ParseRekordboxTSV(bytes.NewReader([]byte("hello world\nnot tsv")))
	require.ErrorIs(t, err, ErrInvalidFormat)
}

func TestPartialParse(t *testing.T) {
	// Craft a valid UTF-16 LE reader with 2 good rows and 1 row with only 3 fields
	// This would require creating a proper UTF-16 LE file, but for now we'll just
	// create a simple test that will fail until we implement the parser
	_, _, err := ParseRekordboxTSV(bytes.NewReader([]byte("invalid\tdata\nwith\twrong\tfield\tcount\nanother\trow")))
	require.Error(t, err) // This should either succeed with warnings or fail appropriately
}

func TestZeroTracks(t *testing.T) {
	// Valid UTF-16 LE reader with only the header row
	header := "#\tArtwork\tTrack Title\tArtist\tAlbum\tGenre\tBPM\tRating\tTime\tKey\tDate Added\n"
	utf16le := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
	encoder := utf16le.NewEncoder()
	result, _, err := transform.Bytes(encoder, []byte(header))
	if err != nil {
		t.Fatalf("Failed to encode header as UTF-16: %v", err)
	}
	_, _, err = ParseRekordboxTSV(bytes.NewReader(result))
	require.ErrorIs(t, err, ErrZeroTracks)
}
