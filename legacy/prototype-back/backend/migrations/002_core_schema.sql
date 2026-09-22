-- +goose Up
CREATE TABLE kpi (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    target_value numeric(14,4) NOT NULL,
    lower_bound numeric(14,4),
    upper_bound numeric(14,4),
    weight numeric(8,4) NOT NULL DEFAULT 1,
    unit text,
    description text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    status text NOT NULL,
    risk_level text,
    selected_action_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cases_status_check CHECK (
        status IN (
            'registered',
            'normalized',
            'deviation_confirmed',
            'diagnosed',
            'risk_assessed',
            'alternatives_generated',
            'action_selected',
            'expert_validation',
            'expert_approved',
            'expert_rejected',
            'expert_returned',
            'executed',
            'archived'
        )
    )
);

CREATE TABLE events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id uuid NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    source text NOT NULL DEFAULT 'manual',
    raw_data jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE kpi_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id uuid NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    kpi_id uuid NOT NULL REFERENCES kpi(id),
    planned numeric(14,4) NOT NULL,
    actual numeric(14,4) NOT NULL,
    deviation numeric(14,4) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE diagnostics (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id uuid NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    hypothesis text NOT NULL,
    confidence numeric(8,4) NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE scenarios (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id uuid NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    name text NOT NULL,
    probability numeric(8,6) NOT NULL,
    loss_vector jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE corrective_actions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id uuid NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    code text,
    description text NOT NULL,
    action_type text NOT NULL,
    expected_loss numeric(14,4),
    var_value numeric(14,4),
    cvar_value numeric(14,4),
    constraints_violated jsonb NOT NULL DEFAULT '[]'::jsonb,
    is_selected boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE cases
    ADD CONSTRAINT cases_selected_action_id_fkey
    FOREIGN KEY (selected_action_id) REFERENCES corrective_actions(id);

CREATE TABLE decision_trace (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id uuid NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    step text NOT NULL,
    timestamp timestamptz NOT NULL DEFAULT now(),
    data_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    model_version text,
    parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
    risk jsonb NOT NULL DEFAULT '{}'::jsonb,
    expert_decision jsonb NOT NULL DEFAULT '{}'::jsonb,
    final_action_id uuid REFERENCES corrective_actions(id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE experiments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_at timestamptz NOT NULL DEFAULT now(),
    mode text NOT NULL,
    metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT experiments_mode_check CHECK (mode IN ('base', 'intelligent'))
);

CREATE TABLE experiment_results (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id uuid NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    case_id uuid REFERENCES cases(id) ON DELETE SET NULL,
    mode text NOT NULL,
    kpi_deviation numeric(14,4),
    cvar numeric(14,4),
    violations integer NOT NULL DEFAULT 0,
    reproducibility numeric(8,4),
    metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT experiment_results_mode_check CHECK (mode IN ('base', 'intelligent'))
);

CREATE INDEX idx_cases_status ON cases(status);
CREATE INDEX idx_cases_created_at ON cases(created_at);
CREATE INDEX idx_events_case_id ON events(case_id);
CREATE INDEX idx_kpi_snapshots_case_id ON kpi_snapshots(case_id);
CREATE INDEX idx_diagnostics_case_id ON diagnostics(case_id);
CREATE INDEX idx_scenarios_case_id ON scenarios(case_id);
CREATE INDEX idx_corrective_actions_case_id ON corrective_actions(case_id);
CREATE INDEX idx_decision_trace_case_id ON decision_trace(case_id);
CREATE INDEX idx_decision_trace_step ON decision_trace(step);
CREATE INDEX idx_experiment_results_experiment_id ON experiment_results(experiment_id);

-- +goose Down
DROP TABLE IF EXISTS experiment_results;
DROP TABLE IF EXISTS experiments;
DROP TABLE IF EXISTS decision_trace;
ALTER TABLE IF EXISTS cases DROP CONSTRAINT IF EXISTS cases_selected_action_id_fkey;
DROP TABLE IF EXISTS corrective_actions;
DROP TABLE IF EXISTS scenarios;
DROP TABLE IF EXISTS diagnostics;
DROP TABLE IF EXISTS kpi_snapshots;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS cases;
DROP TABLE IF EXISTS kpi;
