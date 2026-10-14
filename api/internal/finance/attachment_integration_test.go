package finance

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// requireAttPG skips without Postgres and guarantees the attachments table is
// empty before and after: the entry tests clean up with DELETE FROM
// finance_entries, which the RESTRICT foreign key would block.
func requireAttPG(t *testing.T) {
	t.Helper()
	requireEntryPG(t)
	clean := func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM finance_entry_attachments`) }
	clean()
	t.Cleanup(clean)
}

func newExpenseForAtt(t *testing.T, svc *EntryService, note string) *Entry {
	t.Helper()
	return createEntryForTest(t, svc, CreateEntryRequest{
		Kind: EntryKindExpense, AmountMinor: 1200, Currency: "EUR", Category: "gear", EntryDate: "2026-10-02", Notes: note,
	})
}

func newAttNew(entryID uuid.UUID, name string) NewAttachment {
	return NewAttachment{
		EntryID: entryID, StorageKey: "finance/entries/" + entryID.String() + "/" + uuid.NewString(), Filename: name,
		MimeType: "application/pdf", SizeBytes: 100, ChecksumSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
}

func TestIntegration_AttachmentRepo_RoundTripAndScoping(t *testing.T) {
	requireAttPG(t)
	ctx := context.Background()
	entries := NewEntryService(NewEntryRepository(testPool))
	repo := NewAttachmentRepository(testPool)
	a, b := newExpenseForAtt(t, entries, "strings"), newExpenseForAtt(t, entries, "cables")

	first, err := repo.Create(ctx, newAttNew(a.ID, "one.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Create(ctx, newAttNew(a.ID, "two.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, newAttNew(b.ID, "other.pdf")); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List(ctx, a.ID)
	if err != nil || len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("list = %+v, %v (want the two files of entry a, oldest first)", list, err)
	}
	got, err := repo.Get(ctx, a.ID, first.ID)
	if err != nil || got.StorageKey != first.StorageKey || got.Filename != "one.pdf" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	// An attachment id from another entry never resolves.
	if _, err := repo.Get(ctx, b.ID, first.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Errorf("cross-entry get: %v, want ErrAttachmentNotFound", err)
	}

	key, err := repo.Delete(ctx, a.ID, first.ID)
	if err != nil || key != first.StorageKey {
		t.Fatalf("delete = %q, %v", key, err)
	}
	if _, err := repo.Delete(ctx, a.ID, first.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if left, _ := repo.List(ctx, a.ID); len(left) != 1 {
		t.Errorf("after delete: %d files left, want 1", len(left))
	}
}

func TestIntegration_AttachmentRepo_ConcurrentUploadsRespectTheLimit(t *testing.T) {
	requireAttPG(t)
	ctx := context.Background()
	e := newExpenseForAtt(t, NewEntryService(NewEntryRepository(testPool)), "strings")
	repo := NewAttachmentRepository(testPool)

	const attempts = 25
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, limited := 0, 0
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Create(ctx, newAttNew(e.ID, "r.pdf"))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrAttachmentLimit):
				limited++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != MaxAttachmentsPerEntry || limited != attempts-MaxAttachmentsPerEntry {
		t.Errorf("created %d, limited %d; want %d and %d (the entry row lock must serialise the count)",
			ok, limited, MaxAttachmentsPerEntry, attempts-MaxAttachmentsPerEntry)
	}
}

func TestIntegration_AttachmentRepo_EntryStateRules(t *testing.T) {
	requireAttPG(t)
	ctx := context.Background()
	entries := NewEntryService(NewEntryRepository(testPool))
	repo := NewAttachmentRepository(testPool)

	if err := repo.Ensure(ctx, uuid.New()); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("unknown entry: %v", err)
	}

	// Voided: the evidence stays readable, but nothing can be added or removed.
	voided := newExpenseForAtt(t, entries, "voided one")
	att, err := repo.Create(ctx, newAttNew(voided.ID, "keep.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entries.Void(ctx, voided.ID, voided.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if err := repo.Ensure(ctx, voided.ID); !errors.Is(err, ErrEntryInactive) {
		t.Errorf("voided Ensure: %v", err)
	}
	if _, err := repo.Create(ctx, newAttNew(voided.ID, "late.pdf")); !errors.Is(err, ErrEntryInactive) {
		t.Errorf("voided Create: %v", err)
	}
	if _, err := repo.Delete(ctx, voided.ID, att.ID); !errors.Is(err, ErrEntryInactive) {
		t.Errorf("voided Delete: %v", err)
	}
	if list, err := repo.List(ctx, voided.ID); err != nil || len(list) != 1 {
		t.Errorf("voided List = %v, %v; the receipt must stay visible", list, err)
	}
	if _, err := repo.Get(ctx, voided.ID, att.ID); err != nil {
		t.Errorf("voided Get: %v", err)
	}

	// Soft-deleted: the entry is gone from the API, its evidence is not.
	gone := newExpenseForAtt(t, entries, "deleted one")
	if _, err := repo.Create(ctx, newAttNew(gone.ID, "evidence.pdf")); err != nil {
		t.Fatal(err)
	}
	if err := entries.Delete(ctx, gone.ID, gone.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if err := repo.Ensure(ctx, gone.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("deleted Ensure: %v", err)
	}
	if _, err := repo.List(ctx, gone.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("deleted List: %v", err)
	}
	var kept int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM finance_entry_attachments WHERE entry_id=$1`, gone.ID).Scan(&kept); err != nil || kept != 1 {
		t.Errorf("soft-deleting an entry removed its evidence: %d rows, %v", kept, err)
	}
}

