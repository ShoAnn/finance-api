package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ShoAnn/finance-api/db/sqlc"
)

// API holds the dependencies shared by the HTTP handlers.
type API struct {
	store            *Store
	readinessTimeout time.Duration
	// authenticate validates a presented API key. main wires it to the
	// database-backed Store.authenticateAPIKey.
	authenticate func(context.Context, string) error
}

// routes returns the HTTP handler with every route registered.
func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("GET /readyz", a.handleReady)

	mux.HandleFunc("GET /accounts", a.handle(a.listAccounts))
	mux.HandleFunc("POST /accounts", a.handle(a.createAccount))
	mux.HandleFunc("GET /accounts/{accountId}", a.handle(a.getAccount))
	mux.HandleFunc("DELETE /accounts/{accountId}", a.handle(a.deleteAccount))
	mux.HandleFunc("POST /accounts/{accountId}/archive", a.handle(a.archiveAccount))
	mux.HandleFunc("GET /accounts/{accountId}/balance", a.handle(a.accountBalance))

	mux.HandleFunc("GET /categories", a.handle(a.listCategories))
	mux.HandleFunc("POST /categories", a.handle(a.createCategory))
	mux.HandleFunc("PUT /categories/{categoryId}", a.handle(a.updateCategory))
	mux.HandleFunc("DELETE /categories/{categoryId}", a.handle(a.deleteCategory))

	mux.HandleFunc("GET /transactions", a.handle(a.listTransactions))
	mux.HandleFunc("POST /transactions", a.handle(a.createTransaction))
	mux.HandleFunc("GET /transactions/{transactionId}", a.handle(a.getTransaction))
	mux.HandleFunc("PUT /transactions/{transactionId}", a.handle(a.updateTransaction))
	mux.HandleFunc("DELETE /transactions/{transactionId}", a.handle(a.deleteTransaction))

	// Method fallbacks replace the mux's plain-text 405 responses.
	mux.HandleFunc("/healthz", a.methodNotAllowed(http.MethodGet))
	mux.HandleFunc("/readyz", a.methodNotAllowed(http.MethodGet))
	mux.HandleFunc("/accounts", a.methodNotAllowed(http.MethodGet, http.MethodPost))
	mux.HandleFunc("/accounts/{accountId}", a.methodNotAllowed(http.MethodGet, http.MethodDelete))
	mux.HandleFunc("/accounts/{accountId}/archive", a.methodNotAllowed(http.MethodPost))
	mux.HandleFunc("/accounts/{accountId}/balance", a.methodNotAllowed(http.MethodGet))
	mux.HandleFunc("/categories", a.methodNotAllowed(http.MethodGet, http.MethodPost))
	mux.HandleFunc("/categories/{categoryId}", a.methodNotAllowed(http.MethodPut, http.MethodDelete))
	mux.HandleFunc("/transactions", a.methodNotAllowed(http.MethodGet, http.MethodPost))
	mux.HandleFunc("/transactions/{transactionId}", a.methodNotAllowed(http.MethodGet, http.MethodPut, http.MethodDelete))

	// Everything else is a JSON 404.
	mux.HandleFunc("/", a.notFound)

	return logRequests(a.requireAPIKey(mux))
}

// handle adapts a handler that returns an error to an http.HandlerFunc.
func (a *API) handle(h func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}

func (a *API) notFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, NotFound("resource not found"))
}

func (a *API) methodNotAllowed(methods ...string) http.HandlerFunc {
	allow := strings.Join(methods, ", ")
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		WriteError(w, r, &APIError{
			Status:  http.StatusMethodNotAllowed,
			Code:    CodeMethodNotAllowed,
			Message: "method not allowed",
		})
	}
}

// statusRecorder captures the response status for request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	if s.status != 0 {
		return
	}
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// logRequests logs method, path, status, and duration for every request.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration", time.Since(start),
		)
	})
}

// handleHealth reports that the process is running. It does not check
// dependencies.
func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady reports whether the API can serve traffic, which requires the
// database to be reachable.
func (a *API) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.readinessTimeout)
	defer cancel()
	if err := a.store.Ping(ctx); err != nil {
		WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "error": "database unreachable"})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// validationErrors accumulates field-level problems before responding.
type validationErrors struct {
	details []Detail
}

func (v *validationErrors) add(field, message string) {
	v.details = append(v.details, Detail{Field: field, Message: message})
}

func (v *validationErrors) err() error {
	if len(v.details) == 0 {
		return nil
	}
	return Validation(v.details...)
}

