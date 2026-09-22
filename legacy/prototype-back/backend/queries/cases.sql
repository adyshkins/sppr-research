-- name: CreateCase :one
INSERT INTO cases (
    status,
    risk_level
) VALUES (
    $1, $2
)
RETURNING *;

-- name: GetCase :one
SELECT *
FROM cases
WHERE id = $1;

-- name: ListCases :many
SELECT *
FROM cases
ORDER BY created_at DESC;

-- name: ListCasesByStatus :many
SELECT *
FROM cases
WHERE status = $1
ORDER BY created_at DESC;

-- name: UpdateCaseStatus :one
UPDATE cases
SET
    status = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateCaseSelectedAction :one
UPDATE cases
SET
    selected_action_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
