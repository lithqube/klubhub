package rider

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/klubhub/dj/api/internal/gig"
	"github.com/klubhub/dj/api/internal/settings"
)

// ─── Input limits ───────────────────────────────────────────────────────────

func TestService_CreateTemplate_Limits(t *testing.T) {
	cases := []struct {
		name    string
		in      CreateTemplateInput
		wantErr string // "" = accepted
	}{
		{"name at the limit", CreateTemplateInput{Name: strings.Repeat("n", MaxNameChars)}, ""},
		{"name over the limit", CreateTemplateInput{Name: strings.Repeat("n", MaxNameChars+1)}, "name is too long"},
		{"limit counts characters, not bytes", CreateTemplateInput{Name: strings.Repeat("é", MaxNameChars)}, ""},
		{"NUL in name", CreateTemplateInput{Name: "a\x00b"}, "name contains a NUL"},
		{"section at the limit", CreateTemplateInput{Name: "x", RiderSectionValues: RiderSectionValues{Technical: strings.Repeat("t", MaxSectionChars)}}, ""},
		{"section over the limit", CreateTemplateInput{Name: "x", RiderSectionValues: RiderSectionValues{Backline: strings.Repeat("b", MaxSectionChars+1)}}, "backline is too long"},
		{"NUL in a section", CreateTemplateInput{Name: "x", RiderSectionValues: RiderSectionValues{OtherNotes: "a\x00"}}, "otherNotes contains a NUL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeRepo(), &fakeStorage{}, &fakeSettings{})
			_, err := svc.CreateTemplate(context.Background(), tc.in)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestService_UpdateTemplate_Limits_OnlyValidatesWhatIsSet(t *testing.T) {
	repo := newFakeRepo()
	tpl, _ := repo.CreateTemplate(context.Background(), CreateTemplateInput{Name: "T"})
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	long, nul, ok := strings.Repeat("x", MaxSectionChars+1), "a\x00", "fine"

	_, err := svc.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Technical: &long}, UpdatedAt: tpl.UpdatedAt})
	require.ErrorIs(t, err, ErrInvalidInput)
	_, err = svc.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Hospitality: &nul}, UpdatedAt: tpl.UpdatedAt})
	require.ErrorIs(t, err, ErrInvalidInput)
	tooLongName := strings.Repeat("n", MaxNameChars+1)
	_, err = svc.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{Name: &tooLongName, UpdatedAt: tpl.UpdatedAt})
	require.ErrorIs(t, err, ErrInvalidInput)

	// Valid values for the fields that are set go through; absent ones are not inspected.
	got, err := svc.UpdateTemplate(context.Background(), tpl.ID, UpdateTemplateInput{
		RiderSectionPatch: RiderSectionPatch{Backline: &ok}, UpdatedAt: tpl.UpdatedAt})
	require.NoError(t, err)
	assert.Equal(t, "fine", got.Backline)
}

func TestService_Attachments_Limits(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	long, nul := strings.Repeat("x", MaxSectionChars+1), "a\x00"

	_, err := svc.CreateAttachment(context.Background(), CreateAttachmentInput{
		GigID: uuid.New(), RiderSectionValues: RiderSectionValues{Technical: long}})
	require.ErrorIs(t, err, ErrInvalidInput, "manual entry is validated")

	att, err := svc.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: uuid.New()})
	require.NoError(t, err)
	_, err = svc.UpdateAttachment(context.Background(), att.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{Technical: &long}, UpdatedAt: att.UpdatedAt})
	require.ErrorIs(t, err, ErrInvalidInput)
	_, err = svc.UpdateAttachment(context.Background(), att.ID, UpdateAttachmentInput{
		RiderSectionPatch: RiderSectionPatch{OtherNotes: &nul}, UpdatedAt: att.UpdatedAt})
	require.ErrorIs(t, err, ErrInvalidInput)
}

// ─── PDF export: honest errors, cleanup ─────────────────────────────────────

type fakeGigReader struct {
	gig *gig.Gig
	err error
}

