package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ShoAnn/finance-api/db/sqlc"
)

func TestMain(m *testing.M) {
	// Keep expected 5xx logging out of the test output.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

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

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
		} `json:"details"`
	} `json:"error"`
}

func writeAPIError(t *testing.T, err error) (*httptest.ResponseRecorder, errorBody) {
	t.Helper()
	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/accounts/1", nil), err)

	var body errorBody
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body %q: %v", rec.Body.String(), err)
		}
	}
	return rec, body
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

func TestWriteErrorNotFound(t *testing.T) {
	rec, body := writeAPIError(t, NotFound("account not found"))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if body.Error.Code != string(CodeNotFound) {
		t.Errorf("code = %q, want %q", body.Error.Code, CodeNotFound)
	}
	if body.Error.Message != "account not found" {
		t.Errorf("message = %q, want %q", body.Error.Message, "account not found")
	}
	if body.Error.Details != nil {
		t.Errorf("details = %v, want omitted", body.Error.Details)
	}
}

func TestWriteErrorWrapped(t *testing.T) {
	wrapped := fmt.Errorf("load account: %w", NotFound("account not found"))

	rec, body := writeAPIError(t, wrapped)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if body.Error.Message != "account not found" {
		t.Errorf("message = %q, want %q", body.Error.Message, "account not found")
	}
}

func TestWriteErrorUnknownBecomesInternal(t *testing.T) {
	rec, body := writeAPIError(t, stderrors.New("connection refused"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if body.Error.Code != string(CodeInternal) {
		t.Errorf("code = %q, want %q", body.Error.Code, CodeInternal)
	}
	if body.Error.Message != "internal server error" {
		t.Errorf("message = %q, want %q", body.Error.Message, "internal server error")
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Errorf("body leaks the cause: %s", rec.Body.String())
	}
}

func TestWriteErrorValidationIncludesDetails(t *testing.T) {
	rec, body := writeAPIError(t, Validation(Detail{Field: "amount_cents", Message: "expected an integer"}))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body.Error.Code != string(CodeValidation) {
		t.Errorf("code = %q, want %q", body.Error.Code, CodeValidation)
	}
	if len(body.Error.Details) != 1 {
		t.Fatalf("details = %v, want 1 entry", body.Error.Details)
	}
	if body.Error.Details[0].Field != "amount_cents" {
		t.Errorf("detail field = %q, want %q", body.Error.Details[0].Field, "amount_cents")
	}
}

func TestWriteErrorNilWritesNothing(t *testing.T) {
	rec, _ := writeAPIError(t, nil)
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
}

func TestAPIErrorUnwrap(t *testing.T) {
	cause := stderrors.New("boom")
	err := Internal(cause)

	if !stderrors.Is(err, cause) {
		t.Error("errors.Is could not find the cause")
	}
	if got := err.Error(); got != "internal_error: internal server error" {
		t.Errorf("Error() = %q, want %q", got, "internal_error: internal server error")
	}
}

func TestAsNil(t *testing.T) {
	if got := As(nil); got != nil {
		t.Errorf("As(nil) = %v, want nil", got)
	}
}

func TestFromDB(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    Code
		wantMessage string
		wantField   string
	}{
		{
			name:        "no rows",
			err:         pgx.ErrNoRows,
			wantStatus:  http.StatusNotFound,
			wantCode:    CodeNotFound,
			wantMessage: "resource not found",
		},
		{
			name:        "unique violation with known constraint",
			err:         &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: "categories_name_key"},
			wantStatus:  http.StatusConflict,
			wantCode:    CodeConflict,
			wantMessage: "category name already exists",
		},
		{
			name:        "unique violation with unknown constraint",
			err:         &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: "accounts_pkey"},
			wantStatus:  http.StatusConflict,
			wantCode:    CodeConflict,
			wantMessage: "resource already exists",
		},
		{
			name:       "check violation",
			err:        &pgconn.PgError{Code: pgCheckViolation, ConstraintName: "accounts_type_check"},
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeValidation,
			wantField:  "type",
		},
		{
			name:        "invalid text representation",
			err:         &pgconn.PgError{Code: pgInvalidTextRepresentation},
			wantStatus:  http.StatusBadRequest,
			wantCode:    CodeInvalidRequest,
			wantMessage: "malformed value",
		},
		{
			name:        "foreign key violation",
			err:         &pgconn.PgError{Code: pgForeignKeyViolation},
			wantStatus:  http.StatusConflict,
			wantCode:    CodeConflict,
			wantMessage: "resource is referenced by other records",
		},
		{
			name:       "unknown postgres error",
			err:        &pgconn.PgError{Code: "53300"},
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
		},
		{
			name:       "plain error",
			err:        stderrors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromDB(tt.err)
			if got.Status != tt.wantStatus {
				t.Errorf("status = %d, want %d", got.Status, tt.wantStatus)
			}
			if got.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", got.Code, tt.wantCode)
			}
			if tt.wantMessage != "" && got.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMessage)
			}
			if tt.wantField != "" {
				if len(got.Details) != 1 {
					t.Fatalf("details = %v, want 1 entry", got.Details)
				}
				if got.Details[0].Field != tt.wantField {
					t.Errorf("detail field = %q, want %q", got.Details[0].Field, tt.wantField)
				}
			}
		})
	}
}

