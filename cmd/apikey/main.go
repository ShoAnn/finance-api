// Command apikey creates an API key for one client.
//
// It prints the key once on stdout and stores only its SHA-256 hash. Run it
// once per client; the plaintext key is never persisted and cannot be
// recovered later.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ShoAnn/finance-api/db/sqlc"
	"github.com/ShoAnn/finance-api/internal/apikey"
)

const defaultDatabaseURL = "postgres://finance:finance@localhost:5432/finance?sslmode=disable"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	name := flag.String("name", "", "client name or label (required)")
	flag.Parse()

	label := strings.TrimSpace(*name)
	if label == "" {
		return fmt.Errorf("-name is required, for example: apikey -name laptop")
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = defaultDatabaseURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close(ctx)

	key, hash, err := apikey.Generate()
	if err != nil {
		return err
	}

	row, err := sqlc.New(conn).CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		Name:    label,
		KeyHash: hash,
	})
	if err != nil {
		return fmt.Errorf("store API key: %w", err)
	}

	// Print the key only after the hash is stored, and only to stdout so it
	// can be captured safely. Everything else goes to stderr.
	fmt.Fprintln(os.Stdout, key)
	fmt.Fprintf(os.Stderr, "created API key %q (id %d); this key will not be shown again\n", row.Name, row.ID)
	return nil
}
