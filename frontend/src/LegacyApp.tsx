import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react';
import {
  Archive,
  ArrowLeft,
  Check,
  ClipboardCheck,
  ExternalLink,
  FilePlus2,
  Play,
  RefreshCcw,
  RotateCcw,
  X,
} from 'lucide-react';
import {
  api,
  API_BASE_URL,
  type CaseSummary,
  type CorrectiveAction,
  type Diagnostic,
  type ExpertOutcome,
  type Scenario,
  type TraceEntry,
  type WorkflowRequest,
  type WorkflowResponse,
} from './api';
import { EmptyState, ErrorState, JsonBlock, LoadingState, Section, StatusBadge } from './components';
import { formatDate, formatNumber, formatPercent, humanize, shortId } from './format';

type View = 'dashboard' | 'create' | 'case';

const initialForm: WorkflowRequest = {
  event_id: `evt-ui-${Date.now()}`,
  product: 'A100',
  plan_demand: 1000,
  fact_demand: 1350,
  capacity_load: 0.93,
  forecast_error: 0.18,
  campaign_flag: true,
  alpha: 0.95,
};

const statuses = [
  'all',
  'action_selected',
  'expert_validation',
  'expert_approved',
  'expert_rejected',
  'expert_returned',
  'executed',
  'archived',
];

export function LegacyApp() {
  const [view, setView] = useState<View>('dashboard');
  const [selectedCaseId, setSelectedCaseId] = useState<string | null>(null);

  const openCase = (id: string) => {
    setSelectedCaseId(id);
    setView('case');
  };

  return (
    <div className="min-h-screen bg-paper">
      <header className="border-b border-line bg-white">
        <div className="mx-auto flex max-w-7xl flex-col gap-4 px-4 py-5 sm:px-6 lg:flex-row lg:items-center lg:justify-between lg:px-8">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-science">СППР / внутрифирменное планирование</p>
            <h1 className="mt-1 text-2xl font-bold text-ink">Панель управления управленческими случаями</h1>
            <p className="mt-1 text-sm text-graphite">Backend API: {API_BASE_URL}</p>
          </div>
          <nav className="flex flex-wrap gap-2">
            <button className={view === 'dashboard' ? 'btn-primary' : 'btn-secondary'} onClick={() => setView('dashboard')}>
              <ClipboardCheck className="h-4 w-4" aria-hidden="true" />
              Реестр кейсов
            </button>
            <button className={view === 'create' ? 'btn-primary' : 'btn-secondary'} onClick={() => setView('create')}>
              <FilePlus2 className="h-4 w-4" aria-hidden="true" />
              Новый кейс
            </button>
          </nav>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
        {view === 'dashboard' ? <Dashboard onOpenCase={openCase} /> : null}
        {view === 'create' ? <CreateCase onCreated={openCase} /> : null}
        {view === 'case' && selectedCaseId ? (
          <CaseDetail id={selectedCaseId} onBack={() => setView('dashboard')} />
        ) : null}
      </main>
    </div>
  );
}

