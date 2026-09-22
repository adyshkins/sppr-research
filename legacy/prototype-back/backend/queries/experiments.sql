-- name: CreateExperiment :one
INSERT INTO experiments (
    mode,
    metrics
) VALUES (
    $1, $2
)
RETURNING *;

-- name: GetExperiment :one
SELECT *
FROM experiments
WHERE id = $1;

-- name: ListExperiments :many
SELECT *
FROM experiments
ORDER BY run_at DESC, created_at DESC;

-- name: UpdateExperimentMetrics :one
UPDATE experiments
SET metrics = $2
WHERE id = $1
RETURNING *;

-- name: CreateExperimentResult :one
INSERT INTO experiment_results (
    experiment_id,
    case_id,
    mode,
    kpi_deviation,
    cvar,
    violations,
    reproducibility,
    metrics
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: ListExperimentResults :many
SELECT *
FROM experiment_results
WHERE experiment_id = $1
ORDER BY created_at DESC;
