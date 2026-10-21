// Package main runs the personal finance API.
//
// The whole service lives in this package: configuration in this file, the
// HTTP handlers in handlers.go, and database access in store.go.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Defaults for every setting.
const (
	DefaultPort              = 8080
	DefaultDatabaseURL       = "postgres://finance:finance@localhost:5432/finance?sslmode=disable"
	DefaultLogLevel          = slog.LevelInfo
	DefaultShutdownTimeout   = 10 * time.Second
	DefaultStartupTimeout    = 5 * time.Second
	DefaultReadinessTimeout  = 2 * time.Second
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultReadTimeout       = 10 * time.Second
	DefaultWriteTimeout      = 15 * time.Second
	DefaultIdleTimeout       = 60 * time.Second
	DefaultMaxConnLifetime   = time.Hour
	DefaultMaxConnIdleTime   = 30 * time.Minute
	DefaultHealthCheckPeriod = time.Minute
)

// Config is the complete runtime configuration.
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port int
	// DatabaseURL is the Postgres connection string.
	DatabaseURL string
	// LogLevel is the minimum level that is logged.
	LogLevel slog.Level

	// ShutdownTimeout bounds the graceful drain after SIGTERM.
	ShutdownTimeout time.Duration
	// StartupTimeout bounds the database ping at startup.
	StartupTimeout time.Duration
	// ReadinessTimeout bounds the database ping behind GET /readyz.
	ReadinessTimeout time.Duration

	// HTTP holds the HTTP server timeouts.
	HTTP HTTPConfig
	// Database holds the connection pool settings.
	Database DatabaseConfig
}

// HTTPConfig holds the HTTP server timeouts.
type HTTPConfig struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// DatabaseConfig holds the connection pool settings. A zero value means the
// pgx default is kept.
type DatabaseConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// Load reads the configuration from the environment. It returns every problem
// it finds, joined into a single error.
func Load() (Config, error) {
	cfg := Config{
		Port:             DefaultPort,
		DatabaseURL:      DefaultDatabaseURL,
		LogLevel:         DefaultLogLevel,
		ShutdownTimeout:  DefaultShutdownTimeout,
		StartupTimeout:   DefaultStartupTimeout,
		ReadinessTimeout: DefaultReadinessTimeout,
		HTTP: HTTPConfig{
			ReadHeaderTimeout: DefaultReadHeaderTimeout,
			ReadTimeout:       DefaultReadTimeout,
			WriteTimeout:      DefaultWriteTimeout,
			IdleTimeout:       DefaultIdleTimeout,
		},
		Database: DatabaseConfig{
			MaxConnLifetime:   DefaultMaxConnLifetime,
			MaxConnIdleTime:   DefaultMaxConnIdleTime,
			HealthCheckPeriod: DefaultHealthCheckPeriod,
		},
	}

	var errs []error

	port, err := intEnv("PORT", DefaultPort)
	switch {
	case err != nil:
		errs = append(errs, err)
	case port < 1 || port > 65535:
		errs = append(errs, fmt.Errorf("PORT: must be between 1 and 65535, got %d", port))
	default:
		cfg.Port = port
	}

	if url := env("DATABASE_URL"); url != "" {
		cfg.DatabaseURL = url
	}

	level, err := levelEnv("LOG_LEVEL", DefaultLogLevel)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.LogLevel = level
	}

	if d, err := durationEnv("SHUTDOWN_TIMEOUT", DefaultShutdownTimeout); err != nil {
		errs = append(errs, err)
	} else if d <= 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: must be positive, got %s", d))
	} else {
		cfg.ShutdownTimeout = d
	}

	if d, err := durationEnv("STARTUP_TIMEOUT", DefaultStartupTimeout); err != nil {
		errs = append(errs, err)
	} else if d <= 0 {
		errs = append(errs, fmt.Errorf("STARTUP_TIMEOUT: must be positive, got %s", d))
	} else {
		cfg.StartupTimeout = d
	}

	if d, err := durationEnv("READINESS_TIMEOUT", DefaultReadinessTimeout); err != nil {
		errs = append(errs, err)
	} else if d <= 0 {
		errs = append(errs, fmt.Errorf("READINESS_TIMEOUT: must be positive, got %s", d))
	} else {
		cfg.ReadinessTimeout = d
	}

	errs = durationSetting(errs, "HTTP_READ_HEADER_TIMEOUT", DefaultReadHeaderTimeout, &cfg.HTTP.ReadHeaderTimeout)
	errs = durationSetting(errs, "HTTP_READ_TIMEOUT", DefaultReadTimeout, &cfg.HTTP.ReadTimeout)
	errs = durationSetting(errs, "HTTP_WRITE_TIMEOUT", DefaultWriteTimeout, &cfg.HTTP.WriteTimeout)
	errs = durationSetting(errs, "HTTP_IDLE_TIMEOUT", DefaultIdleTimeout, &cfg.HTTP.IdleTimeout)

	maxConns, err := intEnv("DB_MAX_CONNS", 0)
	if err != nil {
		errs = append(errs, err)
	} else if maxConns < 0 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS: must not be negative, got %d", maxConns))
	} else {
		cfg.Database.MaxConns = int32(maxConns)
	}

	minConns, err := intEnv("DB_MIN_CONNS", 0)
	if err != nil {
		errs = append(errs, err)
	} else if minConns < 0 {
		errs = append(errs, fmt.Errorf("DB_MIN_CONNS: must not be negative, got %d", minConns))
	} else {
		cfg.Database.MinConns = int32(minConns)
	}

	if cfg.Database.MaxConns > 0 && cfg.Database.MinConns > cfg.Database.MaxConns {
		errs = append(errs, fmt.Errorf("DB_MIN_CONNS: must not be greater than DB_MAX_CONNS (%d), got %d",
			cfg.Database.MaxConns, cfg.Database.MinConns))
	}

	errs = durationSetting(errs, "DB_MAX_CONN_LIFETIME", DefaultMaxConnLifetime, &cfg.Database.MaxConnLifetime)
	errs = durationSetting(errs, "DB_MAX_CONN_IDLE_TIME", DefaultMaxConnIdleTime, &cfg.Database.MaxConnIdleTime)
	errs = durationSetting(errs, "DB_HEALTH_CHECK_PERIOD", DefaultHealthCheckPeriod, &cfg.Database.HealthCheckPeriod)

	return cfg, errors.Join(errs...)
}

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