function Dashboard({ onOpenCase }: { onOpenCase: (id: string) => void }) {
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [status, setStatus] = useState('all');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadCases = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setCases(await api.listCases(status === 'all' ? undefined : status));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось загрузить список кейсов');
    } finally {
      setLoading(false);
    }
  }, [status]);

  useEffect(() => {
    void loadCases();
  }, [loadCases]);

  const metrics = useMemo(() => {
    const active = cases.filter((item) => item.status !== 'archived').length;
    const validation = cases.filter((item) => item.status === 'expert_validation').length;
    return { total: cases.length, active, validation };
  }, [cases]);

  return (
    <div className="space-y-5">
      <div className="grid gap-4 md:grid-cols-3">
        <Metric title="Всего в выборке" value={metrics.total} />
        <Metric title="Активные" value={metrics.active} />
        <Metric title="На валидации" value={metrics.validation} />
      </div>

      <Section
        title="Реестр кейсов"
        subtitle="Список управленческих случаев из backend API"
        action={
          <div className="flex flex-wrap gap-2">
            <select className="field h-10 w-56" value={status} onChange={(event) => setStatus(event.target.value)}>
              {statuses.map((item) => (
                <option key={item} value={item}>
                  {item === 'all' ? 'Все статусы' : humanize(item)}
                </option>
              ))}
            </select>
            <button className="btn-secondary" onClick={loadCases}>
              <RefreshCcw className="h-4 w-4" aria-hidden="true" />
              Обновить
            </button>
          </div>
        }
      >
        {error ? <ErrorState message={error} /> : null}
        {loading ? <LoadingState /> : null}
        {!loading && !error && cases.length === 0 ? (
          <EmptyState title="Кейсы не найдены" text="Создайте новый управленческий случай или измените фильтр по статусу." />
        ) : null}
        {!loading && cases.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[820px] border-collapse text-left text-sm">
              <thead>
                <tr className="border-b border-line text-xs uppercase tracking-wide text-graphite">
                  <th className="px-3 py-3 font-bold">ID</th>
                  <th className="px-3 py-3 font-bold">Статус</th>
                  <th className="px-3 py-3 font-bold">Риск</th>
                  <th className="px-3 py-3 font-bold">Выбранное действие</th>
                  <th className="px-3 py-3 font-bold">Создан</th>
                  <th className="px-3 py-3 font-bold">Обновлён</th>
                  <th className="px-3 py-3 font-bold">Открыть</th>
                </tr>
              </thead>
              <tbody>
                {cases.map((item) => (
                  <tr key={item.id} className="border-b border-line last:border-0 hover:bg-slate-50">
                    <td className="px-3 py-3 font-mono text-xs text-ink">{shortId(item.id)}</td>
                    <td className="px-3 py-3">
                      <StatusBadge status={item.status} />
                    </td>
                    <td className="px-3 py-3 text-graphite">{item.risk_level}</td>
                    <td className="px-3 py-3 font-mono text-xs text-graphite">
                      {item.selected_action_id ? shortId(item.selected_action_id) : '—'}
                    </td>
                    <td className="px-3 py-3 text-graphite">{formatDate(item.created_at)}</td>
                    <td className="px-3 py-3 text-graphite">{formatDate(item.updated_at)}</td>
                    <td className="px-3 py-3">
                      <button className="btn-secondary min-h-9 px-3 py-1.5" onClick={() => onOpenCase(item.id)}>
                        <ExternalLink className="h-4 w-4" aria-hidden="true" />
                        Карточка
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </Section>
    </div>
  );
}

function Metric({ title, value }: { title: string; value: number }) {
  return (
    <div className="panel px-5 py-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-graphite">{title}</p>
      <p className="mt-2 text-3xl font-bold text-ink">{value}</p>
    </div>
  );
}

function CreateCase({ onCreated }: { onCreated: (id: string) => void }) {
  const [form, setForm] = useState<WorkflowRequest>(initialForm);
  const [result, setResult] = useState<WorkflowResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const setField = <K extends keyof WorkflowRequest>(key: K, value: WorkflowRequest[K]) => {
    setForm((current) => ({ ...current, [key]: value }));
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      const response = await api.processWorkflow(form);
      setResult(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось создать кейс');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_360px]">
      <Section title="Создание управленческого случая" subtitle="Запуск workflow: регистрация события, диагностика, сценарии, CVaR и выбор действия">
        <form className="grid gap-4 md:grid-cols-2" onSubmit={submit}>
          <Field label="Event ID">
            <input className="field" value={form.event_id} onChange={(event) => setField('event_id', event.target.value)} required />
          </Field>
          <Field label="Продукт">
            <input className="field" value={form.product} onChange={(event) => setField('product', event.target.value)} required />
          </Field>
          <Field label="Плановый спрос">
            <input className="field" type="number" min="1" value={form.plan_demand} onChange={(event) => setField('plan_demand', Number(event.target.value))} required />
          </Field>
          <Field label="Фактический спрос">
            <input className="field" type="number" value={form.fact_demand} onChange={(event) => setField('fact_demand', Number(event.target.value))} required />
          </Field>
          <Field label="Загрузка мощности">
            <input className="field" type="number" step="0.01" value={form.capacity_load} onChange={(event) => setField('capacity_load', Number(event.target.value))} required />
          </Field>
          <Field label="Ошибка прогноза">
            <input className="field" type="number" step="0.01" value={form.forecast_error} onChange={(event) => setField('forecast_error', Number(event.target.value))} required />
          </Field>
          <Field label="Alpha CVaR">
            <input className="field" type="number" min="0" max="0.99" step="0.01" value={form.alpha} onChange={(event) => setField('alpha', Number(event.target.value))} required />
          </Field>
          <label className="mt-6 flex h-11 items-center gap-3">
            <input
              className="h-4 w-4 accent-science"
              type="checkbox"
              checked={form.campaign_flag}
              onChange={(event) => setField('campaign_flag', event.target.checked)}
            />
            <span className="text-sm font-semibold text-ink">Активная промо-кампания</span>
          </label>
          <div className="flex flex-wrap gap-2 md:col-span-2">
            <button className="btn-primary" disabled={loading} type="submit">
              <Play className="h-4 w-4" aria-hidden="true" />
              {loading ? 'Обработка' : 'Запустить workflow'}
            </button>
            <button className="btn-secondary" type="button" onClick={() => setForm({ ...initialForm, event_id: `evt-ui-${Date.now()}` })}>
              <RotateCcw className="h-4 w-4" aria-hidden="true" />
              Сбросить пример
            </button>
          </div>
        </form>
      </Section>

      <aside className="space-y-5">
        <Section title="Результат">
          {error ? <ErrorState message={error} /> : null}
          {!error && !result ? <EmptyState title="Ожидание запуска" text="После успешного POST-запроса здесь появится краткая сводка созданного кейса." /> : null}
          {result ? (
            <div className="space-y-3 text-sm">
              <ResultRow label="Case ID" value={shortId(result.case_id)} />
              <ResultRow label="Статус" value={humanize(result.final_status)} />
              <ResultRow label="Причина" value={humanize(result.top_cause_code)} />
              <ResultRow label="Действие" value={humanize(result.selected_action_code)} />
              <ResultRow label="Expected loss" value={formatNumber(result.expected_loss)} />
              <ResultRow label="VaR" value={formatNumber(result.var_value)} />
              <ResultRow label="CVaR" value={formatNumber(result.cvar_value)} />
              <button className="btn-primary w-full" onClick={() => onCreated(result.case_id)}>
                Открыть карточку
              </button>
            </div>
          ) : null}
        </Section>
      </aside>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label>
      <span className="label">{label}</span>
      {children}
    </label>
  );
}

function ResultRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3 border-b border-line pb-2 last:border-0">
      <span className="text-graphite">{label}</span>
      <span className="text-right font-semibold text-ink">{value}</span>
    </div>
  );
}

function CaseDetail({ id, onBack }: { id: string; onBack: () => void }) {
  const [item, setItem] = useState<CaseSummary | null>(null);
  const [diagnostics, setDiagnostics] = useState<Diagnostic[]>([]);
  const [scenarios, setScenarios] = useState<Scenario[]>([]);
  const [actions, setActions] = useState<CorrectiveAction[]>([]);
  const [trace, setTrace] = useState<TraceEntry[]>([]);
  const [comment, setComment] = useState('Решение подтверждено экспертом');
  const [expertId, setExpertId] = useState('demo-expert');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [caseItem, diagnosticsData, scenariosData, actionsData, traceData] = await Promise.all([
        api.getCase(id),
        api.getDiagnostics(id),
        api.getScenarios(id),
        api.getActions(id),
        api.getTrace(id),
      ]);
      setItem(caseItem);
      setDiagnostics(diagnosticsData);
      setScenarios(scenariosData);
      setActions(actionsData);
      setTrace(traceData);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось загрузить карточку кейса');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    void load();
  }, [load]);

  const runHitl = async (name: string, action: () => Promise<unknown>) => {
    setBusy(name);
    setError(null);
    try {
      await action();
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Операция не выполнена');
    } finally {
      setBusy(null);
    }
  };

  const decide = (outcome: ExpertOutcome) => {
    const defaultComment =
      outcome === 'approved'
        ? 'Решение подтверждено экспертом'
        : outcome === 'rejected'
          ? 'Решение отклонено экспертом'
          : 'Решение возвращено на доработку';
    return runHitl(`decision-${outcome}`, () =>
      api.expertDecision(id, {
        outcome,
        comment: comment.trim() || defaultComment,
        expert_id: expertId.trim() || 'demo-expert',
      }),
    );
  };

  if (loading) {
    return <LoadingState text="Загрузка карточки кейса" />;
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <button className="btn-secondary w-fit" onClick={onBack}>
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          К реестру
        </button>
        <button className="btn-secondary w-fit" onClick={load}>
          <RefreshCcw className="h-4 w-4" aria-hidden="true" />
          Обновить карточку
        </button>
      </div>

      {error ? <ErrorState message={error} /> : null}

      {item ? (
        <Section title="Карточка кейса" subtitle={item.id}>
          <div className="grid gap-4 md:grid-cols-4">
            <SummaryBox label="Статус" value={<StatusBadge status={item.status} />} />
            <SummaryBox label="Уровень риска" value={item.risk_level} />
            <SummaryBox label="Создан" value={formatDate(item.created_at)} />
            <SummaryBox label="Обновлён" value={formatDate(item.updated_at)} />
          </div>
        </Section>
      ) : null}

      <Section title="Human-in-the-loop" subtitle="Переходы статусов выполняются через backend FSM и фиксируются в протоколе П">
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(300px,420px)]">
          <div className="flex flex-wrap gap-2">
            <button className="btn-secondary" disabled={!!busy} onClick={() => runHitl('submit', () => api.submitValidation(id))}>
              <ClipboardCheck className="h-4 w-4" aria-hidden="true" />
              submit-validation
            </button>
            <button className="btn-primary" disabled={!!busy} onClick={() => decide('approved')}>
              <Check className="h-4 w-4" aria-hidden="true" />
              approved
            </button>
            <button className="btn-danger" disabled={!!busy} onClick={() => decide('rejected')}>
              <X className="h-4 w-4" aria-hidden="true" />
              rejected
            </button>
            <button className="btn-secondary" disabled={!!busy} onClick={() => decide('returned')}>
              <RotateCcw className="h-4 w-4" aria-hidden="true" />
              returned
            </button>
            <button className="btn-secondary" disabled={!!busy} onClick={() => runHitl('execute', () => api.execute(id))}>
              <Play className="h-4 w-4" aria-hidden="true" />
              execute
            </button>
            <button className="btn-secondary" disabled={!!busy} onClick={() => runHitl('archive', () => api.archive(id))}>
              <Archive className="h-4 w-4" aria-hidden="true" />
              archive
            </button>
          </div>
          <div className="grid gap-3">
            <Field label="Expert ID">
              <input className="field" value={expertId} onChange={(event) => setExpertId(event.target.value)} />
            </Field>
            <Field label="Комментарий эксперта">
              <textarea className="textarea" value={comment} onChange={(event) => setComment(event.target.value)} />
            </Field>
          </div>
        </div>
      </Section>

      <div className="grid gap-5 xl:grid-cols-2">
        <Diagnostics diagnostics={diagnostics} />
        <Scenarios scenarios={scenarios} />
      </div>
      <Actions actions={actions} />
      <Trace trace={trace} />
    </div>
  );
}

