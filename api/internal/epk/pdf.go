package epk

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"codeberg.org/go-pdf/fpdf"
	"github.com/klubhub/dj/api/internal/settings"
)

// listItemRe matches ordered list items such as "1. item" or "12. item".
var listItemRe = regexp.MustCompile(`^\d+\.\s`)

// boldRe matches **bold** spans.
var boldRe = regexp.MustCompile(`\*\*(.+?)\*\*`)

// italicRe matches *italic* spans (single asterisk, not double).
var italicRe = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)

// linkRe matches [text](url) spans.
var linkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// tr converts UTF-8 to cp1252, the encoding of fpdf's built-in core fonts.
// Writing UTF-8 straight into them garbles anything non-ASCII ("•" printed as
// "â€¢", "·" as "Â·"). Characters outside cp1252 still degrade; rendering
// those needs an embedded UTF-8 font.
var tr = fpdf.New("P", "mm", "A4", "").UnicodeTranslatorFromDescriptor("")

// Section heading text is printed in dark ink: the accent defaults to the
// app's light cyan, which is unreadable as text on a white page. The accent
// is still used for the rules so the brand colour carries through.
const headingInkR, headingInkG, headingInkB = 26, 26, 26

// renderEPKPDF generates a white A4 portrait PDF with 20mm margins.
// The PDF includes all non-empty sections that are enabled in sectionVisibility.
// Section order: Bio → Photos → Gig Highlights → Press Quotes → Tech Rider → Stage Plot → Social Links → Contact Info
func renderEPKPDF(content *EPKContent, userSettings *settings.UserSettings, buf *bytes.Buffer) error {
	f := fpdf.New("P", "mm", "A4", "")
	f.SetMargins(20, 20, 20)
	f.SetAutoPageBreak(true, 20)
	f.AddPage()

	// Determine accent colour.
	accentHex := "#96F8FF"
	if userSettings.DefaultColors != nil {
		if v, ok := userSettings.DefaultColors["primary"]; ok {
			if s, ok := v.(string); ok && s != "" {
				accentHex = s
			}
		}
	}
	ar, ag, ab := hexToRGB(accentHex)

	// ── Header: DJ name (and logo placeholder) ──────────────────────────────
	f.SetFont("Helvetica", "B", 18)
	f.SetTextColor(headingInkR, headingInkG, headingInkB)
	djName := userSettings.DJName
	if djName == "" {
		djName = "ARTIST EPK"
	}
	f.MultiCell(0, 12, tr(strings.ToUpper(djName)), "", "L", false)
	f.SetDrawColor(ar, ag, ab)
	f.SetLineWidth(0.8)
	f.Line(20, f.GetY(), 190, f.GetY())
	f.Ln(6)

	// ── Bio section ──────────────────────────────────────────────────────────
	if sectionEnabled(content.SectionVisibility, "bio") {
		hasBio := !isBlank(content.BioShort) || !isBlank(content.BioLong)
		if hasBio {
			writeSectionHeading(f, "BIO", ar, ag, ab)

			if !isBlank(content.BioShort) {
				f.SetFont("Helvetica", "B", 10)
				f.SetTextColor(26, 26, 26)
				f.MultiCell(0, 5, tr(content.BioShort), "", "L", false)
				f.Ln(3)
			}

			// An imported bio often fills both fields with the same text.
			if !isBlank(content.BioLong) && strings.TrimSpace(content.BioLong) != strings.TrimSpace(content.BioShort) {
				f.SetFont("Helvetica", "", 10)
				f.SetTextColor(26, 26, 26)
				renderMarkdown(f, content.BioLong)
				f.Ln(3)
			}
		}
	}

	// ── Press Photos section ─────────────────────────────────────────────────
	// Note: photos are stored as Garage object keys; we include a reference list
	// in the PDF (full photo embedding requires an S3 byte fetch, which is out of
	// scope for the PDF renderer — paths are listed as references).
	if sectionEnabled(content.SectionVisibility, "photos") && len(content.PhotoPaths) > 0 {
		writeSectionHeading(f, "PRESS PHOTOS", ar, ag, ab)
		f.SetFont("Helvetica", "", 9)
		f.SetTextColor(26, 26, 26)
		f.MultiCell(0, 5, tr(fmt.Sprintf("%d press photo(s) available on request.", len(content.PhotoPaths))), "", "L", false)
		f.Ln(3)
	}

	// ── Gig Highlights section ───────────────────────────────────────────────
	if sectionEnabled(content.SectionVisibility, "gig_highlights") && len(content.GigHighlights) > 0 {
		writeSectionHeading(f, "GIG HIGHLIGHTS", ar, ag, ab)
		f.SetFont("Helvetica", "", 10)
		f.SetTextColor(26, 26, 26)
		for _, h := range content.GigHighlights {
			if !isBlank(h) {
				f.MultiCell(0, 5, tr("• "+h), "", "L", false)
			}
		}
		f.Ln(3)
	}

	// ── Press Quotes section ─────────────────────────────────────────────────
	if sectionEnabled(content.SectionVisibility, "press_quotes") && len(content.PressQuotes) > 0 {
		writeSectionHeading(f, "PRESS QUOTES", ar, ag, ab)
		f.SetFont("Helvetica", "", 10)
		f.SetTextColor(26, 26, 26)
		for _, q := range content.PressQuotes {
			if !isBlank(q.Text) {
				f.SetFont("Helvetica", "I", 10)
				f.MultiCell(0, 5, tr(fmt.Sprintf("\"%s\"", q.Text)), "", "L", false)
				if !isBlank(q.Source) {
					f.SetFont("Helvetica", "", 9)
					f.SetTextColor(80, 80, 80)
					f.MultiCell(0, 4, tr("— "+q.Source), "", "R", false)
					f.SetTextColor(26, 26, 26)
				}
				f.Ln(2)
			}
		}
		f.Ln(2)
	}

	// ── Tech Rider section ───────────────────────────────────────────────────
	if sectionEnabled(content.SectionVisibility, "tech_rider") && !isBlank(content.TechRider) {
		writeSectionHeading(f, "TECH RIDER", ar, ag, ab)
		f.SetFont("Helvetica", "", 10)
		f.SetTextColor(26, 26, 26)
		f.MultiCell(0, 5, tr(content.TechRider), "", "L", false)
		f.Ln(3)
	}

	// ── Stage Plot section ───────────────────────────────────────────────────
	if sectionEnabled(content.SectionVisibility, "stage_plot") && !isBlank(content.StagePlotPath) {
		writeSectionHeading(f, "STAGE PLOT", ar, ag, ab)
		f.SetFont("Helvetica", "I", 9)
		f.SetTextColor(80, 80, 80)
		f.MultiCell(0, 5, tr("Stage plot available on request."), "", "L", false)
		f.Ln(3)
	}

	// ── Social Links section ─────────────────────────────────────────────────
	if sectionEnabled(content.SectionVisibility, "social_links") {
		socialLinks := formatSocialLinks(userSettings)
		if socialLinks != "" {
			writeSectionHeading(f, "SOCIAL LINKS", ar, ag, ab)
			f.SetFont("Helvetica", "", 10)
			f.SetTextColor(26, 26, 26)
			f.MultiCell(0, 5, tr(socialLinks), "", "L", false)
			f.Ln(3)
		}
	}

	// ── Contact Info section ─────────────────────────────────────────────────
	if sectionEnabled(content.SectionVisibility, "contact_info") && !isBlank(userSettings.ContactInfo) {
		writeSectionHeading(f, "CONTACT", ar, ag, ab)
		f.SetFont("Helvetica", "", 10)
		f.SetTextColor(26, 26, 26)
		f.MultiCell(0, 5, tr(userSettings.ContactInfo), "", "L", false)
		f.Ln(3)
	}

	return f.Output(buf)
}

