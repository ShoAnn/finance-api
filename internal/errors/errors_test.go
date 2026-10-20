package errors

import (
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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMain(m *testing.M) {
	// Keep expected 5xx logging out of the test output.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
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

func write(t *testing.T, err error) (*httptest.ResponseRecorder, errorBody) {
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

func TestWriteErrorNotFound(t *testing.T) {
	rec, body := write(t, NotFound("account not found"))

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

	rec, body := write(t, wrapped)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if body.Error.Message != "account not found" {
		t.Errorf("message = %q, want %q", body.Error.Message, "account not found")
	}
}

func TestWriteErrorUnknownBecomesInternal(t *testing.T) {
	rec, body := write(t, stderrors.New("connection refused"))

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
	rec, body := write(t, Validation(Detail{Field: "amount_cents", Message: "expected an integer"}))

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
	rec, _ := write(t, nil)
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
