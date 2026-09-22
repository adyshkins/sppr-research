package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"sppr-prototype/backend/internal/db"
)

type SQLCRepository struct {
	queries *db.Queries
}

func NewSQLCRepository(queries *db.Queries) *SQLCRepository {
	return &SQLCRepository{queries: queries}
}

func (r *SQLCRepository) GetCase(ctx context.Context, id uuid.UUID) (db.Case, error) {
	return r.queries.GetCase(ctx, uuidToPgtype(id))
}

func (r *SQLCRepository) UpdateCaseStatus(ctx context.Context, arg db.UpdateCaseStatusParams) (db.Case, error) {
	return r.queries.UpdateCaseStatus(ctx, arg)
}

func (r *SQLCRepository) CreateDecisionTrace(ctx context.Context, arg db.CreateDecisionTraceParams) (db.DecisionTrace, error) {
	return r.queries.CreateDecisionTrace(ctx, arg)
}

func (r *SQLCRepository) CreateCase(ctx context.Context, arg db.CreateCaseParams) (db.Case, error) {
	return r.queries.CreateCase(ctx, arg)
}

func (r *SQLCRepository) CreateEvent(ctx context.Context, arg db.CreateEventParams) (db.Event, error) {
	return r.queries.CreateEvent(ctx, arg)
}

func (r *SQLCRepository) CreateDiagnostic(ctx context.Context, arg db.CreateDiagnosticParams) (db.Diagnostic, error) {
	return r.queries.CreateDiagnostic(ctx, arg)
}

func (r *SQLCRepository) CreateScenario(ctx context.Context, arg db.CreateScenarioParams) (db.Scenario, error) {
	return r.queries.CreateScenario(ctx, arg)
}

func (r *SQLCRepository) CreateCorrectiveAction(ctx context.Context, arg db.CreateCorrectiveActionParams) (db.CorrectiveAction, error) {
	return r.queries.CreateCorrectiveAction(ctx, arg)
}

func (r *SQLCRepository) ClearSelectedCorrectiveActionsByCase(ctx context.Context, caseID uuid.UUID) error {
	return r.queries.ClearSelectedCorrectiveActionsByCase(ctx, uuidToPgtype(caseID))
}

func (r *SQLCRepository) MarkCorrectiveActionSelected(ctx context.Context, id uuid.UUID) (db.CorrectiveAction, error) {
	return r.queries.SelectCorrectiveAction(ctx, uuidToPgtype(id))
}

func (r *SQLCRepository) UpdateCaseSelectedAction(ctx context.Context, arg db.UpdateCaseSelectedActionParams) (db.Case, error) {
	return r.queries.UpdateCaseSelectedAction(ctx, arg)
}

func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{
		Bytes: id,
		Valid: true,
	}
}
