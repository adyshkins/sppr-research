import { useEffect, useRef, useState } from 'react';
import { RESEARCH_URL, request } from './research-api';

type Summary = { total_loss: number; kpi_loss: number; action_cost: number; days: number; forecasts: number; recovery_executions: number; primary_reasons: Record<string, number> };
type Episode = { plan_index: number; arm: string; summary?: Summary; file_hashes?: Record<string,string>; replay_records?: number };
type Job = { id: string; series: string; status: string; created: string; updated: string; request: { profile: string; repeat: number; arms: string[] }; episodes: Episode[]; replay_status: string; error?: string; plan_sha256: string; engine_sha256: string };
const labels: Record<string,string> = { queued:'В очереди', running:'Выполняется', complete:'Расчёт завершён', failed:'Ошибка', cancelled:'Отменён', interrupted:'Прерван при остановке сервера', verified:'Replay пройден', not_run:'Replay не выполнялся' };
const arms = ['B0','H','EM','ET'];
const armNames: Record<string,string> = { B0:'Контроль по ожиданию', H:'Продолжение плана при отказе', EM:'Исключение по ожиданию', ET:'Исключение по превышению риска' };
const profiles: Record<string,string> = { normal:'Нормальный режим', S:'Нарушение снабжения', C:'Снижение мощности', D:'Рост спроса', DS:'Спрос и снабжение' };
const number = (x: number) => new Intl.NumberFormat('ru-RU',{maximumFractionDigits:6}).format(x);
const post = <T,>(path: string, value: unknown = {}) => request<T>(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(value)});

