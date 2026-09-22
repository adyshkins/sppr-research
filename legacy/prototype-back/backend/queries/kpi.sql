-- name: CreateKPI :one
INSERT INTO kpi (
    code,
    name,
    target_value,
    lower_bound,
    upper_bound,
    weight,
    unit,
    description
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetKPI :one
SELECT *
FROM kpi
WHERE id = $1;

-- name: ListKPI :many
SELECT *
FROM kpi
ORDER BY code;

-- name: UpdateKPI :one
UPDATE kpi
SET
    code = $2,
    name = $3,
    target_value = $4,
    lower_bound = $5,
    upper_bound = $6,
    weight = $7,
    unit = $8,
    description = $9
WHERE id = $1
RETURNING *;

-- name: DeleteKPI :exec
DELETE FROM kpi
WHERE id = $1;