func TestFromDBNil(t *testing.T) {
	if got := FromDB(nil); got != nil {
		t.Errorf("FromDB(nil) = %v, want nil", got)
	}
}

// ---- test harness ----

type fakeQuerier struct {
	archiveAccount    func(context.Context, int64) error
	createAPIKey      func(context.Context, sqlc.CreateAPIKeyParams) (sqlc.ApiKey, error)
	createAccount     func(context.Context, sqlc.CreateAccountParams) (sqlc.Account, error)
	createCategory    func(context.Context, sqlc.CreateCategoryParams) (sqlc.Category, error)
	createTransaction func(context.Context, sqlc.CreateTransactionParams) (sqlc.Transaction, error)
	deleteAccount     func(context.Context, int64) (int64, error)
	deleteCategory    func(context.Context, int64) (int64, error)
	deleteTransaction func(context.Context, int64) (int64, error)
	getAPIKeyByHash   func(context.Context, string) (sqlc.ApiKey, error)
	getAccount        func(context.Context, int64) (sqlc.Account, error)
	getAccountBalance func(context.Context, int64) (int64, error)
	getTransaction    func(context.Context, int64) (sqlc.Transaction, error)
	listAccounts      func(context.Context) ([]sqlc.Account, error)
	listCategories    func(context.Context) ([]sqlc.Category, error)
	listTransactions  func(context.Context, sqlc.ListTransactionsParams) ([]sqlc.Transaction, error)
	touchAPIKey       func(context.Context, int64) error
	updateCategory    func(context.Context, sqlc.UpdateCategoryParams) (sqlc.Category, error)
	updateTransaction func(context.Context, sqlc.UpdateTransactionParams) (sqlc.Transaction, error)
}

func (f *fakeQuerier) ArchiveAccount(ctx context.Context, id int64) error {
	return f.archiveAccount(ctx, id)
}

func (f *fakeQuerier) CreateAPIKey(ctx context.Context, arg sqlc.CreateAPIKeyParams) (sqlc.ApiKey, error) {
	return f.createAPIKey(ctx, arg)
}

func (f *fakeQuerier) CreateAccount(ctx context.Context, arg sqlc.CreateAccountParams) (sqlc.Account, error) {
	return f.createAccount(ctx, arg)
}

func (f *fakeQuerier) CreateCategory(ctx context.Context, arg sqlc.CreateCategoryParams) (sqlc.Category, error) {
	return f.createCategory(ctx, arg)
}

func (f *fakeQuerier) CreateTransaction(ctx context.Context, arg sqlc.CreateTransactionParams) (sqlc.Transaction, error) {
	return f.createTransaction(ctx, arg)
}

func (f *fakeQuerier) DeleteAccount(ctx context.Context, id int64) (int64, error) {
	return f.deleteAccount(ctx, id)
}

func (f *fakeQuerier) DeleteCategory(ctx context.Context, id int64) (int64, error) {
	return f.deleteCategory(ctx, id)
}

func (f *fakeQuerier) DeleteTransaction(ctx context.Context, id int64) (int64, error) {
	return f.deleteTransaction(ctx, id)
}

func (f *fakeQuerier) GetAccount(ctx context.Context, id int64) (sqlc.Account, error) {
	return f.getAccount(ctx, id)
}

func (f *fakeQuerier) GetAPIKeyByHash(ctx context.Context, keyHash string) (sqlc.ApiKey, error) {
	return f.getAPIKeyByHash(ctx, keyHash)
}

func (f *fakeQuerier) GetAccountBalance(ctx context.Context, accountID int64) (int64, error) {
	return f.getAccountBalance(ctx, accountID)
}

