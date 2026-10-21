package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ShoAnn/finance-api/db/sqlc"
)

// integrationStore connects to TEST_DATABASE_URL and empties the tables. Tests
// are skipped when the variable is unset, so `go test ./...` stays offline.
func integrationStore(t *testing.T) *Store {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	store := NewStore(pool)
	if err := store.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE transactions, categories, accounts RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return store
}

func TestIntegrationConstraintMapping(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()

	account, err := store.CreateAccount(ctx, sqlc.CreateAccountParams{
		Name: "Everyday Checking", Type: "checking", Currency: "USD",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	category, err := store.CreateCategory(ctx, sqlc.CreateCategoryParams{Name: "Groceries", Kind: "expense"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	// Unique violation.
	_, err = store.CreateCategory(ctx, sqlc.CreateCategoryParams{Name: "Groceries", Kind: "expense"})
	if apiErr := FromDB(err); apiErr.Code != CodeConflict || apiErr.Message != "category name already exists" {
		t.Errorf("duplicate category = %+v", apiErr)
	}

	// Check constraint.
	_, err = store.CreateAccount(ctx, sqlc.CreateAccountParams{Name: "Bad", Type: "crypto", Currency: "USD"})
	apiErr := FromDB(err)
	if apiErr.Code != CodeValidation || len(apiErr.Details) != 1 || apiErr.Details[0].Field != "type" {
		t.Errorf("bad account type = %+v", apiErr)
	}

	// Missing reference on insert.
	occurredOn := fixedDate("2026-01-20")
	_, err = store.CreateTransaction(ctx, sqlc.CreateTransactionParams{
		AccountID: 9999, AmountCents: 100, OccurredOn: occurredOn,
	})
	if apiErr := transactionWriteError(err); apiErr.Code != CodeInvalidRequest || apiErr.Message != "account does not exist" {
		t.Errorf("missing account = %+v", apiErr)
	}

	missingCategory := int64(9999)
	_, err = store.CreateTransaction(ctx, sqlc.CreateTransactionParams{
		AccountID: account.ID, CategoryID: &missingCategory, AmountCents: 100, OccurredOn: occurredOn,
	})
	if apiErr := transactionWriteError(err); apiErr.Code != CodeInvalidRequest || apiErr.Message != "category does not exist" {
		t.Errorf("missing category = %+v", apiErr)
	}

	// Successful transaction and balance.
	transaction, err := store.CreateTransaction(ctx, sqlc.CreateTransactionParams{
		AccountID: account.ID, CategoryID: &category.ID, AmountCents: -4599, OccurredOn: occurredOn,
	})
	if err != nil {
		t.Fatalf("create transaction: %v", err)
	}
	second, err := store.CreateTransaction(ctx, sqlc.CreateTransactionParams{
		AccountID: account.ID, AmountCents: 5000, OccurredOn: occurredOn,
	})
	if err != nil {
		t.Fatalf("create second transaction: %v", err)
	}

	balance, err := store.GetAccountBalance(ctx, account.ID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if balance != 401 {
		t.Errorf("balance = %d, want 401", balance)
	}

	// ON DELETE SET NULL for categories.
	if _, err := store.DeleteCategory(ctx, category.ID); err != nil {
		t.Fatalf("delete category: %v", err)
	}
	reloaded, err := store.GetTransaction(ctx, transaction.ID)
	if err != nil {
		t.Fatalf("reload transaction: %v", err)
	}
	if reloaded.CategoryID != nil {
		t.Errorf("category_id = %v, want nil", reloaded.CategoryID)
	}

	// ON DELETE RESTRICT for referenced accounts.
	if _, err := store.DeleteAccount(ctx, account.ID); !isForeignKeyViolation(err) {
		t.Errorf("delete referenced account error = %v, want a foreign key violation", err)
	}

	if _, err := store.DeleteTransaction(ctx, transaction.ID); err != nil {
		t.Fatalf("delete transaction: %v", err)
	}
	if _, err := store.DeleteTransaction(ctx, second.ID); err != nil {
		t.Fatalf("delete second transaction: %v", err)
	}
	if _, err := store.DeleteAccount(ctx, account.ID); err != nil {
		t.Fatalf("delete account: %v", err)
	}
}
