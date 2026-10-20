-- name: ListCategories :many
SELECT * FROM categories
ORDER BY name;

-- name: CreateCategory :one
INSERT INTO categories (name, kind)
VALUES ($1, $2)
RETURNING *;

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = $1;