func TestIntegration_AttachmentRepo_DatabaseRejectsBadRows(t *testing.T) {
	requireAttPG(t)
	ctx := context.Background()
	e := newExpenseForAtt(t, NewEntryService(NewEntryRepository(testPool)), "strings")
	for name, mutate := range map[string]func(*NewAttachment){
		"svg mime":       func(n *NewAttachment) { n.MimeType = "image/svg+xml" },
		"html mime":      func(n *NewAttachment) { n.MimeType = "text/html" },
		"blank filename": func(n *NewAttachment) { n.Filename = "   " },
		"zero size":      func(n *NewAttachment) { n.SizeBytes = 0 },
		"short checksum": func(n *NewAttachment) { n.ChecksumSHA256 = "abc" },
		"upper checksum": func(n *NewAttachment) { n.ChecksumSHA256 = string(bytes.ToUpper([]byte(n.ChecksumSHA256))) },
	} {
		in := newAttNew(e.ID, "x.pdf")
		mutate(&in)
		if _, err := NewAttachmentRepository(testPool).Create(ctx, in); err == nil {
			t.Errorf("%s: the database accepted the row", name)
		}
	}
}

func TestIntegration_EntriesCarryAttachmentCountAndReceiptFilter(t *testing.T) {
	requireAttPG(t)
	ctx := context.Background()
	entries := NewEntryService(NewEntryRepository(testPool))
	repo := NewAttachmentRepository(testPool)
	with, without := newExpenseForAtt(t, entries, "with receipt"), newExpenseForAtt(t, entries, "without receipt")
	for _, n := range []string{"a.pdf", "b.pdf"} {
		if _, err := repo.Create(ctx, newAttNew(with.ID, n)); err != nil {
			t.Fatal(err)
		}
	}

	byID := func(list []*Entry) map[uuid.UUID]*Entry {
		m := map[uuid.UUID]*Entry{}
		for _, e := range list {
			m[e.ID] = e
		}
		return m
	}
	all, err := entries.List(ctx, EntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	m := byID(all)
	if m[with.ID].AttachmentCount != 2 || m[without.ID].AttachmentCount != 0 {
		t.Errorf("list counts = %d / %d, want 2 / 0", m[with.ID].AttachmentCount, m[without.ID].AttachmentCount)
	}

	// "Missing" means an active expense that still needs a receipt: an income
	// row (a gig payment, royalties) and a voided expense (it can no longer
	// take files) without files are not nagging candidates.
	createEntryForTest(t, entries, CreateEntryRequest{
		Kind: EntryKindIncome, AmountMinor: 5000, Currency: "EUR", Category: "gig_fee", EntryDate: "2026-10-02", Description: "royalties",
	})
	voidedNoFiles := newExpenseForAtt(t, entries, "voided, no files")
	if _, err := entries.Void(ctx, voidedNoFiles.ID, voidedNoFiles.UpdatedAt); err != nil {
		t.Fatal(err)
	}

	missing, _ := entries.List(ctx, EntryFilter{Receipt: ReceiptMissing})
	present, _ := entries.List(ctx, EntryFilter{Receipt: ReceiptPresent})
	if len(missing) != 1 || missing[0].ID != without.ID || len(present) != 1 || present[0].ID != with.ID {
		t.Errorf("receipt filter: missing=%v present=%v (want only the active expense without files, and only the one with files)", ids(missing), ids(present))
	}

	// Every read path that returns an entry reports the count, so the UI never
	// shows 0 receipts for an entry it just edited or fetched.
	got, err := entries.Get(ctx, with.ID)
	if err != nil || got.AttachmentCount != 2 {
		t.Errorf("Get count = %v, %v", got, err)
	}
	upd, err := entries.Update(ctx, with.ID, UpdateEntryRequest{
		Kind: EntryKindExpense, AmountMinor: 1300, Currency: "EUR", Category: "gear", EntryDate: "2026-10-02", Notes: "with receipt", UpdatedAt: with.UpdatedAt,
	})
	if err != nil || upd.AttachmentCount != 2 {
		t.Errorf("Update count = %v, %v", upd, err)
	}
	voided, err := entries.Void(ctx, with.ID, upd.UpdatedAt)
	if err != nil || voided.AttachmentCount != 2 {
		t.Errorf("Void count = %v, %v", voided, err)
	}
}

func ids(list []*Entry) []uuid.UUID {
	out := make([]uuid.UUID, len(list))
	for i, e := range list {
		out[i] = e.ID
	}
	return out
}

// The handler, service and Postgres repository together, with an in-memory
// object store: what the API does for a phone upload.
func TestIntegration_AttachmentsEndToEnd(t *testing.T) {
	requireAttPG(t)
	entries := NewEntryService(NewEntryRepository(testPool))
	store := newAttStore()
	h := NewAttachmentHandler(NewAttachmentService(NewAttachmentRepository(testPool), store, "bucket"))
	hs := &attHarness{t: t, repo: nil, store: store, h: h, entry: newExpenseForAtt(t, entries, "strings").ID}

	rec := hs.upload(hs.entry, "IMG_2041.JPG", jpegBytes)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	a := decodeAttachment(t, rec)
	if a.MimeType != "image/jpeg" || a.SizeBytes != int64(len(jpegBytes)) {
		t.Errorf("attachment = %+v", a)
	}

	dl := hs.do(http.MethodGet, hs.path(hs.entry, a.ID.String())+"?inline=1")
	if dl.Code != http.StatusOK || !bytes.Equal(dl.Body.Bytes(), jpegBytes) {
		t.Errorf("download: %d, %d bytes", dl.Code, dl.Body.Len())
	}
	if rec := hs.do(http.MethodDelete, hs.path(hs.entry, a.ID.String())); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rec.Code)
	}
	if store.count() != 0 {
		t.Errorf("object left in storage after delete")
	}
}
