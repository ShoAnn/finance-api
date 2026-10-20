// Package config loads and validates the API's runtime configuration from the
// environment.
//
// Every setting has a default that matches the local development setup, so the
// API runs with no environment at all. Invalid values are reported together
// rather than one at a time, so a misconfigured deployment can be fixed in a
// single pass.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
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
