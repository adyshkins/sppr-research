import { useEffect, useRef, useState, type ChangeEvent, type ReactNode } from 'react';
import { R08Panel } from './R08Panel';
import { LegacyApp } from './LegacyApp';
import { researchAPI, RESEARCH_URL, type Analysis, type Capabilities, type Catalog, type Detail, type ProfileReport, type RunInfo } from './research-api';
import './research.css';

type Tab = 'overview' | 'profile' | 'runs' | 'r08' | 'legacy';
const fmt = (x: number, digits = 4) => new Intl.NumberFormat('ru-RU', { maximumFractionDigits: digits }).format(x);
const message = (error: unknown) => error instanceof Error ? error.message : 'Неизвестная ошибка';
function ErrorBox({ text }: { text: string }) { return <div className="rw-alert rw-error" role="alert">{text}</div>; }
function Box({ title, children, caption }: { title: string; children: ReactNode; caption?: string }) {
  return <section className="rw-card"><div className="rw-card-heading"><h2>{title}</h2>{caption && <p>{caption}</p>}</div>{children}</section>;
}
function PurposeBadge({ purpose }: { purpose: string }) {
  return <span className={`rw-badge ${purpose === 'software_test' ? 'rw-badge-warning' : ''}`}>{purpose === 'software_test' ? 'Инженерный тест · не научные результаты' : 'Импорт · воспроизведение не проверено'}</span>;
}

export function App() {
  const [tab, setTab] = useState<Tab>('overview');
  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const [capabilities, setCapabilities] = useState<Capabilities | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true; setLoading(true); setError('');
    Promise.all([researchAPI.catalog(), researchAPI.capabilities()]).then(([c, s]) => {
      if (!Array.isArray(c.series) || !Array.isArray(s.blockers)) throw new Error('Неожиданный формат ответа. Проверьте версию research-api.');
      if (active) { setCatalog(c); setCapabilities(s); }
    }).catch(err => { if (active) { setCatalog(null); setCapabilities(null); setError(message(err)); } })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [retry]);
  return <div className="rw-app">
    <header className="rw-header"><div className="rw-header-inner">
      <a href="#main" className="rw-skip">Перейти к содержимому</a>
      <div className="rw-brand"><span className="rw-brand-mark" aria-hidden="true">П</span><div><p className="rw-eyebrow">СППР · исследовательский проект</p><h1>Контур управления</h1></div></div>
      <div className="rw-header-meta"><span className="rw-version">{capabilities?.version || '0.3.0-dev'}</span><span>Адышкин С. С.</span></div>
    </div></header>
    <div className="rw-nav-wrap"><nav className="rw-nav" aria-label="Разделы приложения">
      {([['overview', 'Обзор и методика'], ['r08', 'Расчётный стенд R08'], ['profile', 'Внешний профиль E11'], ['runs', 'Результаты и анализ'], ['legacy', 'Прежний MVP']] as [Tab, string][]).map(([id, title]) => <button key={id} aria-current={tab === id ? 'page' : undefined} className={tab === id ? 'rw-tab active' : 'rw-tab'} onClick={() => setTab(id)}>{title}</button>)}
    </nav></div>
    <main id="main" className="rw-main">
      <div className="rw-status"><span className="rw-status-dot" aria-hidden="true" /><div><strong>{capabilities?.r08_ready ? 'Подключено исходное ядро R08 (версия 0.8)' : 'Проверка подключения расчётного ядра'}</strong><p>R08 отделён от F10/E11. Воспроизведение итоговых серий статьи пока не подтверждено.</p></div></div>
      {error && <div className="rw-error-row"><ErrorBox text={error} /><button className="rw-btn" onClick={() => setRetry(r => r + 1)}>Повторить соединение</button></div>}
      {loading && <p role="status" className="rw-muted">Проверка соединения с исследовательским API…</p>}
      {tab === 'overview' && <Overview catalog={catalog} capabilities={capabilities} />}
      {tab === 'r08' && <R08Panel />}
      {tab === 'profile' && <ProfilePanel />}
      {tab === 'runs' && <RunsPanel />}
      {tab === 'legacy' && <><div className="rw-alert">Сохранён прежний демонстрационный процесс отдельных случаев. Он использует отдельный сервер на порту 18080 и не является расчётным стендом статьи.</div><LegacyApp /></>}
      <footer className="rw-footer"><span>Исследовательский API: {RESEARCH_URL}</span><span>Модельные потери ≠ денежный эффект</span></footer>
    </main>
  </div>;
}

