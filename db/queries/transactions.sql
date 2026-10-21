-- name: ListTransactions :many
SELECT * FROM transactions
WHERE account_id = $1
  AND occurred_on BETWEEN $2 AND $3
ORDER BY occurred_on DESC, id DESC;

-- name: GetTransaction :one
SELECT * FROM transactions
WHERE id = $1;

-- name: CreateTransaction :one
INSERT INTO transactions (account_id, category_id, amount_cents, description, occurred_on)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateTransaction :one
UPDATE transactions
SET account_id   = $2,
    category_id  = $3,
    amount_cents = $4,
    description  = $5,
    occurred_on  = $6,
    updated_at   = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = $1;

-- name: GetAccountBalance :one
SELECT COALESCE(SUM(amount_cents), 0)::bigint AS balance_cents
FROM transactions
WHERE account_id = $1;
