// Package db owns the Postgres connection pool and a handful of thin
// repository helpers. The server uses pgx directly — no ORM — because the
// schema is stable and Pindoc's query shape is simple enough that the
// indirection of an ORM would cost more than it saves.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is a tiny wrapper around *pgxpool.Pool so downstream code can swap
// implementations in tests (e.g. a fake using the same query surface). For
// Phase 2 we expose the raw pool directly and add a typed layer on top.
type Pool struct {
	*pgxpool.Pool
}

// Open connects, pings, and returns a ready pool. Caller must Close.
func Open(ctx context.Context, dsn string) (*Pool, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database URL is empty")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	// Conservative defaults for a single-host dev setup. Tune for prod.
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 0 // default

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Pool{Pool: pool}, nil
}