// writeSectionHeading renders an ALL_CAPS section heading in accent colour followed by a divider.
func writeSectionHeading(f *fpdf.Fpdf, title string, r, g, b int) {
	f.SetFont("Helvetica", "B", 11)
	f.SetTextColor(headingInkR, headingInkG, headingInkB)
	f.MultiCell(0, 8, tr(title), "", "L", false)
	f.SetDrawColor(r, g, b)
	f.SetLineWidth(0.3)
	f.Line(20, f.GetY(), 190, f.GetY())
	f.Ln(4)
}

// renderMarkdown renders a Markdown string into the fpdf document.
// Supported constructs: ## H2, ### H3, **bold**, *italic*, - list, 1. ordered list, [text](url).
func renderMarkdown(f *fpdf.Fpdf, md string) {
	lines := strings.Split(md, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "### "):
			f.SetFont("Helvetica", "B", 10)
			f.SetTextColor(26, 26, 26)
			f.MultiCell(0, 5, tr(strings.TrimPrefix(trimmed, "### ")), "", "L", false)
			f.SetFont("Helvetica", "", 10)

		case strings.HasPrefix(trimmed, "## "):
			f.SetFont("Helvetica", "B", 11)
			f.SetTextColor(26, 26, 26)
			f.MultiCell(0, 6, tr(strings.TrimPrefix(trimmed, "## ")), "", "L", false)
			f.SetFont("Helvetica", "", 10)

		case strings.HasPrefix(trimmed, "- "):
			text := renderInline(strings.TrimPrefix(trimmed, "- "))
			f.SetFont("Helvetica", "", 10)
			f.SetTextColor(26, 26, 26)
			f.MultiCell(0, 5, tr("• "+text), "", "L", false)

		case listItemRe.MatchString(trimmed):
			text := renderInline(trimmed)
			f.SetFont("Helvetica", "", 10)
			f.SetTextColor(26, 26, 26)
			f.MultiCell(0, 5, tr(text), "", "L", false)

		case trimmed == "":
			f.Ln(3)

		default:
			text := renderInline(trimmed)
			f.SetFont("Helvetica", "", 10)
			f.SetTextColor(26, 26, 26)
			f.MultiCell(0, 5, tr(text), "", "L", false)
		}
	}
}

