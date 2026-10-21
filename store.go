package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ShoAnn/finance-api/db/sqlc"
)

// Store is the data-access layer. It embeds the generated queries so handlers
// can call them directly, and keeps the pool for health checks and shutdown.
type Store struct {
	*sqlc.Queries

	pool *pgxpool.Pool
}

// NewStore returns a Store backed by pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		Queries: sqlc.New(pool),
		pool:    pool,
	}
}

// Ping reports whether the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Close releases all pooled connections.
func (s *Store) Close() {
	s.pool.Close()
}