func (f *fakeQuerier) GetTransaction(ctx context.Context, id int64) (sqlc.Transaction, error) {
	return f.getTransaction(ctx, id)
}

func (f *fakeQuerier) ListAccounts(ctx context.Context) ([]sqlc.Account, error) {
	return f.listAccounts(ctx)
}

func (f *fakeQuerier) ListCategories(ctx context.Context) ([]sqlc.Category, error) {
	return f.listCategories(ctx)
}

func (f *fakeQuerier) ListTransactions(ctx context.Context, arg sqlc.ListTransactionsParams) ([]sqlc.Transaction, error) {
	return f.listTransactions(ctx, arg)
}

func (f *fakeQuerier) UpdateCategory(ctx context.Context, arg sqlc.UpdateCategoryParams) (sqlc.Category, error) {
	return f.updateCategory(ctx, arg)
}

func (f *fakeQuerier) TouchAPIKey(ctx context.Context, id int64) error {
	return f.touchAPIKey(ctx, id)
}

func (f *fakeQuerier) UpdateTransaction(ctx context.Context, arg sqlc.UpdateTransactionParams) (sqlc.Transaction, error) {
	return f.updateTransaction(ctx, arg)
}

var fixedTime = time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)

func fixedTimestamp() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: fixedTime, Valid: true}
}

func fixedDate(value string) pgtype.Date {
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		panic(err)
	}
	return pgtype.Date{Time: parsed, Valid: true}
}

func sampleAccount() sqlc.Account {
	return sqlc.Account{
		ID:        42,
		Name:      "Everyday Checking",
		Type:      "checking",
		Currency:  "USD",
		CreatedAt: fixedTimestamp(),
	}
}

func sampleCategory() sqlc.Category {
	return sqlc.Category{
		ID:        8,
		Name:      "Groceries",
		Kind:      "expense",
		CreatedAt: fixedTimestamp(),
	}
}

func sampleTransaction() sqlc.Transaction {
	categoryID := int64(8)
	description := "Weekly grocery run"
	return sqlc.Transaction{
		ID:          7,
		AccountID:   42,
		CategoryID:  &categoryID,
		AmountCents: -4599,
		Description: &description,
		OccurredOn:  fixedDate("2026-01-20"),
		CreatedAt:   fixedTimestamp(),
		UpdatedAt:   fixedTimestamp(),
	}
}

func newTestAPI(q sqlc.Querier) http.Handler {
	return newTestAPIWithAuth(q, func(context.Context, string) error { return nil })
}

func newTestAPIWithAuth(q sqlc.Querier, authenticate func(context.Context, string) error) http.Handler {
	return (&API{
		store:            &Store{Querier: q},
		readinessTimeout: time.Second,
		authenticate:     authenticate,
	}).routes()
}

func do(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Authorization", "Bearer test-key")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeResponse[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return value
}

func checkStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, want, rec.Body.String())
	}
}

func checkError(t *testing.T, rec *httptest.ResponseRecorder, status int, code Code) errorBody {
	t.Helper()
	checkStatus(t, rec, status)
	body := decodeResponse[errorBody](t, rec)
	if body.Error.Code != string(code) {
		t.Errorf("code = %q, want %q", body.Error.Code, code)
	}
	return body
}

func hasDetail(body errorBody, field string) bool {
	for _, detail := range body.Error.Details {
		if detail.Field == field {
			return true
		}
	}
	return false
}

// ---- routing ----

func TestJSON404And405(t *testing.T) {
	api := newTestAPI(&fakeQuerier{})

	rec := do(t, api, http.MethodGet, "/missing", "")
	body := checkError(t, rec, http.StatusNotFound, CodeNotFound)
	if body.Error.Message != "resource not found" {
		t.Errorf("message = %q", body.Error.Message)
	}

	rec = do(t, api, http.MethodPost, "/accounts/42", "")
	checkError(t, rec, http.StatusMethodNotAllowed, CodeMethodNotAllowed)
	if got := rec.Header().Get("Allow"); got != "GET, DELETE" {
		t.Errorf("Allow = %q, want %q", got, "GET, DELETE")
	}

	rec = do(t, api, http.MethodDelete, "/healthz", "")
	checkError(t, rec, http.StatusMethodNotAllowed, CodeMethodNotAllowed)
}

