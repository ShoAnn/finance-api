package config

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig builds the pgx connection pool configuration from the database
// settings. Settings left at zero keep the pgx default.
func (c Config) PoolConfig() (*pgxpool.Config, error) {
	poolCfg, err := pgxpool.ParseConfig(c.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL: %w", err)
	}

	if c.Database.MaxConns > 0 {
		poolCfg.MaxConns = c.Database.MaxConns
	}
	if c.Database.MinConns > 0 {
		poolCfg.MinConns = c.Database.MinConns
	}
	if c.Database.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = c.Database.MaxConnLifetime
	}
	if c.Database.MaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = c.Database.MaxConnIdleTime
	}
	if c.Database.HealthCheckPeriod > 0 {
		poolCfg.HealthCheckPeriod = c.Database.HealthCheckPeriod
	}

	return poolCfg, nil
}
