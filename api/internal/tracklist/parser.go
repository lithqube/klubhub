package tracklist

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// ParseRekordboxTSV parses a Rekordbox TSV file and returns tracks and warnings
func ParseRekordboxTSV(r io.Reader) ([]Track, []ParseWarning, error) {
	// Wrap the reader with UTF-16 LE detection (BOM override to ignore BOM if present)
	utf16le := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	reader := transform.NewReader(r, utf16le.NewDecoder())

	// Read the entire content
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, err
	}

	// Convert to string
	text := string(content)

	// Split into lines
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	if len(lines) == 0 {
		return nil, nil, ErrZeroTracks
	}

	// Process header
	headerLine := lines[0]
	headerFields := strings.Split(headerLine, "\t")
	if len(headerFields) == 0 {
		return nil, nil, ErrInvalidFormat
	}

	// Build column index from header
	colIndex := buildColumnIndex(headerFields)
	if _, ok := colIndex["Track Title"]; !ok {
		return nil, nil, ErrInvalidFormat
	}

	var tracks []Track
	var warnings []ParseWarning

	// Process data rows
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			// Skip empty lines
			continue
		}

		fields := strings.Split(line, "\t")
		originalFieldCount := len(fields)

		// If we have fewer fields than header, prepend empty strings
		if len(fields) < len(headerFields) {
			// Prepend empty strings to make up the difference
			prefix := make([]string, len(headerFields)-len(fields))
			fields = append(prefix, fields...)
		} else if len(fields) > len(headerFields) {
			// Truncate to header length
			fields = fields[:len(headerFields)]
		}

		// If the field count after adjustment still doesn't match, something is wrong
		if len(fields) != len(headerFields) {
			// This shouldn't happen after our adjustment, but just in case
			warnings = append(warnings, ParseWarning{
				Row:     i,
				Field:   "multiple",
				Message: "field count mismatch after adjustment",
			})
			continue
		}

		// If the original field count didn't match, add a warning
		if originalFieldCount != len(headerFields) {
			warnings = append(warnings, ParseWarning{
				Row:     i,
				Field:   "multiple",
				Message: "incorrect field count",
			})
		}

		// Extract fields by index
		track := Track{}
		if idx := colIndex["#"]; idx != -1 && idx < len(fields) {
			if pos, err := strconv.Atoi(strings.TrimSpace(fields[idx])); err == nil {
				track.Position = pos
			}
		}
		if idx := colIndex["Track Title"]; idx != -1 && idx < len(fields) {
			track.Title = strings.TrimSpace(fields[idx])
		}
		if idx := colIndex["Artist"]; idx != -1 && idx < len(fields) {
			track.Artist = strings.TrimSpace(fields[idx])
		}
		if idx := colIndex["Album"]; idx != -1 && idx < len(fields) {
			track.Album = strings.TrimSpace(fields[idx])
		}
		if idx := colIndex["Genre"]; idx != -1 && idx < len(fields) {
			track.Genre = strings.TrimSpace(fields[idx])
		}
		if idx := colIndex["BPM"]; idx != -1 && idx < len(fields) {
			if bpm, err := strconv.Atoi(strings.TrimSpace(fields[idx])); err == nil {
				track.BPM = float64(bpm)
			}
		}
		if idx := colIndex["Rating"]; idx != -1 && idx < len(fields) {
			if rating, err := strconv.Atoi(strings.TrimSpace(fields[idx])); err == nil {
				track.Rating = rating
			}
		}
		if idx := colIndex["Time"]; idx != -1 && idx < len(fields) {
			if durationSecs, err := parseTimeToSeconds(strings.TrimSpace(fields[idx])); err == nil {
				track.DurationSeconds = durationSecs
			}
		}
		if idx := colIndex["Key"]; idx != -1 && idx < len(fields) {
			track.MusicalKey = strings.TrimSpace(fields[idx])
		}
		if idx := colIndex["Date Added"]; idx != -1 && idx < len(fields) {
			if dateAdded, err := parseDate(strings.TrimSpace(fields[idx])); err == nil {
				track.DateAdded = dateAdded
			}
		}

		// Only add track if it has a title (required field)
		if track.Title != "" {
			tracks = append(tracks, track)
		}
	}

	if len(tracks) == 0 {
		return nil, warnings, ErrZeroTracks
	}

	return tracks, warnings, nil
}

// buildColumnIndex creates a map from column names to their indices
func buildColumnIndex(header []string) map[string]int {
	index := make(map[string]int)
	for i, col := range header {
		// Trim whitespace and remove BOM if present
		cleanedCol := strings.TrimSpace(col)
		cleanedCol = strings.TrimPrefix(cleanedCol, "\ufeff")
		index[cleanedCol] = i
	}
	return index
}

// parseTimeToSeconds converts a time string like "4:48" or "04:48" to seconds
func parseTimeToSeconds(timeStr string) (int, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0, errors.New("invalid time format")
	}

	minutes, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}

	seconds, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}

	if minutes < 0 || seconds < 0 || seconds >= 60 {
		return 0, errors.New("invalid time value")
	}

	return minutes*60 + seconds, nil
}

// parseDate converts a date string like "2025/10/23" to time.Time
func parseDate(dateStr string) (time.Time, error) {
	// Expected format: YYYY/MM/DD
	parts := strings.Split(dateStr, "/")
	if len(parts) != 3 {
		return time.Time{}, errors.New("invalid date format")
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, err
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, err
	}

	day, err := strconv.Atoi(parts[2])
	if err != nil {
		return time.Time{}, err
	}

	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), nil
}

// DetectFormat determines the format based on column names present
func DetectFormat(header []string) string {
	// Normalize header names
	normalized := make([]string, len(header))
	for i, h := range header {
		normalized[i] = strings.TrimSpace(h)
	}

	// Check for required columns for each format
	rekordboxCols := map[string]bool{
		"Track Title": true,
		"Artist":      true,
		"Album":       true,
		"Genre":       true,
		"BPM":         true,
		"Rating":      true,
		"Time":        true,
		"Key":         true,
		"Date Added":  true,
	}

	seratoCols := map[string]bool{
		"Track Title": true,
		"Artist":      true,
		"Album":       true,
		"Genre":       true,
		"BPM":         true,
		"Rating":      true,
		"Length":      true, // Serato uses Length instead of Time
		"Key":         true,
		"Added Date":  true, // Serato uses Added Date
	}

	traktorCols := map[string]bool{
		"Track Title": true,
		"Artist":      true,
		"Album":       true,
		"Genre":       true,
		"BPM":         true,
		"Rating":      true,
		"Length":      true, // Traktor uses Length
		"Key":         true,
		"Date":        true, // Traktor uses Date
	}

	// Count matches for each format
	rekordboxMatch := 0
	for col := range rekordboxCols {
		for _, h := range normalized {
			if h == col {
				rekordboxMatch++
				break
			}
		}
	}

	seratoMatch := 0
	for col := range seratoCols {
		for _, h := range normalized {
			if h == col {
				seratoMatch++
				break
			}
		}
	}

	traktorMatch := 0
	for col := range traktorCols {
		for _, h := range normalized {
			if h == col {
				traktorMatch++
				break
			}
		}
	}

	// Return the format with the most matches (minimum 7 matches needed)
	if rekordboxMatch >= 7 {
		return "rekordbox"
	}
	if seratoMatch >= 7 {
		return "serato"
	}
	if traktorMatch >= 7 {
		return "traktor"
	}

	return "unknown"
}