func TestHealthRoute(t *testing.T) {
	rec := do(t, newTestAPI(&fakeQuerier{}), http.MethodGet, "/healthz", "")
	checkStatus(t, rec, http.StatusOK)
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
}

// ---- authentication ----

func TestAPIKeyRequired(t *testing.T) {
	var seenToken string
	fake := &fakeQuerier{
		listAccounts: func(context.Context) ([]sqlc.Account, error) { return nil, nil },
	}
	api := newTestAPIWithAuth(fake, func(_ context.Context, token string) error {
		seenToken = token
		if token != "good-key" {
			return Unauthorized("invalid API key")
		}
		return nil
	})

	serve := func(path, header string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}

	// Health probes stay public.
	checkStatus(t, serve("/healthz", ""), http.StatusOK)

	rec := serve("/accounts", "")
	checkError(t, rec, http.StatusUnauthorized, CodeUnauthorized)
	if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Bearer") {
		t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", got)
	}

	checkError(t, serve("/accounts", "Basic good-key"), http.StatusUnauthorized, CodeUnauthorized)
	checkError(t, serve("/accounts", "Bearer wrong-key"), http.StatusUnauthorized, CodeUnauthorized)

	checkStatus(t, serve("/accounts", "Bearer good-key"), http.StatusOK)
	if seenToken != "good-key" {
		t.Errorf("authenticator saw token %q, want %q", seenToken, "good-key")
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc123", "abc123", true},
		{"bearer abc123", "abc123", true},
		{"Bearer\tabc123", "abc123", true},
		{"Basic abc123", "", false},
		{"Bearer", "", false},
		{"Bearer   ", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := bearerToken(tt.header)
		if got != tt.want || ok != tt.ok {
			t.Errorf("bearerToken(%q) = (%q, %t), want (%q, %t)", tt.header, got, ok, tt.want, tt.ok)
		}
	}
}

// ---- accounts ----

func TestListAccounts(t *testing.T) {
	archived := sampleAccount()
	archived.ID = 43
	archived.Name = "Old Savings"
	archived.ArchivedAt = fixedTimestamp()

	fake := &fakeQuerier{
		listAccounts: func(context.Context) ([]sqlc.Account, error) {
			return []sqlc.Account{sampleAccount(), archived}, nil
		},
	}

	rec := do(t, newTestAPI(fake), http.MethodGet, "/accounts", "")
	checkStatus(t, rec, http.StatusOK)

	accounts := decodeResponse[[]accountResponse](t, rec)
	if len(accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(accounts))
	}
	if accounts[0].CreatedAt != "2026-01-15T09:30:00Z" {
		t.Errorf("created_at = %q", accounts[0].CreatedAt)
	}
	if accounts[0].ArchivedAt != nil {
		t.Errorf("archived_at = %v, want null", accounts[0].ArchivedAt)
	}
	if accounts[1].ArchivedAt == nil || *accounts[1].ArchivedAt != "2026-01-15T09:30:00Z" {
		t.Errorf("archived_at = %v", accounts[1].ArchivedAt)
	}
	if !strings.Contains(rec.Body.String(), `"archived_at":null`) {
		t.Errorf("body does not encode a null archived_at: %s", rec.Body.String())
	}
}

func TestCreateAccount(t *testing.T) {
	var got sqlc.CreateAccountParams
	fake := &fakeQuerier{
		createAccount: func(_ context.Context, arg sqlc.CreateAccountParams) (sqlc.Account, error) {
			got = arg
			account := sampleAccount()
			account.Name = arg.Name
			account.Type = arg.Type
			account.Currency = arg.Currency
			return account, nil
		},
	}

	rec := do(t, newTestAPI(fake), http.MethodPost, "/accounts", `{"name":"  Wallet  ","type":"cash"}`)
	checkStatus(t, rec, http.StatusCreated)
	if got := rec.Header().Get("Location"); got != "/accounts/42" {
		t.Errorf("Location = %q", got)
	}
	if got.Name != "Wallet" || got.Type != "cash" || got.Currency != "USD" {
		t.Errorf("params = %+v", got)
	}
	body := decodeResponse[accountResponse](t, rec)
	if body.ID != 42 || body.Currency != "USD" {
		t.Errorf("body = %+v", body)
	}
}