function Overview({ catalog, capabilities }: { catalog: Catalog | null; capabilities: Capabilities | null }) {
  return <>
    <div className="rw-intro"><p className="rw-eyebrow">Рабочее пространство исследования</p><h2>От входного профиля к проверяемому результату</h2><p>Дизайн F10 и E11 отделён от загруженных данных. Ни наличие файла, ни успешная проверка его структуры не подтверждают воспроизведение статьи.</p></div>
    <div className="rw-grid-2">{catalog?.series.map(s => <section key={s.id} className="rw-series-card"><div className="rw-between"><span className="rw-series-id">{s.id}</span><span className="rw-badge">Дизайн из статьи</span></div><h3>{s.id === 'F10' ? 'Синтетическая итоговая проверка' : 'Внешняя ретроспективная проверка'}</h3><div className="rw-metrics"><div><strong>{s.expected_episodes}</strong><span>эпизодов предусмотрено</span></div><div><strong>{s.horizon_days}</strong><span>суток в эпизоде</span></div><div><strong>{s.replications}</strong><span>парных повторения</span></div></div><p>{s.groups.join(' · ')} · {s.policies.length} политик</p><p className="rw-muted">Первичный анализ: {s.analysis_groups.join(', ')}; равные веса групп.</p><button className="rw-btn" disabled title="Требуется исходный стенд и регрессионная проверка">Запуск недоступен</button></section>)}</div>
    <Box title="Политики управления" caption="Обозначения из последней редакции статьи. Пока это спецификация, а не реализация контроллера.">
      {!catalog ? <p className="rw-muted">Описание загрузится после подключения к API.</p> : <div className="rw-policy-list">{catalog.policies.map(p => <div className="rw-policy" key={p.id}><code>{p.id}</code><div><h3>{p.label}</h3><p>{p.description}</p></div><span className="rw-delay">{p.delay_days === 0 ? 'без ожидания' : `${p.delay_days} суток`}</span></div>)}</div>}
    </Box>
    <Box title="Что ещё нужно для воспроизведения F10/E11">
      {capabilities?.blockers.map((x, i) => <p className="rw-checkline" key={x}><span>{String(i + 1).padStart(2, '0')}</span>{x}</p>)}
      <p className="rw-muted">Ядро R08 уже включено отдельно. Для итоговых серий после получения их исходников: тесты ядра, сопоставление F10, адаптер внешнего входа, журналы и replay E11. Подмена прежнего расчёта новым симулятором не выполняется.</p>
    </Box>
    {catalog && <Box title="Основание спецификации"><p>{catalog.source}</p><p className="rw-hash">SHA-256 документа: {catalog.source_sha256}</p>{catalog.notes.map(n => <p className="rw-muted" key={n}>{n}</p>)}</Box>}
  </>;
}