function SummaryBox({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="subtle-panel px-4 py-3">
      <p className="text-xs font-semibold uppercase tracking-wide text-graphite">{label}</p>
      <div className="mt-2 text-sm font-bold text-ink">{value}</div>
    </div>
  );
}

function Diagnostics({ diagnostics }: { diagnostics: Diagnostic[] }) {
  return (
    <Section title="Diagnostics" subtitle="Причинные гипотезы и уверенность">
      <div className="space-y-3">
        {diagnostics.map((item) => (
          <div className="subtle-panel p-4" key={item.id}>
            <div className="flex items-center justify-between gap-3">
              <h3 className="font-bold text-ink">{humanize(item.hypothesis)}</h3>
              <span className="font-semibold text-science">{formatPercent(item.confidence)}</span>
            </div>
            <div className="mt-3 h-2 overflow-hidden bg-slate-200" style={{ borderRadius: 999 }}>
              <div className="h-full bg-science" style={{ width: `${Math.min(item.confidence * 100, 100)}%` }} />
            </div>
            <div className="mt-3">
              <JsonBlock value={item.details} />
            </div>
          </div>
        ))}
      </div>
    </Section>
  );
}

function Scenarios({ scenarios }: { scenarios: Scenario[] }) {
  return (
    <Section title="Scenarios" subtitle="Сценарные вероятности и компоненты потерь">
      <div className="space-y-3">
        {scenarios.map((item) => (
          <div className="subtle-panel p-4" key={item.id}>
            <div className="flex items-center justify-between gap-3">
              <h3 className="font-bold text-ink">{humanize(item.name)}</h3>
              <span className="font-semibold text-science">{formatPercent(item.probability)}</span>
            </div>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <JsonBlock value={item.loss_vector.kpi_values ?? {}} />
              <JsonBlock value={item.loss_vector.constraints ?? {}} />
            </div>
          </div>
        ))}
      </div>
    </Section>
  );
}