func (v *validationErrors) oneOf(field, value string, allowed ...string) {
	if value == "" {
		v.add(field, "is required")
		return
	}
	for _, candidate := range allowed {
		if value == candidate {
			return
		}
	}
	v.add(field, "must be one of "+strings.Join(allowed, ", "))
}

func (v *validationErrors) minLength(field, value string, min int) {
	if value == "" {
		v.add(field, "is required")
		return
	}
	if len(value) < min {
		v.add(field, fmt.Sprintf("must be at least %d characters", min))
	}
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func (v *validationErrors) currency(value string) {
	if !currencyPattern.MatchString(value) {
		v.add("currency", "must be a 3-letter ISO 4217 code")
	}
}

// decodeJSON reads exactly one JSON object into dst, rejecting unknown fields
// and trailing data.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) *APIError {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if stderrors.As(err, &maxErr) {
			return InvalidRequest("request body is too large")
		}
		return InvalidRequest("request body must be a valid JSON object")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return InvalidRequest("request body must contain a single JSON object")
	}
	return nil
}

// pathID parses a positive identifier from a path parameter.
func pathID(r *http.Request, field string) (int64, *APIError) {
	id, err := strconv.ParseInt(r.PathValue(field), 10, 64)
	if err != nil || id < 1 {
		return 0, Validation(Detail{Field: field, Message: "must be a positive integer"})
	}
	return id, nil
}

// queryID parses a required positive identifier from the query string.
func queryID(r *http.Request, field string, v *validationErrors) (int64, bool) {
	raw := r.URL.Query().Get(field)
	if raw == "" {
		v.add(field, "is required")
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		v.add(field, "must be a positive integer")
		return 0, false
	}
	return id, true
}

const dateLayout = "2006-01-02"

func parseDate(field, value string, v *validationErrors) (pgtype.Date, bool) {
	if value == "" {
		v.add(field, "is required")
		return pgtype.Date{}, false
	}
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		v.add(field, "must be a date in YYYY-MM-DD format")
		return pgtype.Date{}, false
	}
	return pgtype.Date{Time: t, Valid: true}, true
}

func formatDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(dateLayout)
}

func formatTimestamp(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

func formatTimestampPtr(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := formatTimestamp(ts)
	return &s
}

// fromDB translates a database error, naming the resource in 404 responses.
func fromDB(err error, resource string) *APIError {
	apiErr := FromDB(err)
	if apiErr != nil && apiErr.Code == CodeNotFound {
		apiErr.Message = resource + " not found"
	}
	return apiErr
}

func asPgError(err error) *pgconn.PgError {
	var pgErr *pgconn.PgError
	if stderrors.As(err, &pgErr) {
		return pgErr
	}
	return nil
}

func isForeignKeyViolation(err error) bool {
	pgErr := asPgError(err)
	return pgErr != nil && pgErr.Code == pgForeignKeyViolation
}

// ---- accounts ----

type accountResponse struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Currency   string  `json:"currency"`
	CreatedAt  string  `json:"created_at"`
	ArchivedAt *string `json:"archived_at"`
}

func toAccount(a sqlc.Account) accountResponse {
	return accountResponse{
		ID:         a.ID,
		Name:       a.Name,
		Type:       a.Type,
		Currency:   a.Currency,
		CreatedAt:  formatTimestamp(a.CreatedAt),
		ArchivedAt: formatTimestampPtr(a.ArchivedAt),
	}
}

func toAccounts(accounts []sqlc.Account) []accountResponse {
	out := make([]accountResponse, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, toAccount(account))
	}
	return out
}

type accountCreateRequest struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Currency *string `json:"currency"`
}

var accountTypes = []string{"checking", "savings", "credit_card", "cash", "investment"}

func (a *API) listAccounts(w http.ResponseWriter, r *http.Request) error {
	accounts, err := a.store.ListAccounts(r.Context())
	if err != nil {
		return fromDB(err, "account")
	}
	WriteJSON(w, http.StatusOK, toAccounts(accounts))
	return nil
}

func (a *API) createAccount(w http.ResponseWriter, r *http.Request) error {
	var req accountCreateRequest
	if apiErr := decodeJSON(w, r, &req); apiErr != nil {
		return apiErr
	}

	v := &validationErrors{}
	name := strings.TrimSpace(req.Name)
	v.minLength("name", name, 1)
	v.oneOf("type", req.Type, accountTypes...)
	currency := "IDR"
	if req.Currency != nil {
		currency = strings.ToUpper(strings.TrimSpace(*req.Currency))
		v.currency(currency)
	}
	if err := v.err(); err != nil {
		return err
	}

	account, err := a.store.CreateAccount(r.Context(), sqlc.CreateAccountParams{
		Name:     name,
		Type:     req.Type,
		Currency: currency,
	})
	if err != nil {
		return fromDB(err, "account")
	}

	w.Header().Set("Location", fmt.Sprintf("/accounts/%d", account.ID))
	WriteJSON(w, http.StatusCreated, toAccount(account))
	return nil
}

