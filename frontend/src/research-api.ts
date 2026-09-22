export const RESEARCH_URL = (import.meta.env.VITE_RESEARCH_API_BASE_URL || (window.location.port === '5173' ? 'http://127.0.0.1:18081' : window.location.origin)).replace(/\/$/, '');
export type Purpose = 'research_import' | 'software_test';
export interface Capabilities {
  version: string; r08_ready?: boolean; can_run_r08?: boolean; can_replay_r08?: boolean; engine_ready: boolean; engine_status: string;
  can_import: boolean; can_validate_profile: boolean; can_analyze_imports: boolean;
  can_run_experiments: boolean; can_replay: boolean; article_reproduction_verified: boolean;
  blockers: string[]; notice: string;
}
export interface SeriesSpec { id: string; groups: string[]; analysis_groups: string[]; policies: string[]; replications: number; horizon_days: number; expected_episodes: number }
export interface PolicySpec { id: string; label: string; delay_days: number; description: string }
export interface Catalog { version: string; source: string; source_sha256: string; series: SeriesSpec[]; policies: PolicySpec[]; notes: string[] }
export interface RunInfo { id: string; series: string; purpose: Purpose; engine_version: string; episode_count: number; imported_at: string; status: 'imported_unverified' }
export interface Validation { scope: string; checks: number; episode_count: number; paired_blocks: number; reference_input_match?: boolean; warnings: string[] }
export interface Detail {
  id: string; series: string; purpose: Purpose; status: string;
  manifest: { engine_version: string; source_sha256: string; config_sha256: string; input_sha256: string; plan_sha256: string; notes?: string };
  validation: Validation;
  aggregates: { group: string; policy: string; count: number; mean_loss: number; mean_forecast_steps: number }[];
}
export interface ProfileReport {
  sha256: string; expected_sha256: string; rows: number; valid_structure: boolean;
  reference_match: boolean; eligible_for_reference: boolean; issues: string[]; notice: string;
  windows: { window: string; start: string; end: string; count: number; mean_a: number; mean_b: number }[];
}
export interface Analysis {
  series: string; purpose: Purpose; seed: number; resamples: number; confidence: number; family_confidence: number;
  block_count: number; analysis_groups: string[]; analyzer_version: string; runtime: string; notice: string;
  contrasts: { name: string; metric: string; mean: number; lower: number; upper: number; contains_zero: boolean; block_differences: number[] }[];
}
export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const abort = new AbortController();
  const timeout = window.setTimeout(() => abort.abort(), 30000);
  try {
    const response = await fetch(`${RESEARCH_URL}/api/research${path}`, { ...init, signal: abort.signal });
    const payload: unknown = await response.json();
    if (!response.ok) {
      const error = payload as { error?: { message?: string; code?: string } };
      throw new Error(error.error?.message || `Ошибка API: HTTP ${response.status}`);
    }
    return payload as T;
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw new Error('Сервер не ответил за 30 секунд. Проверьте, запущен ли research-api.');
    if (err instanceof TypeError) throw new Error(`Нет соединения с ${RESEARCH_URL}. Запустите research-api и проверьте разрешённый origin.`);
    throw err;
  } finally { window.clearTimeout(timeout); }
}
export const researchAPI = {
  capabilities: () => request<Capabilities>('/capabilities'),
  catalog: () => request<Catalog>('/spec'),
  list: () => request<{ items: RunInfo[] }>('/runs'),
  detail: (id: string) => request<Detail>(`/runs/${encodeURIComponent(id)}`),
  import: (file: File) => request<{ run: RunInfo; validation: Validation }>('/runs', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: file }),
  profile: (file: File) => request<ProfileReport>('/profile/validate', { method: 'POST', headers: { 'Content-Type': 'text/csv' }, body: file }),
  analyze: (id: string, seed: number) => request<Analysis>(`/runs/${encodeURIComponent(id)}/analysis`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ seed }) }),
};