// renderInline strips Markdown inline constructs (**bold**, *italic*, [text](url))
// and returns plain text suitable for fpdf MultiCell (no inline style switching per-run).
// Links are rendered as: text (url).
func renderInline(s string) string {
	// Remove links: [text](url) → text (url)
	s = linkRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := linkRe.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		return sub[1] + " (" + sub[2] + ")"
	})

	// Remove bold markers: **text** → text
	s = boldRe.ReplaceAllString(s, "$1")

	// Remove italic markers: *text* → text (single asterisk)
	s = italicRe.ReplaceAllString(s, " $1")
	s = strings.TrimSpace(s)

	return s
}

// hexToRGB converts a hex colour string (e.g. "#96F8FF" or "96F8FF") to RGB components 0-255.
// Returns (150, 248, 255) — the default cyan — on any parse error.
func hexToRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 150, 248, 255
	}
	r, err1 := strconv.ParseInt(hex[0:2], 16, 32)
	g, err2 := strconv.ParseInt(hex[2:4], 16, 32)
	b, err3 := strconv.ParseInt(hex[4:6], 16, 32)
	if err1 != nil || err2 != nil || err3 != nil {
		return 150, 248, 255
	}
	return int(r), int(g), int(b)
}

// formatSocialLinks builds a newline-delimited string from the settings social_links map.
func formatSocialLinks(userSettings *settings.UserSettings) string {
	if userSettings.SocialLinks == nil {
		return ""
	}
	var lines []string
	for platform, link := range userSettings.SocialLinks {
		if s, ok := link.(string); ok && s != "" {
			lines = append(lines, strings.Title(platform)+": "+s) //nolint:staticcheck
		}
	}
	return strings.Join(lines, "\n")
}
