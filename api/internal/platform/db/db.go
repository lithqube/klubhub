package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a new pgxpool connection pool, pings the database to verify
// connectivity, and returns the pool. Caller is responsible for calling Close().
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
