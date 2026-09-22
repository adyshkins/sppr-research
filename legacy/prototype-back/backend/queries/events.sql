-- name: CreateEvent :one
INSERT INTO events (
    case_id,
    occurred_at,
    source,
    raw_data
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: GetEvent :one
SELECT *
FROM events
WHERE id = $1;

-- name: ListEventsByCase :many
SELECT *
FROM events
WHERE case_id = $1
ORDER BY occurred_at DESC, created_at DESC;
