package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/ShoAnn/finance-api/internal/apikey"
)

// requireAPIKey rejects requests that do not carry a valid API key. The health
// probes stay public so container orchestrators can reach them without a
// credential.
func (a *API) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeUnauthorized(w, r)
			return
		}
		if a.authenticate == nil {
			WriteError(w, r, Internal(errors.New("API key authenticator is not configured")))
			return
		}
		if err := a.authenticate(r.Context(), token); err != nil {
			if apiErr := As(err); apiErr.Status == http.StatusUnauthorized {
				w.Header().Set("WWW-Authenticate", `Bearer realm="finance-api"`)
			}
			WriteError(w, r, err)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header. The scheme is matched case-insensitively, as RFC 7235 requires.
func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

// writeUnauthorized sends the standard JSON 401 response and advertises the
// expected authentication scheme.
func writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="finance-api"`)
	WriteError(w, r, Unauthorized("missing or invalid API key"))
}

// authenticateAPIKey checks a presented token against the stored hash and
// updates the key's last-used timestamp.
func (s *Store) authenticateAPIKey(ctx context.Context, token string) error {
	key, err := s.GetAPIKeyByHash(ctx, apikey.Hash(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Unauthorized("invalid API key")
		}
		return Internal(err)
	}

	if err := s.TouchAPIKey(ctx, key.ID); err != nil {
		// The key is valid; a failed bookkeeping update should not reject it.
		slog.Warn("update API key last_used_at", "id", key.ID, "error", err)
	}
	return nil
}