// durationSetting resolves a duration setting into dst and collects any
// problem it finds.
func durationSetting(errs []error, name string, def time.Duration, dst *time.Duration) []error {
	d, err := durationEnv(name, def)
	if err != nil {
		return append(errs, err)
	}
	if d <= 0 {
		return append(errs, fmt.Errorf("%s: must be positive, got %s", name, d))
	}
	*dst = d
	return errs
}

// env returns the trimmed value of an environment variable.
func env(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

// intEnv reads an integer setting, falling back to def when unset.
func intEnv(name string, def int) (int, error) {
	raw := env(name)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def, fmt.Errorf("%s: must be an integer, got %q", name, raw)
	}
	return v, nil
}

// durationEnv reads a duration setting, falling back to def when unset.
func durationEnv(name string, def time.Duration) (time.Duration, error) {
	raw := env(name)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return def, fmt.Errorf("%s: must be a duration such as 10s, got %q", name, raw)
	}
	return d, nil
}

// levelEnv reads a log level setting, falling back to def when unset.
func levelEnv(name string, def slog.Level) (slog.Level, error) {
	raw := env(name)
	if raw == "" {
		return def, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(raw)); err != nil {
		return def, fmt.Errorf("%s: must be a log level such as info, got %q", name, raw)
	}
	return level, nil
}

func main() {
	cfg, err := Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))

	if err := run(cfg); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run(cfg Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	poolCfg, err := cfg.PoolConfig()
	if err != nil {
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("configure database pool: %w", err)
	}
	store := NewStore(pool)
	defer store.Close()

	pingCtx, cancelPing := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancelPing()
	if err := store.Ping(pingCtx); err != nil {
		// Not fatal: /readyz reports the outage and the pool reconnects on its own.
		slog.Warn("database not reachable at startup", "error", err)
	}

	api := &API{store: store, readinessTimeout: cfg.ReadinessTimeout}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           api.routes(),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "log_level", cfg.LogLevel.String())
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return nil
}
