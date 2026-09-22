-- name: CreateDecisionTrace :one
INSERT INTO decision_trace (
    case_id,
    step,
    data_snapshot,
    model_version,
    parameters,
    risk,
    expert_decision,
    final_action_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: ListDecisionTraceByCase :many
SELECT *
FROM decision_trace
WHERE case_id = $1
ORDER BY timestamp ASC, created_at ASC;
