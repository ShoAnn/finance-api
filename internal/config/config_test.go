package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

// clearEnv blanks every setting the package reads, so a test starts from the
// documented defaults regardless of the ambient environment.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"PORT",
		"DATABASE_URL",
		"LOG_LEVEL",
		"SHUTDOWN_TIMEOUT",
		"STARTUP_TIMEOUT",
		"READINESS_TIMEOUT",
		"HTTP_READ_HEADER_TIMEOUT",
		"HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT",
		"DB_MAX_CONNS",
		"DB_MIN_CONNS",
		"DB_MAX_CONN_LIFETIME",
		"DB_MAX_CONN_IDLE_TIME",
		"DB_HEALTH_CHECK_PERIOD",
	} {
		t.Setenv(name, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.Port != DefaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.DatabaseURL != DefaultDatabaseURL {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, DefaultDatabaseURL)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("ShutdownTimeout = %s, want %s", cfg.ShutdownTimeout, DefaultShutdownTimeout)
	}
	if cfg.StartupTimeout != DefaultStartupTimeout {
		t.Errorf("StartupTimeout = %s, want %s", cfg.StartupTimeout, DefaultStartupTimeout)
	}
	if cfg.ReadinessTimeout != DefaultReadinessTimeout {
		t.Errorf("ReadinessTimeout = %s, want %s", cfg.ReadinessTimeout, DefaultReadinessTimeout)
	}
	if cfg.HTTP.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %s, want %s", cfg.HTTP.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	}
	if cfg.HTTP.ReadTimeout != DefaultReadTimeout {
		t.Errorf("ReadTimeout = %s, want %s", cfg.HTTP.ReadTimeout, DefaultReadTimeout)
	}
	if cfg.HTTP.WriteTimeout != DefaultWriteTimeout {
		t.Errorf("WriteTimeout = %s, want %s", cfg.HTTP.WriteTimeout, DefaultWriteTimeout)
	}
	if cfg.HTTP.IdleTimeout != DefaultIdleTimeout {
		t.Errorf("IdleTimeout = %s, want %s", cfg.HTTP.IdleTimeout, DefaultIdleTimeout)
	}
	if cfg.Database.MaxConns != 0 || cfg.Database.MinConns != 0 {
		t.Errorf("pool conns = %d/%d, want 0/0 (pgx defaults)", cfg.Database.MinConns, cfg.Database.MaxConns)
	}
	if cfg.Database.MaxConnLifetime != DefaultMaxConnLifetime {
		t.Errorf("MaxConnLifetime = %s, want %s", cfg.Database.MaxConnLifetime, DefaultMaxConnLifetime)
	}
	if cfg.Database.MaxConnIdleTime != DefaultMaxConnIdleTime {
		t.Errorf("MaxConnIdleTime = %s, want %s", cfg.Database.MaxConnIdleTime, DefaultMaxConnIdleTime)
	}
	if cfg.Database.HealthCheckPeriod != DefaultHealthCheckPeriod {
		t.Errorf("HealthCheckPeriod = %s, want %s", cfg.Database.HealthCheckPeriod, DefaultHealthCheckPeriod)
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "9000")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5433/other")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("SHUTDOWN_TIMEOUT", "30s")
	t.Setenv("STARTUP_TIMEOUT", "1s")
	t.Setenv("READINESS_TIMEOUT", "500ms")
	t.Setenv("HTTP_READ_HEADER_TIMEOUT", "1s")
	t.Setenv("HTTP_READ_TIMEOUT", "2s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "3s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "4s")
	t.Setenv("DB_MAX_CONNS", "20")
	t.Setenv("DB_MIN_CONNS", "2")
	t.Setenv("DB_MAX_CONN_LIFETIME", "2h")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "15m")
	t.Setenv("DB_HEALTH_CHECK_PERIOD", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("Port = %d, want 9000", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost:5433/other" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, slog.LevelDebug)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 30s", cfg.ShutdownTimeout)
	}
	if cfg.StartupTimeout != time.Second {
		t.Errorf("StartupTimeout = %s, want 1s", cfg.StartupTimeout)
	}
	if cfg.ReadinessTimeout != 500*time.Millisecond {
		t.Errorf("ReadinessTimeout = %s, want 500ms", cfg.ReadinessTimeout)
	}
	if cfg.HTTP.ReadHeaderTimeout != time.Second {
		t.Errorf("ReadHeaderTimeout = %s, want 1s", cfg.HTTP.ReadHeaderTimeout)
	}
	if cfg.HTTP.ReadTimeout != 2*time.Second {
		t.Errorf("ReadTimeout = %s, want 2s", cfg.HTTP.ReadTimeout)
	}
	if cfg.HTTP.WriteTimeout != 3*time.Second {
		t.Errorf("WriteTimeout = %s, want 3s", cfg.HTTP.WriteTimeout)
	}
	if cfg.HTTP.IdleTimeout != 4*time.Second {
		t.Errorf("IdleTimeout = %s, want 4s", cfg.HTTP.IdleTimeout)
	}
	if cfg.Database.MaxConns != 20 || cfg.Database.MinConns != 2 {
		t.Errorf("pool conns = %d/%d, want 2/20", cfg.Database.MinConns, cfg.Database.MaxConns)
	}
	if cfg.Database.MaxConnLifetime != 2*time.Hour {
		t.Errorf("MaxConnLifetime = %s, want 2h", cfg.Database.MaxConnLifetime)
	}
	if cfg.Database.MaxConnIdleTime != 15*time.Minute {
		t.Errorf("MaxConnIdleTime = %s, want 15m", cfg.Database.MaxConnIdleTime)
	}
	if cfg.Database.HealthCheckPeriod != 45*time.Second {
		t.Errorf("HealthCheckPeriod = %s, want 45s", cfg.Database.HealthCheckPeriod)
	}
}

func TestLoadInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantMsg string
	}{
		{name: "port not a number", key: "PORT", value: "abc", wantMsg: "PORT: must be an integer"},
		{name: "port too small", key: "PORT", value: "0", wantMsg: "PORT: must be between 1 and 65535"},
		{name: "port too large", key: "PORT", value: "70000", wantMsg: "PORT: must be between 1 and 65535"},
		{name: "duration without unit", key: "SHUTDOWN_TIMEOUT", value: "10", wantMsg: "SHUTDOWN_TIMEOUT: must be a duration"},
		{name: "negative duration", key: "SHUTDOWN_TIMEOUT", value: "-1s", wantMsg: "SHUTDOWN_TIMEOUT: must be positive"},
		{name: "negative startup timeout", key: "STARTUP_TIMEOUT", value: "-1s", wantMsg: "STARTUP_TIMEOUT: must be positive"},
		{name: "negative readiness timeout", key: "READINESS_TIMEOUT", value: "0s", wantMsg: "READINESS_TIMEOUT: must be positive"},
		{name: "invalid log level", key: "LOG_LEVEL", value: "loud", wantMsg: "LOG_LEVEL: must be a log level"},
		{name: "invalid http timeout", key: "HTTP_READ_TIMEOUT", value: "soon", wantMsg: "HTTP_READ_TIMEOUT: must be a duration"},
		{name: "negative max conns", key: "DB_MAX_CONNS", value: "-1", wantMsg: "DB_MAX_CONNS: must not be negative"},
		{name: "negative min conns", key: "DB_MIN_CONNS", value: "-1", wantMsg: "DB_MIN_CONNS: must not be negative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(tt.key, tt.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestLoadAggregatesErrors(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "abc")
	t.Setenv("LOG_LEVEL", "loud")
	t.Setenv("SHUTDOWN_TIMEOUT", "soon")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
	for _, want := range []string{"PORT", "LOG_LEVEL", "SHUTDOWN_TIMEOUT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

func TestLoadMinConnsAboveMaxConns(t *testing.T) {
	clearEnv(t)
	t.Setenv("DB_MAX_CONNS", "2")
	t.Setenv("DB_MIN_CONNS", "5")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "DB_MIN_CONNS") {
		t.Errorf("error = %q, want it to mention DB_MIN_CONNS", err.Error())
	}
}

func TestPoolConfig(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5433/other")
	t.Setenv("DB_MAX_CONNS", "20")
	t.Setenv("DB_MIN_CONNS", "2")
	t.Setenv("DB_MAX_CONN_LIFETIME", "2h")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "15m")
	t.Setenv("DB_HEALTH_CHECK_PERIOD", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	poolCfg, err := cfg.PoolConfig()
	if err != nil {
		t.Fatalf("PoolConfig() error = %v, want nil", err)
	}

	if poolCfg.MaxConns != 20 {
		t.Errorf("MaxConns = %d, want 20", poolCfg.MaxConns)
	}
	if poolCfg.MinConns != 2 {
		t.Errorf("MinConns = %d, want 2", poolCfg.MinConns)
	}
	if poolCfg.MaxConnLifetime != 2*time.Hour {
		t.Errorf("MaxConnLifetime = %s, want 2h", poolCfg.MaxConnLifetime)
	}
	if poolCfg.MaxConnIdleTime != 15*time.Minute {
		t.Errorf("MaxConnIdleTime = %s, want 15m", poolCfg.MaxConnIdleTime)
	}
	if poolCfg.HealthCheckPeriod != 45*time.Second {
		t.Errorf("HealthCheckPeriod = %s, want 45s", poolCfg.HealthCheckPeriod)
	}
	if poolCfg.ConnConfig.Host != "localhost" || poolCfg.ConnConfig.Port != 5433 {
		t.Errorf("host:port = %s:%d, want localhost:5433", poolCfg.ConnConfig.Host, poolCfg.ConnConfig.Port)
	}
}

func TestPoolConfigKeepsDefaultsWhenUnset(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	poolCfg, err := cfg.PoolConfig()
	if err != nil {
		t.Fatalf("PoolConfig() error = %v, want nil", err)
	}

	if poolCfg.MaxConns <= 0 {
		t.Errorf("MaxConns = %d, want the pgx default", poolCfg.MaxConns)
	}
	if poolCfg.MinConns != 0 {
		t.Errorf("MinConns = %d, want 0", poolCfg.MinConns)
	}
}

func TestPoolConfigInvalidURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "://not a url")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if _, err := cfg.PoolConfig(); err == nil {
		t.Fatal("PoolConfig() error = nil, want an error")
	}
}
