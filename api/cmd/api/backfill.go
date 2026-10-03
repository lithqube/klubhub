package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/klubhub/dj/api/internal/finance"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/db"
	applog "github.com/klubhub/dj/api/internal/platform/log"
	"github.com/klubhub/dj/api/internal/platform/storage"
)

// runBackfillArchive is `api -backfill-archive [-dry-run]`: it archives the
// PDF (and, when the invoice can be exported, the e-invoice XML and its
// validation report) of every invoice and credit note that was issued before
// archiving existed. It is meant to be run by hand once, after the server has
// started and migrated the database:
//
//	docker compose exec app /api -backfill-archive -dry-run
//	docker compose exec app /api -backfill-archive
//
// It is idempotent and safe to repeat or to run beside the live server.
// Returns the process exit code: 0 on success, 1 if any invoice failed.
func runBackfillArchive(dryRun bool) int {
	return backfillArchive(os.Stdout, os.Stderr, dryRun)
}

func backfillArchive(stdout, stderr io.Writer, dryRun bool) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "backfill: load config: %v\n", err)
		return 1
	}
	logger := applog.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(stderr, "backfill: connect database: %v\n", err)
		return 1
	}
	defer pool.Close()

	storeClient, err := storage.New(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "backfill: create storage client: %v\n", err)
		return 1
	}
	_, invoiceSvc := newInvoiceService(cfg, pool, storeClient, logger)

	rep, err := invoiceSvc.BackfillArchive(ctx, finance.BackfillOptions{
		DryRun: dryRun,
		Progress: func(done, total int, number, outcome string) {
			fmt.Fprintf(stdout, "[%d/%d] %s: %s\n", done, total, number, outcome)
		},
	})
	if err != nil {
		fmt.Fprintf(stderr, "backfill: %v\n", err)
		if rep == nil {
			return 1
		}
	}
	printBackfillReport(stdout, rep)
	if err != nil || len(rep.Failed) > 0 {
		return 1
	}
	return 0
}

func printBackfillReport(w io.Writer, rep *finance.BackfillReport) {
	verb := "archived"
	if rep.DryRun {
		verb = "would be archived (dry run, nothing written)"
	}
	fmt.Fprintf(w, "\n%d numbered invoices and credit notes\n", rep.Total)
	fmt.Fprintf(w, "  already complete: %d\n  %s: %d\n", rep.Complete, verb, rep.Archived)
	if !rep.DryRun {
		kinds := make([]string, 0, len(rep.Created))
		for k := range rep.Created {
			kinds = append(kinds, string(k))
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			fmt.Fprintf(w, "  new %s documents: %d\n", k, rep.Created[finance.DocumentKind(k)])
		}
		fmt.Fprintf(w, "  without an e-invoice afterwards: %d\n", rep.WithoutEInvoice)
	}
	if len(rep.Failed) > 0 {
		fmt.Fprintf(w, "  FAILED: %d\n", len(rep.Failed))
		for _, f := range rep.Failed {
			fmt.Fprintf(w, "    %s (%s): %s\n", f.Number, f.ID, f.Error)
		}
	}
	if !rep.DryRun && rep.Archived > 0 {
		fmt.Fprintln(w, "\nThese documents were rendered now from the stored invoices; they are marked 'backfill', not the files produced when the invoices were issued.")
	}
}