function ProfilePanel() {
  const [report, setReport] = useState<ProfileReport | null>(null);
  const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [name, setName] = useState('');
  const active = useRef(true);
  useEffect(() => { active.current = true; return () => { active.current = false; }; }, []);
  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]; event.target.value = ''; if (!file) return;
    setReport(null); setError(''); setName(file.name);
    if (file.size > 4 * 1024 * 1024) { setError('Максимальный размер CSV — 4 МиБ.'); return; }
    setBusy(true);
    try { const result = await researchAPI.profile(file); if (active.current) setReport(result); }
    catch (err) { if (active.current) setError(message(err)); }
    finally { if (active.current) setBusy(false); }
  }
  return <><div className="rw-intro"><p className="rw-eyebrow">Входные данные</p><h2>Проверка спросового профиля E11</h2><p>Исходные байты не изменяются: без повторного сглаживания, нормирования и клиппинга. Этот экран не передаёт данные контроллеру.</p></div>
    <Box title="external_profiles_240d.csv" caption="240 последовательных дат · 4 окна по 60 суток · material_code 264 и 486">
      <label className="rw-upload"><span className="rw-upload-title">{busy ? 'Проверка файла…' : 'Выбрать CSV для проверки'}</span><span>Файл отправляется только в ваш локальный исследовательский API, до 4 МиБ.</span><input aria-label="Выбрать CSV E11" type="file" accept=".csv,text/csv" disabled={busy} onChange={upload} /></label>
      {name && <p className="rw-muted">Файл: {name}</p>}{error && <ErrorBox text={error} />}
    </Box>
    {report && <Box title="Результат проверки"><div className="rw-grid-2"><div className="rw-summary"><span>Структура профиля</span><strong>{report.valid_structure ? 'Соответствует контракту' : 'Обнаружены ошибки'}</strong></div><div className="rw-summary"><span>Байтовый хэш из статьи</span><strong>{report.reference_match ? 'Совпадает' : 'Не совпадает'}</strong></div></div>
      <div className="rw-alert">{report.eligible_for_reference ? 'Структура и байтовый хэш соответствуют зафиксированному входу E11. Это не проверка расчётов модели.' : 'Этот файл не подтверждён как зафиксированный вход E11. Даже корректной структуры недостаточно без совпадения SHA-256.'}</div>
      <p className="rw-hash">Получен: {report.sha256}</p><p className="rw-hash">Ожидается: {report.expected_sha256}</p>
      {report.issues.map((issue, i) => <p key={`${i}-${issue}`} className="rw-issue">{issue}</p>)}
      {report.windows.length > 0 && <div className="rw-table-wrap"><table><caption>Средние значения загруженного профиля, не прогноз контроллера</caption><thead><tr><th>Окно</th><th>Период</th><th>Строк</th><th>Спрос A</th><th>Спрос B</th></tr></thead><tbody>{report.windows.map(w => <tr key={w.window}><th>{w.window}</th><td>{w.start} — {w.end}</td><td>{w.count}</td><td>{fmt(w.mean_a, 2)}</td><td>{fmt(w.mean_b, 2)}</td></tr>)}</tbody></table></div>}
      <p className="rw-muted">{report.notice}</p>
    </Box>}
  </>;
}

