import { useEffect, useRef, useState } from 'react';
import { Activity, AlertCircle, ArrowRight, Cable, Network, Route, X } from 'lucide-react';
import { api, ProcessInfo, ProcessInspection } from '../api/wails';
import { ProcessIcon } from './ProcessIcon';
import './AppInspection.css';

const clock = new Intl.DateTimeFormat(undefined, { hour: 'numeric', minute: '2-digit', second: '2-digit' });

function AddressList({ values, empty }: { values: string[]; empty: string }) {
  return values.length ? <ul className="inspection-addresses">
    {values.map(value => <li key={value}><code>{value}</code></li>)}
  </ul> : <span className="inspection-muted">{empty}</span>;
}

export function AppInspection({ process, onClose }: { process: ProcessInfo; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [snapshot, setSnapshot] = useState<ProcessInspection | null>(null);
  const [error, setError] = useState('');
  const [view, setView] = useState<'sockets' | 'activity'>('sockets');

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const data = await api.inspectApplication(process, controller.signal);
        if (controller.signal.aborted) return;
        setSnapshot(data);
        setError('');
      } catch (err) {
        if (controller.signal.aborted) return;
        setError(err instanceof Error ? err.message : String(err));
      }
      timer = setTimeout(refresh, 2000);
    };
    void refresh();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [process]);

  useEffect(() => {
    if (dialog.current && !dialog.current.open) dialog.current.showModal();
  }, []);

  return (
    <dialog ref={dialog} className="app-inspection" aria-labelledby="app-inspection-title" onClose={onClose}>
      <header className="inspection-header">
        <ProcessIcon pid={process.pid} executable={process.executable} />
        <div className="inspection-identity">
          <div className="inspection-title">
            <h2 id="app-inspection-title">{process.name}</h2>
            <span className="inspection-pid font-mono">PID {process.pid}</span>
          </div>
          {process.executable && <p className="inspection-path font-mono" title={process.executable}>{process.executable}</p>}
        </div>
        <button type="button" className="btn btn-secondary inspection-close" aria-label="Close inspection" title="Close (Esc)"
          onClick={() => dialog.current?.close()}><X size={17} aria-hidden="true" /></button>
      </header>

      <div className="inspection-body">
        {error && <p className="inspection-alert" role="alert">
          <AlertCircle size={16} aria-hidden="true" />
          <span>{error}{snapshot && ' Showing the last successful snapshot.'}</span>
        </p>}
        {!snapshot && !error && <p className="inspection-empty" role="status">Reading process connections…</p>}
        {snapshot && <>
          <dl className="inspection-summary">
            <div>
              <dt><Cable size={14} aria-hidden="true" /> Open sockets</dt>
              <dd className="tabular-nums">{snapshot.sockets.length || (snapshot.socketError ? '—' : 0)}</dd>
            </div>
            <div>
              <dt><Activity size={14} aria-hidden="true" /> Forwarded events</dt>
              <dd className="tabular-nums">{snapshot.activityEnabled ? snapshot.activity.length : '—'}</dd>
            </div>
            <div>
              <dt><Route size={14} aria-hidden="true" /> Assigned route</dt>
              <dd>
                {snapshot.process.assignedNic || '—'}
                {snapshot.process.ruleMatched && <span className="inspection-muted">Routing rule matched</span>}
              </dd>
            </div>
          </dl>

          <div className="inspection-grid">
            <section className="inspection-connections" aria-label="Process connections and activity">
              <div className="inspection-switcher" role="group" aria-label="Inspection view">
                <button type="button" className="tab-btn" aria-pressed={view === 'sockets'} onClick={() => setView('sockets')}>
                  <Cable size={14} aria-hidden="true" /> Open sockets
                </button>
                <button type="button" className="tab-btn" aria-pressed={view === 'activity'} onClick={() => setView('activity')}>
                  <Activity size={14} aria-hidden="true" /> Forwarded activity
                </button>
              </div>
              {view === 'sockets' ? (
                <div className="inspection-view netpilot-enter" key="sockets">
                  {snapshot.socketError && <p className="inspection-help">{snapshot.socketError}</p>}
                  {snapshot.sockets.length > 0 ? <div className="inspection-table-scroll" role="region" aria-label="Open sockets" tabIndex={0}>
                    <table className="data-table inspection-table inspection-table-sockets">
                      <thead><tr><th scope="col">Protocol</th><th scope="col">Endpoint pair</th></tr></thead>
                      <tbody>{snapshot.sockets.map(({ protocol, local, remote }, index) => (
                        <tr key={`${protocol}-${local}-${remote}-${index}`}>
                          <td><span className="inspection-protocol">{protocol}</span></td>
                          <td>
                            <div className="inspection-endpoint font-mono"><span className="sr-only">Local </span>{local}</div>
                            <div className="inspection-endpoint inspection-remote font-mono"><ArrowRight size={12} aria-hidden="true" />
                              <span className="sr-only">Remote </span>{remote || '—'}
                            </div>
                          </td>
                        </tr>
                      ))}</tbody>
                    </table>
                  </div> : <p className="inspection-empty">
                    {snapshot.socketError ? 'No socket data available.' : 'This process has no visible TCP or UDP sockets.'}
                  </p>}
                </div>
              ) : (
                <div className="inspection-view netpilot-enter" key="activity">
                  {snapshot.activityEnabled ? <>
                    <p className="inspection-help">Destinations and visible UDP DNS questions. HTTPS paths and request bodies are unavailable.</p>
                    {snapshot.activity.length > 0 ? <div className="inspection-table-scroll" role="region" aria-label="Forwarded activity" tabIndex={0}>
                      <table className="data-table inspection-table">
                        <thead><tr><th scope="col">Time</th><th scope="col">Destination</th><th scope="col">Adapter</th></tr></thead>
                        <tbody>{snapshot.activity.map(item => (
                          <tr key={`${item.protocol}-${item.destination}-${item.adapter}-${item.dnsQuestion}`}>
                            <td className="font-mono inspection-time"><time dateTime={item.at}>{clock.format(new Date(item.at))}</time></td>
                            <td>
                              <span className="inspection-protocol">{item.protocol}</span>
                              <div className="inspection-endpoint font-mono">{item.destination}</div>
                              {item.dnsQuestion && <div className="inspection-endpoint inspection-remote font-mono">{item.dnsQuestion}</div>}
                            </td>
                            <td>{item.adapter}</td>
                          </tr>
                        ))}</tbody>
                      </table>
                    </div> : <p className="inspection-empty">No attributed activity observed since opening the inspector.</p>}
                  </> : <p className="inspection-empty">Forwarded activity is available through the in-app NetPilot tunnel or SOCKS proxy.</p>}
                </div>
              )}
            </section>

            <aside className="inspection-dns" aria-labelledby="inspection-dns-title">
              <h3 id="inspection-dns-title"><Network size={15} aria-hidden="true" /> DNS</h3>
              <dl>
                <div><dt>Observed port 53</dt><dd><AddressList values={snapshot.dnsEndpoints} empty="None observed" /></dd></div>
                <div><dt>System resolvers</dt><dd><AddressList values={snapshot.systemResolvers} empty="Unavailable" /></dd></div>
                {snapshot.savedRuleDns && <div>
                  <dt>Saved rule DNS</dt>
                  <dd><AddressList values={[snapshot.savedRuleDns]} empty="" /></dd>
                  <dd className="inspection-muted">The current engine does not enforce this rule value.</dd>
                </div>}
              </dl>
              <p className="inspection-help">The OS resolver and DNS over HTTPS can hide which process made a request.</p>
            </aside>
          </div>
        </>}
      </div>

      <footer className="inspection-footer">
        <span className="inspection-connection-state">
          <span className={`beszel-dot ${error ? 'warning' : snapshot ? 'online' : 'standby'}`} aria-hidden="true" />
          {error ? 'Refresh interrupted' : snapshot ? 'Live snapshot' : 'Connecting'}
        </span>
        {snapshot && <span>Last updated <time className="font-mono" dateTime={snapshot.at}>{clock.format(new Date(snapshot.at))}</time></span>}
      </footer>
    </dialog>
  );
}
