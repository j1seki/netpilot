import { useId, type ReactNode } from 'react';
import { CheckCircle2, PauseCircle, Power } from 'lucide-react';

export function NetMergeControl({ running, busy, disabled, statusLabel, summary, actions, onToggle }: {
  running: boolean;
  busy: boolean;
  disabled?: boolean;
  statusLabel?: string;
  summary?: string;
  actions?: ReactNode;
  onToggle: () => void;
}) {
  const titleId = useId();
  const StateIcon = running ? CheckCircle2 : PauseCircle;

  return <section className="card net-merge-control" data-running={running} aria-labelledby={titleId}>
    <div className="net-merge-control-state">
      <span className="net-merge-control-icon"><StateIcon size={22} aria-hidden="true" /></span>
      <div>
        <div className="net-merge-control-heading">
          <h2 id={titleId}>Net-Merge Virtual Adapter</h2>
          <span aria-live="polite" className={`status-pill ${running && !statusLabel ? 'pass' : 'blocked'}`}>
            {statusLabel ?? (running ? 'ACTIVE · net-merge' : 'STANDBY · System Default')}
          </span>
        </div>
        {summary && <p className="rule-editor-note">{summary}</p>}
      </div>
    </div>
    <div className="net-merge-control-actions">
      {actions}
      <button type="button" className={`btn ${running ? 'btn-danger' : 'btn-emerald'}`} onClick={onToggle} disabled={busy || disabled}>
        <Power size={14} aria-hidden="true" />
        {busy ? (running ? 'Stopping…' : 'Starting…') : (running ? 'Stop Net-Merge' : 'Start Net-Merge')}
      </button>
    </div>
  </section>;
}