func TestCreateAccountValidation(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		field string
	}{
		{name: "missing name", body: `{"type":"cash"}`, field: "name"},
		{name: "missing type", body: `{"name":"Wallet"}`, field: "type"},
		{name: "unknown type", body: `{"name":"Wallet","type":"crypto"}`, field: "type"},
		{name: "bad currency", body: `{"name":"Wallet","type":"cash","currency":"us"}`, field: "currency"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newTestAPI(&fakeQuerier{}), http.MethodPost, "/accounts", tt.body)
			body := checkError(t, rec, http.StatusBadRequest, CodeValidation)
			if !hasDetail(body, tt.field) {
				t.Errorf("details = %+v, want field %q", body.Error.Details, tt.field)
			}
		})
	}
}

func TestStrictJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"name":"Wallet","type":"cash","extra":true}`},
		{name: "trailing data", body: `{"name":"Wallet","type":"cash"} {}`},
		{name: "malformed", body: `{`},
		{name: "empty", body: ``},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newTestAPI(&fakeQuerier{}), http.MethodPost, "/accounts", tt.body)
			checkError(t, rec, http.StatusBadRequest, CodeInvalidRequest)
		})
	}
}

func TestGetAccount(t *testing.T) {
	fake := &fakeQuerier{
		getAccount: func(_ context.Context, id int64) (sqlc.Account, error) {
			if id != 42 {
				t.Errorf("id = %d", id)
			}
			return sampleAccount(), nil
		},
	}
	rec := do(t, newTestAPI(fake), http.MethodGet, "/accounts/42", "")
	checkStatus(t, rec, http.StatusOK)
	if body := decodeResponse[accountResponse](t, rec); body.Name != "Everyday Checking" {
		t.Errorf("body = %+v", body)
	}

	missing := &fakeQuerier{
		getAccount: func(context.Context, int64) (sqlc.Account, error) { return sqlc.Account{}, pgx.ErrNoRows },
	}
	body := checkError(t, do(t, newTestAPI(missing), http.MethodGet, "/accounts/99", ""), http.StatusNotFound, CodeNotFound)
	if body.Error.Message != "account not found" {
		t.Errorf("message = %q", body.Error.Message)
	}

	rec = do(t, newTestAPI(&fakeQuerier{}), http.MethodGet, "/accounts/abc", "")
	checkError(t, rec, http.StatusBadRequest, CodeValidation)
}

func TestArchiveAccount(t *testing.T) {
	archived := sampleAccount()
	archived.ArchivedAt = fixedTimestamp()
	var archivedID int64

	fake := &fakeQuerier{
		archiveAccount: func(_ context.Context, id int64) error {
			archivedID = id
			return nil
		},
		getAccount: func(context.Context, int64) (sqlc.Account, error) { return archived, nil },
	}

	rec := do(t, newTestAPI(fake), http.MethodPost, "/accounts/42/archive", "")
	checkStatus(t, rec, http.StatusOK)
	if archivedID != 42 {
		t.Errorf("archived id = %d", archivedID)
	}
	if body := decodeResponse[accountResponse](t, rec); body.ArchivedAt == nil {
		t.Error("archived_at = nil, want a timestamp")
	}
}

func TestAccountBalance(t *testing.T) {
	fake := &fakeQuerier{
		getAccount:        func(context.Context, int64) (sqlc.Account, error) { return sampleAccount(), nil },
		getAccountBalance: func(context.Context, int64) (int64, error) { return 152340, nil },
	}
	rec := do(t, newTestAPI(fake), http.MethodGet, "/accounts/42/balance", "")
	checkStatus(t, rec, http.StatusOK)
	body := decodeResponse[balanceResponse](t, rec)
	if body.AccountID != 42 || body.BalanceCents != 152340 {
		t.Errorf("body = %+v", body)
	}

	missing := &fakeQuerier{
		getAccount: func(context.Context, int64) (sqlc.Account, error) { return sqlc.Account{}, pgx.ErrNoRows },
	}
	checkError(t, do(t, newTestAPI(missing), http.MethodGet, "/accounts/99/balance", ""), http.StatusNotFound, CodeNotFound)
}

func TestDeleteAccount(t *testing.T) {
	tests := []struct {
		name   string
		result int64
		err    error
		status int
		code   Code
	}{
		{name: "deleted", result: 1, status: http.StatusNoContent},
		{name: "missing", result: 0, status: http.StatusNotFound, code: CodeNotFound},
		{name: "referenced", err: &pgconn.PgError{Code: pgForeignKeyViolation}, status: http.StatusConflict, code: CodeConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeQuerier{
				deleteAccount: func(context.Context, int64) (int64, error) { return tt.result, tt.err },
			}
			rec := do(t, newTestAPI(fake), http.MethodDelete, "/accounts/42", "")
			if tt.code == "" {
				checkStatus(t, rec, tt.status)
				return
			}
			body := checkError(t, rec, tt.status, tt.code)
			if tt.code == CodeConflict && body.Error.Message != "account has transactions and cannot be deleted" {
				t.Errorf("message = %q", body.Error.Message)
			}
		})
	}
}

// ---- categories ----

func TestListCategories(t *testing.T) {
	fake := &fakeQuerier{
		listCategories: func(context.Context) ([]sqlc.Category, error) {
			return []sqlc.Category{sampleCategory()}, nil
		},
	}
	rec := do(t, newTestAPI(fake), http.MethodGet, "/categories", "")
	checkStatus(t, rec, http.StatusOK)
	categories := decodeResponse[[]categoryResponse](t, rec)
	if len(categories) != 1 || categories[0].Kind != "expense" {
		t.Errorf("categories = %+v", categories)
	}
}

func TestCreateCategory(t *testing.T) {
	var got sqlc.CreateCategoryParams
	fake := &fakeQuerier{
		createCategory: func(_ context.Context, arg sqlc.CreateCategoryParams) (sqlc.Category, error) {
			got = arg
			return sampleCategory(), nil
		},
	}
	rec := do(t, newTestAPI(fake), http.MethodPost, "/categories", `{"name":" Groceries ","kind":"expense"}`)
	checkStatus(t, rec, http.StatusCreated)
	if rec.Header().Get("Location") != "/categories/8" {
		t.Errorf("Location = %q", rec.Header().Get("Location"))
	}
	if got.Name != "Groceries" || got.Kind != "expense" {
		t.Errorf("params = %+v", got)
	}

	rec = do(t, newTestAPI(&fakeQuerier{}), http.MethodPost, "/categories", `{"name":"Groceries","kind":"other"}`)
	body := checkError(t, rec, http.StatusBadRequest, CodeValidation)
	if !hasDetail(body, "kind") {
		t.Errorf("details = %+v", body.Error.Details)
	}
}

func TestCreateCategoryConflict(t *testing.T) {
	fake := &fakeQuerier{
		createCategory: func(context.Context, sqlc.CreateCategoryParams) (sqlc.Category, error) {
			return sqlc.Category{}, &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: "categories_name_key"}
		},
	}
	rec := do(t, newTestAPI(fake), http.MethodPost, "/categories", `{"name":"Groceries","kind":"expense"}`)
	body := checkError(t, rec, http.StatusConflict, CodeConflict)
	if body.Error.Message != "category name already exists" {
		t.Errorf("message = %q", body.Error.Message)
	}
}

func TestUpdateCategory(t *testing.T) {
	var got sqlc.UpdateCategoryParams
	fake := &fakeQuerier{
		updateCategory: func(_ context.Context, arg sqlc.UpdateCategoryParams) (sqlc.Category, error) {
			got = arg
			return sampleCategory(), nil
		},
	}
	rec := do(t, newTestAPI(fake), http.MethodPut, "/categories/8", `{"name":"Groceries","kind":"expense"}`)
	checkStatus(t, rec, http.StatusOK)
	if got.ID != 8 || got.Name != "Groceries" {
		t.Errorf("params = %+v", got)
	}

	missing := &fakeQuerier{
		updateCategory: func(context.Context, sqlc.UpdateCategoryParams) (sqlc.Category, error) {
			return sqlc.Category{}, pgx.ErrNoRows
		},
	}
	checkError(t, do(t, newTestAPI(missing), http.MethodPut, "/categories/99", `{"name":"Groceries","kind":"expense"}`),
		http.StatusNotFound, CodeNotFound)
}

func TestDeleteCategory(t *testing.T) {
	fake := &fakeQuerier{
		deleteCategory: func(context.Context, int64) (int64, error) { return 1, nil },
	}
	checkStatus(t, do(t, newTestAPI(fake), http.MethodDelete, "/categories/8", ""), http.StatusNoContent)

	missing := &fakeQuerier{
		deleteCategory: func(context.Context, int64) (int64, error) { return 0, nil },
	}
	checkError(t, do(t, newTestAPI(missing), http.MethodDelete, "/categories/99", ""), http.StatusNotFound, CodeNotFound)
}

// ---- transactions ----

func TestListTransactions(t *testing.T) {
	var got sqlc.ListTransactionsParams
	fake := &fakeQuerier{
		listTransactions: func(_ context.Context, arg sqlc.ListTransactionsParams) ([]sqlc.Transaction, error) {
			got = arg
			return []sqlc.Transaction{sampleTransaction()}, nil
		},
	}

	rec := do(t, newTestAPI(fake), http.MethodGet, "/transactions?account_id=42&from=2026-01-01&to=2026-01-31", "")
	checkStatus(t, rec, http.StatusOK)
	if got.AccountID != 42 {
		t.Errorf("account_id = %d", got.AccountID)
	}
	if got.FromDate.Time.Format(dateLayout) != "2026-01-01" || got.ToDate.Time.Format(dateLayout) != "2026-01-31" {
		t.Errorf("range = %s to %s", got.FromDate.Time, got.ToDate.Time)
	}
	transactions := decodeResponse[[]transactionResponse](t, rec)
	if len(transactions) != 1 || transactions[0].OccurredOn != "2026-01-20" {
		t.Errorf("transactions = %+v", transactions)
	}
}

func TestListTransactionsValidation(t *testing.T) {
	tests := []struct {
		name   string
		target string
		fields []string
	}{
		{name: "missing all", target: "/transactions", fields: []string{"account_id", "from", "to"}},
		{name: "bad account", target: "/transactions?account_id=abc&from=2026-01-01&to=2026-01-31", fields: []string{"account_id"}},
		{name: "bad date", target: "/transactions?account_id=42&from=01-01-2026&to=2026-01-31", fields: []string{"from"}},
		{name: "reversed range", target: "/transactions?account_id=42&from=2026-02-01&to=2026-01-01", fields: []string{"from"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newTestAPI(&fakeQuerier{}), http.MethodGet, tt.target, "")
			body := checkError(t, rec, http.StatusBadRequest, CodeValidation)
			for _, field := range tt.fields {
				if !hasDetail(body, field) {
					t.Errorf("details = %+v, want field %q", body.Error.Details, field)
				}
			}
		})
	}
}

func TestCreateTransaction(t *testing.T) {
	var got sqlc.CreateTransactionParams
	fake := &fakeQuerier{
		createTransaction: func(_ context.Context, arg sqlc.CreateTransactionParams) (sqlc.Transaction, error) {
			got = arg
			return sampleTransaction(), nil
		},
	}
	body := `{"account_id":42,"category_id":8,"amount_cents":-4599,"description":"Weekly grocery run","occurred_on":"2026-01-20"}`
	rec := do(t, newTestAPI(fake), http.MethodPost, "/transactions", body)
	checkStatus(t, rec, http.StatusCreated)
	if rec.Header().Get("Location") != "/transactions/7" {
		t.Errorf("Location = %q", rec.Header().Get("Location"))
	}
	if got.AccountID != 42 || got.AmountCents != -4599 || got.CategoryID == nil || *got.CategoryID != 8 {
		t.Errorf("params = %+v", got)
	}
	if got.OccurredOn.Time.Format(dateLayout) != "2026-01-20" {
		t.Errorf("occurred_on = %s", got.OccurredOn.Time)
	}
}

func TestCreateTransactionValidation(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		field string
	}{
		{name: "missing account", body: `{"amount_cents":1,"occurred_on":"2026-01-20"}`, field: "account_id"},
		{name: "missing amount", body: `{"account_id":42,"occurred_on":"2026-01-20"}`, field: "amount_cents"},
		{name: "missing date", body: `{"account_id":42,"amount_cents":1}`, field: "occurred_on"},
		{name: "bad category", body: `{"account_id":42,"category_id":0,"amount_cents":1,"occurred_on":"2026-01-20"}`, field: "category_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newTestAPI(&fakeQuerier{}), http.MethodPost, "/transactions", tt.body)
			body := checkError(t, rec, http.StatusBadRequest, CodeValidation)
			if !hasDetail(body, tt.field) {
				t.Errorf("details = %+v, want field %q", body.Error.Details, tt.field)
			}
		})
	}
}

func TestCreateTransactionReferences(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		message    string
	}{
		{name: "missing account", constraint: "transactions_account_id_fkey", message: "account does not exist"},
		{name: "missing category", constraint: "transactions_category_id_fkey", message: "category does not exist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeQuerier{
				createTransaction: func(context.Context, sqlc.CreateTransactionParams) (sqlc.Transaction, error) {
					return sqlc.Transaction{}, &pgconn.PgError{Code: pgForeignKeyViolation, ConstraintName: tt.constraint}
				},
			}
			body := `{"account_id":42,"amount_cents":1,"occurred_on":"2026-01-20"}`
			rec := do(t, newTestAPI(fake), http.MethodPost, "/transactions", body)
			errBody := checkError(t, rec, http.StatusBadRequest, CodeInvalidRequest)
			if errBody.Error.Message != tt.message {
				t.Errorf("message = %q, want %q", errBody.Error.Message, tt.message)
			}
		})
	}
}

func TestGetTransaction(t *testing.T) {
	fake := &fakeQuerier{
		getTransaction: func(context.Context, int64) (sqlc.Transaction, error) { return sampleTransaction(), nil },
	}
	rec := do(t, newTestAPI(fake), http.MethodGet, "/transactions/7", "")
	checkStatus(t, rec, http.StatusOK)
	body := decodeResponse[transactionResponse](t, rec)
	if body.CategoryID == nil || *body.CategoryID != 8 || body.Description == nil {
		t.Errorf("body = %+v", body)
	}

	missing := &fakeQuerier{
		getTransaction: func(context.Context, int64) (sqlc.Transaction, error) { return sqlc.Transaction{}, pgx.ErrNoRows },
	}
	checkError(t, do(t, newTestAPI(missing), http.MethodGet, "/transactions/99", ""), http.StatusNotFound, CodeNotFound)
}

func TestUpdateTransaction(t *testing.T) {
	var got sqlc.UpdateTransactionParams
	fake := &fakeQuerier{
		updateTransaction: func(_ context.Context, arg sqlc.UpdateTransactionParams) (sqlc.Transaction, error) {
			got = arg
			return sampleTransaction(), nil
		},
	}
	body := `{"account_id":42,"category_id":null,"amount_cents":100,"description":null,"occurred_on":"2026-01-21"}`
	rec := do(t, newTestAPI(fake), http.MethodPut, "/transactions/7", body)
	checkStatus(t, rec, http.StatusOK)
	if got.ID != 7 || got.CategoryID != nil || got.Description != nil || got.AmountCents != 100 {
		t.Errorf("params = %+v", got)
	}

	missing := &fakeQuerier{
		updateTransaction: func(context.Context, sqlc.UpdateTransactionParams) (sqlc.Transaction, error) {
			return sqlc.Transaction{}, pgx.ErrNoRows
		},
	}
	checkError(t, do(t, newTestAPI(missing), http.MethodPut, "/transactions/99", body), http.StatusNotFound, CodeNotFound)
}

func TestDeleteTransaction(t *testing.T) {
	fake := &fakeQuerier{
		deleteTransaction: func(context.Context, int64) (int64, error) { return 1, nil },
	}
	checkStatus(t, do(t, newTestAPI(fake), http.MethodDelete, "/transactions/7", ""), http.StatusNoContent)

	missing := &fakeQuerier{
		deleteTransaction: func(context.Context, int64) (int64, error) { return 0, nil },
	}
	checkError(t, do(t, newTestAPI(missing), http.MethodDelete, "/transactions/99", ""), http.StatusNotFound, CodeNotFound)
}

// ---- DTO mapping ----

func TestMappingNulls(t *testing.T) {
	account := toAccount(sqlc.Account{ID: 1, Name: "Cash", Type: "cash", Currency: "USD", CreatedAt: fixedTimestamp()})
	if account.ArchivedAt != nil {
		t.Errorf("archived_at = %v, want nil", account.ArchivedAt)
	}

	transaction := toTransaction(sqlc.Transaction{
		ID:          1,
		AccountID:   1,
		AmountCents: 100,
		OccurredOn:  fixedDate("2026-01-20"),
		CreatedAt:   fixedTimestamp(),
		UpdatedAt:   fixedTimestamp(),
	})
	if transaction.CategoryID != nil || transaction.Description != nil {
		t.Errorf("nullable fields = %v/%v, want nil", transaction.CategoryID, transaction.Description)
	}
}

func TestFormatHelpers(t *testing.T) {
	if got := formatDate(pgtype.Date{}); got != "" {
		t.Errorf("formatDate(zero) = %q", got)
	}
	if got := formatTimestamp(pgtype.Timestamptz{}); got != "" {
		t.Errorf("formatTimestamp(zero) = %q", got)
	}
	if got := formatTimestampPtr(pgtype.Timestamptz{}); got != nil {
		t.Errorf("formatTimestampPtr(zero) = %v", got)
	}
}
