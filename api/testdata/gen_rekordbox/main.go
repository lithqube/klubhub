package main

import (
	"os"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func main() {
	// Create the Rekordbox TSV content
	// Header: #\tArtwork\tTrack Title\tArtist\tAlbum\tGenre\tBPM\tRating\tTime\tKey\tDate Added
	// Row 1: Position=1, Artwork=empty, Title="AFRICA (TOTO)", Artist=empty, Album=empty, Genre="Mix", BPM=103, Rating=0, Time="4:48", Key="8B", Date="2025/10/23"
	content := "#\tArtwork\tTrack Title\tArtist\tAlbum\tGenre\tBPM\tRating\tTime\tKey\tDate Added\n" +
		"1\t\tAFRICA (TOTO)\t\t\tMix\t103\t0\t4:48\t8B\t2025/10/23\n" +
		"2\t\t夜桜お七\t小林幸子\t\tJ-Pop\t128\t0\t6:00\t10A\t2025/10/20\n" +
		"3\t\tBohemian Rhapsody\tQueen\t\tRock\t75\t0\t3:55\t3A\t2025/10/19\n" +
		"4\t\tHotel California\tEagles\t\tRock\t63\t0\t6:30\t4B\t2025/10/18\n" +
		"5\t\tImagine\tJohn Lennon\t\tPop\t75\t0\t3:03\t5A\t2025/10/17\n"

	// Convert to UTF-16 LE with BOM
	utf16le := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
	writer := transform.NewWriter(os.Stdout, utf16le.NewEncoder())
	writer.Write([]byte(content))
	writer.Close()
}
