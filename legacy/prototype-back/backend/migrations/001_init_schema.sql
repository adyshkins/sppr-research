-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Base schema placeholder.
-- The next stage can add domain tables:
-- kpi, kpi_snapshots, cases, events, diagnostics, scenarios,
-- corrective_actions, decision_trace, experiments, experiment_results.

-- +goose Down
DROP EXTENSION IF EXISTS pgcrypto;