func (a *API) getAccount(w http.ResponseWriter, r *http.Request) error {
	id, apiErr := pathID(r, "accountId")
	if apiErr != nil {
		return apiErr
	}
	account, err := a.store.GetAccount(r.Context(), id)
	if err != nil {
		return fromDB(err, "account")
	}
	WriteJSON(w, http.StatusOK, toAccount(account))
	return nil
}

func (a *API) archiveAccount(w http.ResponseWriter, r *http.Request) error {
	id, apiErr := pathID(r, "accountId")
	if apiErr != nil {
		return apiErr
	}
	if err := a.store.ArchiveAccount(r.Context(), id); err != nil {
		return fromDB(err, "account")
	}
	account, err := a.store.GetAccount(r.Context(), id)
	if err != nil {
		return fromDB(err, "account")
	}
	WriteJSON(w, http.StatusOK, toAccount(account))
	return nil
}

func (a *API) accountBalance(w http.ResponseWriter, r *http.Request) error {
	id, apiErr := pathID(r, "accountId")
	if apiErr != nil {
		return apiErr
	}
	if _, err := a.store.GetAccount(r.Context(), id); err != nil {
		return fromDB(err, "account")
	}
	balance, err := a.store.GetAccountBalance(r.Context(), id)
	if err != nil {
		return fromDB(err, "account")
	}
	WriteJSON(w, http.StatusOK, balanceResponse{AccountID: id, BalanceCents: balance})
	return nil
}

func (a *API) deleteAccount(w http.ResponseWriter, r *http.Request) error {
	id, apiErr := pathID(r, "accountId")
	if apiErr != nil {
		return apiErr
	}
	rows, err := a.store.DeleteAccount(r.Context(), id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return Conflict("account has transactions and cannot be deleted")
		}
		return fromDB(err, "account")
	}
	if rows == 0 {
		return NotFound("account not found")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type balanceResponse struct {
	AccountID    int64 `json:"account_id"`
	BalanceCents int64 `json:"balance_cents"`
}

// ---- categories ----

type categoryResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
}

func toCategory(c sqlc.Category) categoryResponse {
	return categoryResponse{
		ID:        c.ID,
		Name:      c.Name,
		Kind:      c.Kind,
		CreatedAt: formatTimestamp(c.CreatedAt),
	}
}

func toCategories(categories []sqlc.Category) []categoryResponse {
	out := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		out = append(out, toCategory(category))
	}
	return out
}

type categoryRequest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

var categoryKinds = []string{"income", "expense"}

func (a *API) listCategories(w http.ResponseWriter, r *http.Request) error {
	categories, err := a.store.ListCategories(r.Context())
	if err != nil {
		return fromDB(err, "category")
	}
	WriteJSON(w, http.StatusOK, toCategories(categories))
	return nil
}

func (a *API) createCategory(w http.ResponseWriter, r *http.Request) error {
	var req categoryRequest
	if apiErr := decodeJSON(w, r, &req); apiErr != nil {
		return apiErr
	}
	name, kind, err := validateCategory(req)
	if err != nil {
		return err
	}

	category, err := a.store.CreateCategory(r.Context(), sqlc.CreateCategoryParams{Name: name, Kind: kind})
	if err != nil {
		return fromDB(err, "category")
	}

	w.Header().Set("Location", fmt.Sprintf("/categories/%d", category.ID))
	WriteJSON(w, http.StatusCreated, toCategory(category))
	return nil
}

func (a *API) updateCategory(w http.ResponseWriter, r *http.Request) error {
	id, apiErr := pathID(r, "categoryId")
	if apiErr != nil {
		return apiErr
	}
	var req categoryRequest
	if apiErr := decodeJSON(w, r, &req); apiErr != nil {
		return apiErr
	}
	name, kind, err := validateCategory(req)
	if err != nil {
		return err
	}

	category, err := a.store.UpdateCategory(r.Context(), sqlc.UpdateCategoryParams{
		ID:   id,
		Name: name,
		Kind: kind,
	})
	if err != nil {
		return fromDB(err, "category")
	}
	WriteJSON(w, http.StatusOK, toCategory(category))
	return nil
}

