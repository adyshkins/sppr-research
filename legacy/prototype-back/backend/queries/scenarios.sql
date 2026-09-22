-- name: CreateScenario :one
INSERT INTO scenarios (
    case_id,
    name,
    probability,
    loss_vector
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: ListScenariosByCase :many
SELECT *
FROM scenarios
WHERE case_id = $1
ORDER BY created_at DESC;
