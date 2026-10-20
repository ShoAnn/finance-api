// Package errors defines the API error types, the JSON error envelope, and
// the translation of database failures into API errors.
//
// The package is named "errors" to match its directory, which collides with
// the standard library package. Import it under an alias:
//
//	import apierr "github.com/ShoAnn/finance-api/internal/errors"
package errors

import (
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http"
)

// Code is a stable, machine-readable error identifier.
type Code string

const (
	// CodeInvalidRequest reports a malformed request, such as a body that is
	// not valid JSON.
	CodeInvalidRequest Code = "invalid_request"
	// CodeValidation reports a well-formed request that fails validation.
	CodeValidation Code = "validation_error"
	// CodeNotFound reports a resource that does not exist.
	CodeNotFound Code = "not_found"
	// CodeConflict reports a request that conflicts with the current state of
	// a resource.
	CodeConflict Code = "conflict"
	// CodeInternal reports an unexpected server-side failure.
	CodeInternal Code = "internal_error"
)

// Detail describes a single field-level validation failure.
type Detail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// APIError is an error that carries the HTTP status and the JSON body to
// return to the client. The wire shape is:
//
//	{"error":{"code":"not_found","message":"account not found"}}
type APIError struct {
	// Status is the HTTP status code to respond with.
	Status int `json:"-"`
	// Code is the machine-readable error identifier.
	Code Code `json:"code"`
	// Message is the human-readable explanation.
	Message string `json:"message"`
	// Details holds field-level validation failures, when applicable.
	Details []Detail `json:"details,omitempty"`
	// Cause is the underlying error. It is never sent to the client.
	Cause error `json:"-"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return string(e.Code) + ": " + e.Message
}

// Unwrap returns the underlying cause, if any.
func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// envelope is the top-level JSON body of an error response.
type envelope struct {
	Error *APIError `json:"error"`
}

// InvalidRequest reports a malformed request.
func InvalidRequest(message string) *APIError {
	return &APIError{
		Status:  http.StatusBadRequest,
		Code:    CodeInvalidRequest,
		Message: message,
	}
}

// Validation reports field-level validation failures.
func Validation(details ...Detail) *APIError {
	return &APIError{
		Status:  http.StatusBadRequest,
		Code:    CodeValidation,
		Message: "validation failed",
		Details: details,
	}
}

// NotFound reports a missing resource.
func NotFound(message string) *APIError {
	return &APIError{
		Status:  http.StatusNotFound,
		Code:    CodeNotFound,
		Message: message,
	}
}

// Conflict reports a request that conflicts with the current state of a
// resource.
func Conflict(message string) *APIError {
	return &APIError{
		Status:  http.StatusConflict,
		Code:    CodeConflict,
		Message: message,
	}
}

// Internal reports an unexpected failure. The cause is logged, never sent to
// the client.
func Internal(cause error) *APIError {
	return &APIError{
		Status:  http.StatusInternalServerError,
		Code:    CodeInternal,
		Message: "internal server error",
		Cause:   cause,
	}
}

// As extracts an *APIError from err, wrapping anything it does not recognise
// as an internal error. It returns nil when err is nil.
func As(err error) *APIError {
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if stderrors.As(err, &apiErr) {
		if apiErr.Status == 0 {
			apiErr.Status = http.StatusInternalServerError
		}
		return apiErr
	}
	return Internal(err)
}

// WriteError writes err to w as a JSON error envelope. Errors that are not
// already an *APIError are reported as internal errors, and only 5xx
// responses are logged.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	apiErr := As(err)
	if apiErr.Status >= http.StatusInternalServerError {
		attrs := []any{"error", err}
		if r != nil {
			attrs = append([]any{"method", r.Method, "path", r.URL.Path}, attrs...)
		}
		slog.Error("request failed", attrs...)
	}
	WriteJSON(w, apiErr.Status, envelope{Error: apiErr})
}

// WriteJSON writes body to w as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write response", "error", err)
	}
}
