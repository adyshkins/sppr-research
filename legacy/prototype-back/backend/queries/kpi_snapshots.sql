-- name: CreateKPISnapshot :one
INSERT INTO kpi_snapshots (
    case_id,
    kpi_id,
    planned,
    actual,
    deviation
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: ListKPISnapshotsByCase :many
SELECT *
FROM kpi_snapshots
WHERE case_id = $1
ORDER BY created_at DESC;
