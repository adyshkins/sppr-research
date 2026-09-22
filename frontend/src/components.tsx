import type { ReactNode } from 'react';
import { AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';

type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger';

const statusTone: Record<string, StatusTone> = {
  action_selected: 'info',
  expert_validation: 'warning',
  expert_approved: 'success',
  executed: 'success',
  archived: 'neutral',
  expert_rejected: 'danger',
  expert_returned: 'warning',
};

const toneClass: Record<StatusTone, string> = {
  neutral: 'border-slate-300 bg-slate-100 text-slate-700',
  info: 'border-sky-200 bg-sky-50 text-sky-800',
  success: 'border-emerald-200 bg-emerald-50 text-emerald-800',
  warning: 'border-amber-200 bg-amber-50 text-amber-800',
  danger: 'border-red-200 bg-red-50 text-red-800',
};

export function StatusBadge({ status }: { status: string }) {
  const tone = statusTone[status] ?? 'neutral';
  return (
    <span className={`inline-flex items-center border px-2.5 py-1 text-xs font-semibold ${toneClass[tone]}`} style={{ borderRadius: 999 }}>
      {status.replaceAll('_', ' ')}
    </span>
  );
}

export function Section({
  title,
  subtitle,
  children,
  action,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <section className="panel">
      <div className="flex flex-col gap-3 border-b border-line px-5 py-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-base font-bold text-ink">{title}</h2>
          {subtitle ? <p className="mt-1 text-sm text-graphite">{subtitle}</p> : null}
        </div>
        {action}
      </div>
      <div className="p-5">{children}</div>
    </section>
  );
}

export function EmptyState({ title, text }: { title: string; text: string }) {
  return (
    <div className="subtle-panel flex flex-col items-center justify-center px-5 py-10 text-center">
      <CheckCircle2 className="h-8 w-8 text-moss" aria-hidden="true" />
      <h3 className="mt-3 text-sm font-bold text-ink">{title}</h3>
      <p className="mt-1 max-w-md text-sm text-graphite">{text}</p>
    </div>
  );
}

export function ErrorState({ message }: { message: string }) {
  return (
    <div className="border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800" style={{ borderRadius: 8 }}>
      <div className="flex items-start gap-2">
        <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <span>{message}</span>
      </div>
    </div>
  );
}

export function LoadingState({ text = 'Загрузка данных' }: { text?: string }) {
  return (
    <div className="flex min-h-40 items-center justify-center gap-3 text-sm font-medium text-graphite">
      <Loader2 className="h-5 w-5 animate-spin text-science" aria-hidden="true" />
      {text}
    </div>
  );
}

export function JsonBlock({ value }: { value: unknown }) {
  return (
    <pre className="max-h-64 overflow-auto border border-line bg-slate-950 p-3 text-xs leading-relaxed text-slate-100" style={{ borderRadius: 6 }}>
      {JSON.stringify(value ?? {}, null, 2)}
    </pre>
  );
}