function RunsPanel() {
  const [runs, setRuns] = useState<RunInfo[]>([]); const [detail, setDetail] = useState<Detail | null>(null);
  const [error, setError] = useState(''); const [busy, setBusy] = useState(false); const [loading, setLoading] = useState(true);
  const [analysis, setAnalysis] = useState<Analysis | null>(null); const [seed, setSeed] = useState('42');
  const active = useRef(true);
  useEffect(() => { active.current = true; void refresh(); return () => { active.current = false; }; }, []);
  async function refresh() {
    setLoading(true);
    try { const result = await researchAPI.list(); if (!Array.isArray(result.items)) throw new Error('Неверный формат списка наборов.'); if (active.current) { setRuns(result.items); setError(''); } }
    catch (err) { if (active.current) setError(message(err)); }
    finally { if (active.current) setLoading(false); }
  }
  async function open(id: string) {
    setBusy(true); setError(''); setAnalysis(null); setDetail(null);
    try { const result = await researchAPI.detail(id); if (active.current) setDetail(result); }
    catch (err) { if (active.current) setError(message(err)); }
    finally { if (active.current) setBusy(false); }
  }
  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]; event.target.value = ''; if (!file) return;
    setError(''); if (file.size > 8 * 1024 * 1024) { setError('Максимальный размер JSON — 8 МиБ.'); return; }
    setBusy(true); setAnalysis(null); setDetail(null);
    try { const response = await researchAPI.import(file); if (active.current) { await refresh(); await open(response.run.id); } }
    catch (err) { if (active.current) setError(message(err)); }
    finally { if (active.current) setBusy(false); }
  }
  async function analyze() {
    if (!detail) return;
    if (!/^-?\d+$/.test(seed) || !Number.isSafeInteger(Number(seed))) { setError('Seed должен быть целым числом в безопасном диапазоне JavaScript.'); return; }
    setBusy(true); setAnalysis(null); setError('');
    try { const result = await researchAPI.analyze(detail.id, Number(seed)); if (active.current) setAnalysis(result); }
    catch (err) { if (active.current) setError(message(err)); }
    finally { if (active.current) setBusy(false); }
  }
  return <><div className="rw-intro"><p className="rw-eyebrow">Доказательная база</p><h2>Импорт и анализ результатов</h2><p>Здесь отображаются только загруженные наборы. Результаты статьи не подставляются автоматически и не выдаются за новый расчёт.</p></div>
    <Box title="Наборы экспериментов" caption="Новый формат обмена sppr-results-v1. Преобразователь исходных архивов ещё не реализован.">
      <div className="rw-actions"><label className="rw-file-btn">Импортировать JSON<input type="file" accept=".json,application/json" aria-label="Импортировать набор JSON" disabled={busy} onChange={upload} /></label><button className="rw-btn" disabled={loading || busy} onClick={refresh}>Обновить список</button><a className="rw-link" href={`${RESEARCH_URL}/api/research/openapi.json`} target="_blank" rel="noreferrer">Контракт API</a></div>
      {error && <ErrorBox text={error} />}{busy && <p role="status" className="rw-muted">Обработка запроса…</p>}{loading && <p role="status" className="rw-muted">Загрузка списка…</p>}
      {!loading && !runs.length && <div className="rw-empty"><h3>Результаты пока не загружены</h3><p>Нужны полный набор сводок, идентификаторы конфигурации и парных внешних лент. Пустое хранилище не заменяется демонстрационными числами.</p></div>}
      {runs.length > 0 && <div className="rw-run-list">{runs.map(run => <button key={run.id} className={`rw-run ${detail?.id === run.id ? 'selected' : ''}`} disabled={busy} onClick={() => open(run.id)}><div><strong>{run.series}</strong><span>{run.episode_count} эпизода · {run.engine_version}</span><code>{run.id.slice(0, 16)}…</code></div><PurposeBadge purpose={run.purpose} /></button>)}</div>}
    </Box>
    {detail && <>
      <Box title={`${detail.series} · импортированные сводки`}>
        <PurposeBadge purpose={detail.purpose} />
        <p className="rw-hash">SHA-256 загруженного файла: {detail.id}</p>
        <div className="rw-grid-2"><div className="rw-summary"><span>Эпизодов</span><strong>{detail.validation.episode_count}</strong></div><div className="rw-summary"><span>Согласованных групп / повторов</span><strong>{detail.validation.paired_blocks}</strong></div></div>
        {detail.validation.warnings.map(w => <p className="rw-muted" key={w}>{w}</p>)}
        <details className="rw-details"><summary>Манифест и заявленные контрольные суммы</summary><pre>{JSON.stringify(detail.manifest, null, 2)}</pre></details>
        <a className="rw-link" href={`${RESEARCH_URL}/api/research/runs/${detail.id}/download`}>Сохранить исходный JSON без изменений</a>
      </Box>
      <Box title="Сводные показатели" caption="Среднее по 24 повторениям внутри каждой группы. Серии F10 и E11 не объединяются.">
        <div className="rw-table-wrap"><table><thead><tr><th>Группа</th><th>Политика</th><th>Повторов</th><th>Суммарная потеря / эпизод</th><th>Прогнозных шагов / эпизод</th></tr></thead><tbody>{detail.aggregates.map(row => <tr key={`${row.group}/${row.policy}`}><th>{row.group}</th><td><code>{row.policy}</code></td><td>{row.count}</td><td>{fmt(row.mean_loss)}</td><td>{fmt(row.mean_forecast_steps, 1)}</td></tr>)}</tbody></table></div>
      </Box>
      <Box title="Парный блоковый анализ" caption="Новый анализатор сводок: 24 блока, 20 000 перевыборок, четыре контраста, интервалы 98,75%.">
        <p className="rw-muted">Сначала разности усредняются по четырём группам с равными весами. Seed 42 — параметр этого нового анализа, не восстановленный seed исходного исследования.</p>
        <div className="rw-actions"><label className="rw-seed">Seed анализа<input inputMode="numeric" value={seed} disabled={busy} onChange={e => { setSeed(e.target.value); setAnalysis(null); }} /></label><button className="rw-btn rw-btn-primary" disabled={busy} onClick={analyze}>Рассчитать контрасты</button><button className="rw-btn" disabled title="Нужен исходный контроллер">Replay недоступен</button></div>
        {analysis && <><div className="rw-table-wrap"><table><thead><tr><th>Контраст</th><th>Метрика</th><th>Среднее</th><th>Интервал 98,75%</th><th>Содержит ноль</th></tr></thead><tbody>{analysis.contrasts.map((c, i) => <tr key={i}><th>{c.name}</th><td>{c.metric === 'total_loss' ? 'Потери' : 'Прогнозные шаги'}</td><td>{fmt(c.mean)}</td><td>[{fmt(c.lower)}; {fmt(c.upper)}]</td><td>{c.contains_zero ? 'Да' : 'Нет'}</td></tr>)}</tbody></table></div><p className="rw-muted">{analysis.notice}</p><details className="rw-details"><summary>24 блоковые разности и параметры анализа</summary><pre>{JSON.stringify(analysis, null, 2)}</pre></details></>}
      </Box>
    </>}
  </>;
}
