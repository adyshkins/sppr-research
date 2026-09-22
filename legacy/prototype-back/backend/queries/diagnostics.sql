-- name: CreateDiagnostic :one
INSERT INTO diagnostics (
    case_id,
    hypothesis,
    confidence,
    details
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: ListDiagnosticsByCase :many
SELECT *
FROM diagnostics
WHERE case_id = $1
ORDER BY created_at DESC;
