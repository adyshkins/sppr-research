export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:18080';

export type CaseStatus =
  | 'registered'
  | 'normalized'
  | 'deviation_confirmed'
  | 'diagnosed'
  | 'risk_assessed'
  | 'alternatives_generated'
  | 'action_selected'
  | 'expert_validation'
  | 'expert_approved'
  | 'expert_rejected'
  | 'expert_returned'
  | 'executed'
  | 'archived'
  | string;

export type CaseSummary = {
  id: string;
  status: CaseStatus;
  risk_level: string;
  selected_action_id?: string;
  created_at: string;
  updated_at: string;
};

export type WorkflowRequest = {
  event_id: string;
  product: string;
  plan_demand: number;
  fact_demand: number;
  capacity_load: number;
  forecast_error: number;
  campaign_flag: boolean;
  alpha: number;
};

export type WorkflowResponse = {
  case_id: string;
  final_status: string;
  top_cause_code: string;
  selected_action_code: string;
  expected_loss: number;
  var_value: number;
  cvar_value: number;
};

export type Diagnostic = {
  id: string;
  case_id: string;
  hypothesis: string;
  confidence: number;
  details: Record<string, unknown>;
  created_at: string;
};

export type Scenario = {
  id: string;
  case_id: string;
  name: string;
  probability: number;
  loss_vector: {
    kpi_values?: Record<string, number>;
    constraints?: Record<string, number>;
  };
  created_at: string;
};

export type CorrectiveAction = {
  id: string;
  case_id: string;
  code: string;
  description: string;
  action_type: string;
  expected_loss: number;
  var_value: number;
  cvar_value: number;
  constraints_violated: Record<string, unknown>;
  is_selected: boolean;
  created_at: string;
};

export type TraceEntry = {
  id: string;
  case_id: string;
  step: string;
  timestamp: string;
  data_snapshot?: Record<string, unknown>;
  parameters?: Record<string, unknown>;
  risk?: Record<string, unknown>;
  expert_decision?: Record<string, unknown>;
  created_at: string;
};

export type ExpertOutcome = 'approved' | 'rejected' | 'returned';

export type ExpertDecisionRequest = {
  outcome: ExpertOutcome;
  comment: string;
  expert_id: string;
};

export type ValidationResponse = {
  case_id: string;
  status: string;
  outcome?: string;
  comment?: string;
  expert_id?: string;
};

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...options,
    headers: {
      Accept: 'application/json',
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...options.headers,
    },
  });

  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const payload = (await response.json()) as { error?: string; message?: string };
      message = payload.error ?? payload.message ?? message;
    } catch {
      message = response.statusText || message;
    }
    throw new Error(message);
  }

  return (await response.json()) as T;
}

export const api = {
  listCases: (status?: string) =>
    request<CaseSummary[]>(`/api/cases${status ? `?status=${encodeURIComponent(status)}` : ''}`),
  getCase: (id: string) => request<CaseSummary>(`/api/cases/${id}`),
  getTrace: (id: string) => request<TraceEntry[]>(`/api/cases/${id}/trace`),
  getDiagnostics: (id: string) => request<Diagnostic[]>(`/api/cases/${id}/diagnostics`),
  getScenarios: (id: string) => request<Scenario[]>(`/api/cases/${id}/scenarios`),
  getActions: (id: string) => request<CorrectiveAction[]>(`/api/cases/${id}/actions`),
  processWorkflow: (payload: WorkflowRequest) =>
    request<WorkflowResponse>('/api/workflow/process', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  submitValidation: (id: string) =>
    request<ValidationResponse>(`/api/cases/${id}/submit-validation`, { method: 'POST' }),
  expertDecision: (id: string, payload: ExpertDecisionRequest) =>
    request<ValidationResponse>(`/api/cases/${id}/expert-decision`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  execute: (id: string) => request<ValidationResponse>(`/api/cases/${id}/execute`, { method: 'POST' }),
  archive: (id: string) => request<ValidationResponse>(`/api/cases/${id}/archive`, { method: 'POST' }),
};