function Actions({ actions }: { actions: CorrectiveAction[] }) {
  return (
    <Section title="Actions" subtitle="Корректирующие воздействия, ранжированные backend CVaR-модулем">
      <div className="grid gap-3 lg:grid-cols-2">
        {actions.map((item) => (
          <article className={`border p-4 ${item.is_selected ? 'border-science bg-sky-50' : 'border-line bg-white'}`} style={{ borderRadius: 8 }} key={item.id}>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 className="font-bold text-ink">{humanize(item.code)}</h3>
                <p className="mt-1 text-sm text-graphite">{item.description}</p>
              </div>
              {item.is_selected ? <span className="border border-science bg-white px-2.5 py-1 text-xs font-bold text-science">selected</span> : null}
            </div>
            <div className="mt-4 grid grid-cols-3 gap-3 text-sm">
              <ResultRow label="E[L]" value={formatNumber(item.expected_loss)} />
              <ResultRow label="VaR" value={formatNumber(item.var_value)} />
              <ResultRow label="CVaR" value={formatNumber(item.cvar_value)} />
            </div>
            <div className="mt-3">
              <JsonBlock value={item.constraints_violated} />
            </div>
          </article>
        ))}
      </div>
    </Section>
  );
}

function Trace({ trace }: { trace: TraceEntry[] }) {
  return (
    <Section title="Trace / протокол П" subtitle="Хронология workflow, смен статусов и экспертных решений">
      <div className="space-y-3">
        {trace.map((item, index) => (
          <div className="grid gap-3 border-l-2 border-line pl-4 md:grid-cols-[180px_minmax(0,1fr)]" key={item.id}>
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-graphite">Шаг {index + 1}</p>
              <p className="mt-1 text-sm text-graphite">{formatDate(item.timestamp)}</p>
            </div>
            <div className="subtle-panel p-4">
              <h3 className="font-bold text-ink">{humanize(item.step)}</h3>
              <div className="mt-3 grid gap-3 lg:grid-cols-2">
                <JsonBlock value={item.data_snapshot ?? {}} />
                <JsonBlock
                  value={{
                    risk: item.risk ?? {},
                    expert_decision: item.expert_decision ?? {},
                  }}
                />
              </div>
            </div>
          </div>
        ))}
      </div>
    </Section>
  );
}