func (f *fakeGigReader) GetGig(context.Context, uuid.UUID) (*gig.Gig, error) { return f.gig, f.err }

func exportSetup(t *testing.T, reader GigReader) (*Service, *fakeStorage, *RiderAttachment) {
	t.Helper()
	repo := newFakeRepo()
	storage := &fakeStorage{}
	svc := NewService(repo, storage, &fakeSettings{})
	svc.SetGigReader(reader)
	att, err := svc.CreateAttachment(context.Background(), CreateAttachmentInput{GigID: uuid.New(),
		RiderSectionValues: RiderSectionValues{Technical: "2× CDJ-3000"}})
	require.NoError(t, err)
	return svc, storage, att
}

func TestService_GeneratePDF_GigGone_IsNotFound_AndStoresNothing(t *testing.T) {
	svc, storage, att := exportSetup(t, &fakeGigReader{err: gig.ErrNotFound})
	_, err := svc.GeneratePDF(context.Background(), att.ID)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Empty(t, storage.putKeys, "no PDF with an invented venue may be produced")
}

func TestService_GeneratePDF_GigLookupFailure_AbortsInsteadOfInventingAVenue(t *testing.T) {
	svc, storage, att := exportSetup(t, &fakeGigReader{err: errors.New("db: connection reset")})
	_, err := svc.GeneratePDF(context.Background(), att.ID)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound, "a transient failure is not 'gone'")
	assert.Empty(t, storage.putKeys)
}

func TestService_GeneratePDF_NilGig_IsNotFound(t *testing.T) {
	svc, storage, att := exportSetup(t, &fakeGigReader{})
	_, err := svc.GeneratePDF(context.Background(), att.ID)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Empty(t, storage.putKeys)
}

func TestService_GeneratePDF_WithGig_Succeeds(t *testing.T) {
	svc, storage, att := exportSetup(t, &fakeGigReader{gig: &gig.Gig{Venue: "Berghain", City: "Berlin", Country: "DE", Date: time.Date(2026, 3, 14, 22, 0, 0, 0, time.UTC)}})
	_, err := svc.GeneratePDF(context.Background(), att.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"rider/exports/" + att.ID.String() + ".pdf"}, storage.putKeys)
}

func TestService_DeleteAttachment_RemovesTheExportedPDF(t *testing.T) {
	svc, storage, att := exportSetup(t, nil)
	require.NoError(t, svc.DeleteAttachment(context.Background(), att.ID))
	assert.Equal(t, []string{"rider/exports/" + att.ID.String() + ".pdf"}, storage.deletedKeys)
}

func TestService_DeleteAttachment_StorageFailureDoesNotFailTheDetach(t *testing.T) {
	svc, storage, att := exportSetup(t, nil)
	storage.deleteErr = errors.New("garage down")
	require.NoError(t, svc.DeleteAttachment(context.Background(), att.ID), "the detach already succeeded")
}

func TestService_DeleteAttachment_UnknownID_IsNotFound_AndTouchesNoStorage(t *testing.T) {
	svc, storage, _ := exportSetup(t, nil)
	require.ErrorIs(t, svc.DeleteAttachment(context.Background(), uuid.New()), ErrNotFound)
	assert.Empty(t, storage.deletedKeys)
}

// ─── PDF text ───────────────────────────────────────────────────────────────

func TestFormatVenueLine(t *testing.T) {
	cases := []struct{ name, venue, city, country, want string }{
		{"everything", "Berghain", "Berlin", "Germany", "Berghain — Berlin, Germany"},
		{"no country", "Berghain", "Berlin", "", "Berghain — Berlin"},
		{"no city", "Berghain", "", "Germany", "Berghain — Germany"},
		{"venue only (used to render 'V — ,')", "Berghain", "", "", "Berghain"},
		{"no venue name", "", "Berlin", "Germany", "Berlin, Germany"},
		{"city only (used to render 'Berlin,')", "", "Berlin", "", "Berlin"},
		{"whitespace counts as blank", "  ", " ", "\t", "Venue TBA"},
		{"nothing", "", "", "", "Venue TBA"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, formatVenueLine(tc.venue, tc.city, tc.country))
		})
	}
}

