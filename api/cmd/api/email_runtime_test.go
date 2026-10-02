package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klubhub/dj/api/internal/finance"
	"github.com/klubhub/dj/api/internal/platform/config"
	"github.com/klubhub/dj/api/internal/platform/migrations"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestFinanceEmailRuntimeStartupAndDrain(t *testing.T) {
	if testing.Short() {
		t.Skip("disposable migrated postgres")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	container, err := tcpostgres.RunContainer(ctx, testcontainers.WithImage("postgres:16-alpine"), tcpostgres.WithDatabase("email_runtime"), tcpostgres.WithUsername("test"), tcpostgres.WithPassword("test"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	defer container.Terminate(context.Background())
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = migrations.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := finance.NewEmailRepository(pool)
	req := finance.CreateEmailRequest{Kind: finance.EmailKindInvoiceIssued, FromEmail: "from@example.com", ToEmail: "to@example.com", Subject: "test", Body: "test"}
	// Durable row from the previous process, before runtime boot.
	msg, err := finance.NewEmailService(repo, nil).Enqueue(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	called := make(chan string, 1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- r.Header.Get("Idempotency-Key")
		w.Write([]byte(`{"success":true,"data":{"emails":[{"contact":{"email":"to@example.com"},"email":"ac32f08e-c6b9-45d3-9824-a73dff1e3bbf"}],"timestamp":"2025-01-15T10:30:00Z"}}`))
	}))
	defer provider.Close()
	cfg := &config.Config{PlunkBaseURL: provider.URL, PlunkAPIKey: "local-fixture", PlunkProjectID: "fixture", PlunkFromEmail: "from@example.com"}
	handler, done := buildFinanceRuntime(ctx, cfg, pool, nil, nil, silentLogger())
	if handler == nil {
		t.Fatal("no finance routes")
	}
	select {
	case key := <-called:
		if key != msg.ID.String() {
			t.Fatalf("different durable key %s", key)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("startup did not recover queued email")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		row, err := repo.GetByID(context.Background(), msg.ID)
		if err != nil {
			t.Fatal(err)
		}
		if row.Status == finance.EmailStatusSent {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("startup did not finalize %+v", row)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not drain")
	}
	persisted, err := repo.GetByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != finance.EmailStatusSent || persisted.Attempts != 1 {
		t.Fatalf("bad persisted startup result %+v", persisted)
	}
}