func validateCategory(req categoryRequest) (string, string, error) {
	v := &validationErrors{}
	name := strings.TrimSpace(req.Name)
	v.minLength("name", name, 1)
	v.oneOf("kind", req.Kind, categoryKinds...)
	if err := v.err(); err != nil {
		return "", "", err
	}
	return name, req.Kind, nil
}

func (a *API) deleteCategory(w http.ResponseWriter, r *http.Request) error {
	id, apiErr := pathID(r, "categoryId")
	if apiErr != nil {
		return apiErr
	}
	rows, err := a.store.DeleteCategory(r.Context(), id)
	if err != nil {
		return fromDB(err, "category")
	}
	if rows == 0 {
		return NotFound("category not found")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- transactions ----

type transactionResponse struct {
	ID          int64   `json:"id"`
	AccountID   int64   `json:"account_id"`
	CategoryID  *int64  `json:"category_id"`
	AmountCents int64   `json:"amount_cents"`
	Description *string `json:"description"`
	OccurredOn  string  `json:"occurred_on"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func toTransaction(t sqlc.Transaction) transactionResponse {
	return transactionResponse{
		ID:          t.ID,
		AccountID:   t.AccountID,
		CategoryID:  t.CategoryID,
		AmountCents: t.AmountCents,
		Description: t.Description,
		OccurredOn:  formatDate(t.OccurredOn),
		CreatedAt:   formatTimestamp(t.CreatedAt),
		UpdatedAt:   formatTimestamp(t.UpdatedAt),
	}
}

func toTransactions(transactions []sqlc.Transaction) []transactionResponse {
	out := make([]transactionResponse, 0, len(transactions))
	for _, transaction := range transactions {
		out = append(out, toTransaction(transaction))
	}
	return out
}

type transactionRequest struct {
	AccountID   *int64  `json:"account_id"`
	CategoryID  *int64  `json:"category_id"`
	AmountCents *int64  `json:"amount_cents"`
	Description *string `json:"description"`
	OccurredOn  string  `json:"occurred_on"`
}

func (a *API) listTransactions(w http.ResponseWriter, r *http.Request) error {
	v := &validationErrors{}
	accountID, _ := queryID(r, "account_id", v)
	from, fromOK := parseDate("from", r.URL.Query().Get("from"), v)
	to, toOK := parseDate("to", r.URL.Query().Get("to"), v)
	if fromOK && toOK && from.Time.After(to.Time) {
		v.add("from", "must not be after to")
	}
	if err := v.err(); err != nil {
		return err
	}

	transactions, err := a.store.ListTransactions(r.Context(), sqlc.ListTransactionsParams{
		AccountID: accountID,
		FromDate:  from,
		ToDate:    to,
	})
	if err != nil {
		return fromDB(err, "transaction")
	}
	WriteJSON(w, http.StatusOK, toTransactions(transactions))
	return nil
}

func validateTransaction(req transactionRequest) (pgtype.Date, error) {
	v := &validationErrors{}
	if req.AccountID == nil {
		v.add("account_id", "is required")
	} else if *req.AccountID < 1 {
		v.add("account_id", "must be a positive integer")
	}
	if req.CategoryID != nil && *req.CategoryID < 1 {
		v.add("category_id", "must be a positive integer")
	}
	if req.AmountCents == nil {
		v.add("amount_cents", "is required")
	}
	occurredOn, _ := parseDate("occurred_on", req.OccurredOn, v)
	if err := v.err(); err != nil {
		return pgtype.Date{}, err
	}
	return occurredOn, nil
}

func (a *API) createTransaction(w http.ResponseWriter, r *http.Request) error {
	var req transactionRequest
	if apiErr := decodeJSON(w, r, &req); apiErr != nil {
		return apiErr
	}
	occurredOn, err := validateTransaction(req)
	if err != nil {
		return err
	}

	transaction, err := a.store.CreateTransaction(r.Context(), sqlc.CreateTransactionParams{
		AccountID:   *req.AccountID,
		CategoryID:  req.CategoryID,
		AmountCents: *req.AmountCents,
		Description: req.Description,
		OccurredOn:  occurredOn,
	})
	if err != nil {
		return transactionWriteError(err)
	}

	w.Header().Set("Location", fmt.Sprintf("/transactions/%d", transaction.ID))
	WriteJSON(w, http.StatusCreated, toTransaction(transaction))
	return nil
}

func (a *API) getTransaction(w http.ResponseWriter, r *http.Request) error {
	id, apiErr := pathID(r, "transactionId")
	if apiErr != nil {
		return apiErr
	}
	transaction, err := a.store.GetTransaction(r.Context(), id)
	if err != nil {
		return fromDB(err, "transaction")
	}
	WriteJSON(w, http.StatusOK, toTransaction(transaction))
	return nil
}

func (a *API) updateTransaction(w http.ResponseWriter, r *http.Request) error {
	id, apiErr := pathID(r, "transactionId")
	if apiErr != nil {
		return apiErr
	}
	var req transactionRequest
	if apiErr := decodeJSON(w, r, &req); apiErr != nil {
		return apiErr
	}
	occurredOn, err := validateTransaction(req)
	if err != nil {
		return err
	}

	transaction, err := a.store.UpdateTransaction(r.Context(), sqlc.UpdateTransactionParams{
		ID:          id,
		AccountID:   *req.AccountID,
		CategoryID:  req.CategoryID,
		AmountCents: *req.AmountCents,
		Description: req.Description,
		OccurredOn:  occurredOn,
	})
	if err != nil {
		return transactionWriteError(err)
	}
	WriteJSON(w, http.StatusOK, toTransaction(transaction))
	return nil
}

func (a *API) deleteTransaction(w http.ResponseWriter, r *http.Request) error {
	id, apiErr := pathID(r, "transactionId")
	if apiErr != nil {
		return apiErr
	}
	rows, err := a.store.DeleteTransaction(r.Context(), id)
	if err != nil {
		return fromDB(err, "transaction")
	}
	if rows == 0 {
		return NotFound("transaction not found")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// transactionWriteError turns reference failures into 400s rather than the
// generic 409 that deleting a referenced resource produces.
func transactionWriteError(err error) *APIError {
	if pgErr := asPgError(err); pgErr != nil && pgErr.Code == pgForeignKeyViolation {
		switch pgErr.ConstraintName {
		case "transactions_account_id_fkey":
			return InvalidRequest("account does not exist")
		case "transactions_category_id_fkey":
			return InvalidRequest("category does not exist")
		}
	}
	return fromDB(err, "transaction")
}

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
	// CodeMethodNotAllowed reports a method the route does not support.
	CodeMethodNotAllowed Code = "method_not_allowed"
	// CodeUnauthorized reports a missing or invalid API key.
	CodeUnauthorized Code = "unauthorized"
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

// Unauthorized reports a missing or invalid API key.
func Unauthorized(message string) *APIError {
	return &APIError{
		Status:  http.StatusUnauthorized,
		Code:    CodeUnauthorized,
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

// Postgres error codes handled explicitly.
const (
	pgUniqueViolation           = "23505"
	pgForeignKeyViolation       = "23503"
	pgCheckViolation            = "23514"
	pgInvalidTextRepresentation = "22P02"
)

// uniqueMessages maps a unique constraint to a client-facing message.
var uniqueMessages = map[string]string{
	"categories_name_key": "category name already exists",
}

// checkDetails maps a check constraint to the field it constrains.
var checkDetails = map[string]Detail{
	"accounts_type_check": {
		Field:   "type",
		Message: "must be one of checking, savings, credit_card, cash, investment",
	},
	"categories_kind_check": {
		Field:   "kind",
		Message: "must be one of income, expense",
	},
}

// FromDB translates a database error into an API error.
//
// Foreign key violations are reported as conflicts, which is correct when
// deleting a resource that other records still reference. Handlers that insert
// a row with a missing reference should report a 400 instead, for example:
//
//	if pgErr := FromDB(err); pgErr.Code == CodeConflict {
//		return InvalidRequest("account does not exist")
//	}
func FromDB(err error) *APIError {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, pgx.ErrNoRows) {
		return NotFound("resource not found")
	}

	pgErr := asPgError(err)
	if pgErr == nil {
		return Internal(err)
	}

	switch pgErr.Code {
	case pgUniqueViolation:
		message, ok := uniqueMessages[pgErr.ConstraintName]
		if !ok {
			message = "resource already exists"
		}
		return Conflict(message)
	case pgCheckViolation:
		detail, ok := checkDetails[pgErr.ConstraintName]
		if !ok {
			detail = Detail{Field: "value", Message: "violates a database constraint"}
		}
		return Validation(detail)
	case pgInvalidTextRepresentation:
		return InvalidRequest("malformed value")
	case pgForeignKeyViolation:
		return Conflict("resource is referenced by other records")
	default:
		return Internal(err)
	}
}