func TestPDFText(t *testing.T) {
	assert.Equal(t, "Тест Δοκιμή Zażółć €", pdfText("Тест Δοκιμή Zażółć €"), "BMP text passes through")
	assert.Equal(t, "set ? night", pdfText("set 🎧 night"), "astral code points would make fpdf fail the export")
	assert.Equal(t, "a\nb\tc", pdfText("a\r\nb\tc"), "\\r dropped, newline and tab kept")
	assert.Equal(t, "ab", pdfText("a\x07\x1bb"), "control characters dropped")
	assert.Equal(t, "日本語", pdfText("日本語"), "BMP characters without a glyph are left to the font")
}

// pdfStreams returns every zlib-compressed stream in the PDF, inflated.
func pdfStreams(pdf []byte) [][]byte {
	var out [][]byte
	for _, m := range regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`).FindAllSubmatch(pdf, -1) {
		zr, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		if raw, err := io.ReadAll(zr); err == nil {
			out = append(out, raw)
		}
	}
	return out
}

// utf16be is how fpdf writes text for UTF-8 fonts into the page content.
func utf16be(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u>>8), byte(u))
	}
	return b
}

// hasGlyph reports whether any embedded font has a glyph for r. Each UTF-8
// font carries a 128 KiB CIDToGIDMap (2 bytes per code point; 0 = no glyph).
func hasGlyph(streams [][]byte, r rune) bool {
	for _, s := range streams {
		if len(s) == 131072 && int(r)*2+1 < len(s) && (s[int(r)*2] != 0 || s[int(r)*2+1] != 0) {
			return true
		}
	}
	return false
}

func TestRenderRiderPDF_NonLatin1Text_IsRenderedNotReplacedByDots(t *testing.T) {
	sample := map[string]string{
		"Cyrillic": "Тест Москва",
		"Greek":    "Δοκιμή",
		"Polish":   "Zażółć gęślą jaźń",
		"Turkish":  "İşğüç",
		"Symbols":  "€ × →",
	}
	var all strings.Builder
	for _, v := range sample {
		all.WriteString(v + "\n")
	}
	att := &RiderAttachment{ID: uuid.New(), Technical: all.String()}
	var buf bytes.Buffer
	require.NoError(t, renderRiderPDF(att, "Club Москва", "Kraków", "PL", time.Time{}, &settings.UserSettings{DJName: "Łukasz"}, &buf))

	streams := pdfStreams(buf.Bytes())
	var content []byte
	for _, s := range streams {
		if bytes.Contains(s, []byte("Tj")) {
			content = append(content, s...)
		}
	}
	for name, text := range sample {
		for _, line := range strings.Split(text, "\n") {
			assert.True(t, bytes.Contains(content, utf16be(line)),
				"%s: %q must be written as real characters, not substituted", name, line)
		}
		for _, r := range text {
			if r != ' ' {
				assert.True(t, hasGlyph(streams, r), "%s: no glyph embedded for %q", name, r)
			}
		}
	}
	assert.Zero(t, bytes.Count(buf.Bytes(), []byte("Helvetica")), "no core font should be in use")
}

func TestRenderRiderPDF_Emoji_DoesNotFailTheExport(t *testing.T) {
	att := &RiderAttachment{ID: uuid.New(), Technical: "Booth ready 🎧 doors 22:00"}
	var buf bytes.Buffer
	require.NoError(t, renderRiderPDF(att, "Club 🎉", "", "", time.Time{}, &settings.UserSettings{DJName: "DJ 🎧"}, &buf),
		"fpdf rejects astral code points; they must be sanitised first")
	assert.Equal(t, 1, pdfPages(buf.Bytes()))
}


// ─── Configured bucket ──────────────────────────────────────────────────────

func TestService_UsesTheConfiguredBucketForEveryStorageCall(t *testing.T) {
	svc, storage, att := exportSetup(t, nil)
	svc.SetBucket("my-garage-bucket")

	_, err := svc.GeneratePDF(context.Background(), att.ID)
	require.NoError(t, err)
	require.NoError(t, svc.DeleteAttachment(context.Background(), att.ID))

	assert.Equal(t, "my-garage-bucket", storage.lastPutBucket)
	assert.Equal(t, "my-garage-bucket", storage.lastPresignBucket)
	assert.Equal(t, "my-garage-bucket", storage.lastDeleteBucket)
}

func TestService_DefaultsToTheStandardBucket_AndIgnoresAnEmptyOverride(t *testing.T) {
	svc, storage, att := exportSetup(t, nil)
	svc.SetBucket("") // an unset config value must not blank the bucket

	_, err := svc.GeneratePDF(context.Background(), att.ID)
	require.NoError(t, err)
	assert.Equal(t, StorageBucket, storage.lastPutBucket)
}

// ─── Unique template names ──────────────────────────────────────────────────

func TestService_TemplateNames_AreUniqueCaseInsensitively(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeStorage{}, &fakeSettings{})
	ctx := context.Background()
	first, err := svc.CreateTemplate(ctx, CreateTemplateInput{Name: "Standard club"})
	require.NoError(t, err)

	_, err = svc.CreateTemplate(ctx, CreateTemplateInput{Name: "standard CLUB"})
	require.ErrorIs(t, err, ErrInvalidInput, "422, not the 409 the editors read as 'changed elsewhere'")
	assert.NotErrorIs(t, err, ErrConflict)
	assert.NotErrorIs(t, err, ErrStaleUpdate)
	assert.Contains(t, err.Error(), "already exists")

	other, err := svc.CreateTemplate(ctx, CreateTemplateInput{Name: "Festival"})
	require.NoError(t, err)
	taken := "STANDARD CLUB"
	_, err = svc.UpdateTemplate(ctx, other.ID, UpdateTemplateInput{Name: &taken, UpdatedAt: other.UpdatedAt})
	require.ErrorIs(t, err, ErrInvalidInput, "renaming into an existing name is refused too")

	// Keeping your own name (e.g. changing only its capitalisation) is fine.
	same := "STANDARD club"
	_, err = svc.UpdateTemplate(ctx, first.ID, UpdateTemplateInput{Name: &same, UpdatedAt: first.UpdatedAt})
	require.NoError(t, err)
}

func TestService_DeletedTemplate_FreesItsName(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeStorage{}, &fakeSettings{})
	ctx := context.Background()
	tpl, _ := svc.CreateTemplate(ctx, CreateTemplateInput{Name: "Reusable"})
	require.NoError(t, svc.DeleteTemplate(ctx, tpl.ID))
	_, err := svc.CreateTemplate(ctx, CreateTemplateInput{Name: "reusable"})
	require.NoError(t, err)
}

// ─── Deleting a template detaches it from its attachments ───────────────────

func TestService_DeleteTemplate_ClearsTheReference_KeepsTheContent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, &fakeStorage{}, &fakeSettings{})
	ctx := context.Background()
	tpl, _ := svc.CreateTemplate(ctx, CreateTemplateInput{Name: "Source",
		RiderSectionValues: RiderSectionValues{Technical: "2× CDJ-3000"}})
	att, err := svc.CreateAttachment(ctx, CreateAttachmentInput{GigID: uuid.New(), TemplateID: &tpl.ID})
	require.NoError(t, err)
	require.NotNil(t, att.TemplateID)

	require.NoError(t, svc.DeleteTemplate(ctx, tpl.ID))

	got, err := svc.GetAttachment(ctx, att.ID)
	require.NoError(t, err)
	assert.Nil(t, got.TemplateID, "no pointer to a template that no longer exists")
	assert.Equal(t, "2× CDJ-3000", got.Technical, "the per-gig copy keeps its content")
	assert.True(t, got.UpdatedAt.Equal(att.UpdatedAt), "open editors' updatedAt tokens must stay valid")
}
