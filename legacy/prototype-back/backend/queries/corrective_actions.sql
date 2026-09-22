-- name: CreateCorrectiveAction :one
INSERT INTO corrective_actions (
    case_id,
    code,
    description,
    action_type,
    expected_loss,
    var_value,
    cvar_value,
    constraints_violated,
    is_selected
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: ListCorrectiveActionsByCase :many
SELECT *
FROM corrective_actions
WHERE case_id = $1
ORDER BY created_at DESC;

-- name: SelectCorrectiveAction :one
UPDATE corrective_actions
SET is_selected = true
WHERE id = $1
RETURNING *;

-- name: ClearSelectedCorrectiveActionsByCase :exec
UPDATE corrective_actions
SET is_selected = false
WHERE case_id = $1;