export function R08Panel(){
 const [jobs,setJobs]=useState<Job[]>([]);const [selected,setSelected]=useState('');
 const [profile,setProfile]=useState('S');const [repeat,setRepeat]=useState('0');const [chosen,setChosen]=useState<string[]>([...arms]);
 const [error,setError]=useState('');const [busy,setBusy]=useState(false);const [loading,setLoading]=useState(true);
 const active=useRef(true);
 async function refresh(){
  try{const data=await request<{items:Job[]}>('/r08/jobs');if(!Array.isArray(data.items))throw new Error('Неверный формат списка R08.');if(active.current){setJobs(data.items);setLoading(false)}}
  catch(e){if(active.current){setError(e instanceof Error?e.message:'Ошибка соединения');setLoading(false)}}
 }
 useEffect(()=>{active.current=true;let timer:number;let stopped=false;
  const poll=async()=>{await refresh();if(!stopped)timer=window.setTimeout(poll,1500)};void poll();
  return()=>{active.current=false;stopped=true;window.clearTimeout(timer)};
 },[]);
 const current=jobs.find(j=>j.id===selected);
 const running=jobs.some(j=>j.status==='queued'||j.status==='running'||j.replay_status==='running');
 async function action(fn:()=>Promise<Job|unknown>){setBusy(true);setError('');try{await fn();await refresh()}catch(e){if(active.current)setError(e instanceof Error?e.message:'Ошибка запроса')}finally{if(active.current)setBusy(false)}}
 function start(){
  if(!/^\d+$/.test(repeat)||Number(repeat)>11||chosen.length===0){setError('Выберите повтор 0–11 и хотя бы один вариант.');return}
  void action(async()=>{const j=await post<Job>('/r08/experiments',{profile,repeat:Number(repeat),arms:chosen});if(active.current)setSelected(j.id)})
 }
 return <>
  <div className="rw-intro"><p className="rw-eyebrow">Расчётное ядро 0.8 · DEV-R08</p><h2>Запуск и воспроизведение эпизодов</h2><p>Здесь запускается исходный Go-стенд из архива. Не имитация ответа API и не подстановка таблиц статьи. Доступны B0/H/EM/ET; политики F/S/A/G и серии F10/E11 этот архив не содержит.</p></div>
  <section className="rw-card"><div className="rw-card-heading"><h2>Параметры запуска</h2><p>Выбор из зафиксированного плана: 5 профилей × 12 повторений × 4 варианта, по 60 суток. Seeds, модель и риск-лимит не меняются.</p></div>
   <div className="rw-r08-controls">
    <label>Профиль<select aria-label="Профиль R08" value={profile} disabled={busy} onChange={e=>setProfile(e.target.value)}>{Object.entries(profiles).map(([v,n])=><option key={v} value={v}>{v} · {n}</option>)}</select></label>
    <label>Номер повтора<input aria-label="Повтор R08" type="number" min="0" max="11" value={repeat} disabled={busy} onChange={e=>setRepeat(e.target.value)}/></label>
   </div>
   <fieldset className="rw-r08-arms"><legend>Варианты на общей внешней траектории</legend>{arms.map(a=><label key={a}><input type="checkbox" checked={chosen.includes(a)} disabled={busy} onChange={e=>setChosen(c=>e.target.checked?[...c,a]:c.filter(x=>x!==a))}/><span><strong>{a}</strong> · {armNames[a]}</span></label>)}</fieldset>
   <div className="rw-alert">EM/ET исследуют явно разрешённое исключение. Их воздействие не получает риск-сертификацию и не управляет реальным оборудованием. Один запуск или replay одновременно.</div>
   <div className="rw-actions"><button className="rw-btn rw-btn-primary" disabled={busy||running||loading} onClick={start}>Запустить R08</button><span className="rw-muted">Новый каталог результатов · исходный архив не перезаписывается</span></div>
   {error&&<div role="alert" className="rw-alert rw-error">{error}</div>}
  </section>
  <section className="rw-card"><div className="rw-card-heading"><h2>Выполненные запуски</h2><p>Это новые вычисления данной установки, а не автоматически загруженные исторические результаты.</p></div>
   {loading&&<p role="status">Подключение к API…</p>}
   {!loading&&!jobs.length&&<div className="rw-empty"><h3>Новых запусков пока нет</h3><p>Выберите профиль и нажмите «Запустить R08».</p></div>}
   <div className="rw-run-list">{jobs.map(j=><button className={`rw-run ${j.id===selected?'selected':''}`} key={j.id} onClick={()=>setSelected(j.id)}><div><strong>{j.request.profile} · повтор {j.request.repeat} · {j.request.arms.join('/')}</strong><span>{labels[j.status]||j.status} · {j.episodes.filter(e=>e.summary).length}/{j.episodes.length} эпизодов</span><code>{j.id.slice(0,16)}…</code></div><span className="rw-badge">DEV-R08</span></button>)}</div>
  </section>
  {current&&<section className="rw-card"><div className="rw-card-heading"><h2>Результат расчёта</h2><p>{labels[current.status]||current.status} · {labels[current.replay_status]||current.replay_status}</p></div>
   {current.error&&<div role="alert" className="rw-alert rw-error">{current.error}</div>}
   <div className="rw-table-wrap"><table><caption>Суммы за 60 суток. Потери нормированы, не выражены в рублях.</caption><thead><tr><th>Вариант</th><th>Потери</th><th>KPI</th><th>Стоимость действий</th><th>RISK_EMPTY</th><th>Исключений исполнено</th><th>Replay</th></tr></thead><tbody>{current.episodes.map(e=><tr key={e.plan_index}><th>{e.arm}</th><td>{e.summary?number(e.summary.total_loss):'—'}</td><td>{e.summary?number(e.summary.kpi_loss):'—'}</td><td>{e.summary?number(e.summary.action_cost):'—'}</td><td>{e.summary?(e.summary.primary_reasons.RISK_EMPTY||0):'—'}</td><td>{e.summary?.recovery_executions??'—'}</td><td>{e.replay_records!==undefined?`${e.replay_records} записей`:'не проверен'}</td></tr>)}</tbody></table></div>
   <div className="rw-actions"><button className="rw-btn rw-btn-primary" disabled={busy||running||current.status!=='complete'} onClick={()=>void action(()=>post(`/r08/jobs/${current.id}/replay`))}>Повторно вычислить протоколы</button><button className="rw-btn" disabled={busy||!(current.status==='running'||current.status==='queued'||current.replay_status==='running')} onClick={()=>void action(()=>post(`/r08/jobs/${current.id}/cancel`))}>Остановить</button></div>
   <p className="rw-muted">Replay пересчитывает решения по сохранённым наблюдениям. Он не подтверждает промышленную адекватность модели и не воспроизводит более поздние серии F10/E11.</p>
   <details className="rw-details"><summary>Первичные файлы и контрольные суммы</summary><p className="rw-hash">План: {current.plan_sha256}</p><p className="rw-hash">Исполняемый файл: {current.engine_sha256}</p>{current.episodes.map(e=><div key={e.plan_index}><h3>{e.arm} · ep{String(e.plan_index).padStart(3,'0')}</h3>{Object.entries(e.file_hashes||{}).map(([name,sha])=><p className="rw-hash" key={name}><a className="rw-link" href={`${RESEARCH_URL}/api/research/r08/jobs/${current.id}/episodes/${e.plan_index}/files/${name}`}>{name}</a><br/>{sha}</p>)}</div>)}</details>
  </section>}
 </>;
}
