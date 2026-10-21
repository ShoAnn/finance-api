-- name: ListAccounts :many
SELECT * FROM accounts
WHERE archived_at IS NULL
ORDER BY name;

-- name: GetAccount :one
SELECT * FROM accounts
WHERE id = $1;

-- name: CreateAccount :one
INSERT INTO accounts (name, type, currency)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ArchiveAccount :exec
UPDATE accounts
SET archived_at = now()
WHERE id = $1;

-- name: DeleteAccount :execrows
DELETE FROM accounts
WHERE id = $1;
