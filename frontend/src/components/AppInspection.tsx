import { ExecutablePath } from './ExecutablePath';
import { useEffect, useRef, useState } from 'react';
import { Activity, AlertCircle, ArrowRight, Cable, Network, Route, X } from 'lucide-react';
import { api, ProcessInfo, ProcessInspection, ProcessRuleMatch, RuleRouteProbe } from '../api/wails';
import { ProcessIcon } from './ProcessIcon';
import './AppInspection.css';

const clock = new Intl.DateTimeFormat(undefined, { hour: 'numeric', minute: '2-digit', second: '2-digit' });

function AddressList({ values, empty }: { values: string[]; empty: string }) {
  return values.length ? <ul className="inspection-addresses">
    {values.map(value => <li key={value}><code>{value}</code></li>)}
  </ul> : <span className="inspection-muted">{empty}</span>;
}

const scopeDescription: Record<ProcessInspection['ruleScope'], string> = {
  system: 'The NetPilot tunnel can enforce matching rules on new traffic it handles.',
  socks: 'The live test uses NetPilot’s SOCKS5 forwarder. An app must use the proxy for its own traffic to follow these rules.',
  separate: 'A separate tunnel owns system routing. Start NetPilot’s SOCKS5 proxy to test these rules, or start the built-in tunnel from an elevated NetPilot window for system-wide rules.',
  standby: 'Start NetPilot’s SOCKS5 proxy or the in-app system tunnel in Net-Merge to test a live route.',
};

const ruleResult: Record<ProcessRuleMatch['result'], string> = {
  selected: 'Selected',
  shadowed: 'Lower precedence',
  disabled: 'Disabled',
};

function RuleVerification({ snapshot, verify }: { snapshot: ProcessInspection; verify: (destination: string) => void }) {
  const [target, setTarget] = useState(snapshot.ruleDestination);
  const [probe, setProbe] = useState<RuleRouteProbe | null>(null);
  const [probeError, setProbeError] = useState('');
  const [probing, setProbing] = useState(false);
  const [startingProxy, setStartingProxy] = useState(false);
  const startProxy = async () => {
    setProbeError('');
    setStartingProxy(true);
    try {
      await api.controlNetMergeSocks('start');
    } catch (err) {
      setProbeError(err instanceof Error ? err.message : String(err));
    } finally {
      setStartingProxy(false);
    }
  };
  const forwarding = snapshot.ruleScope === 'system' || snapshot.ruleScope === 'socks';
  const testRoute = async () => {
    setProbe(null);
    setProbeError('');
    setProbing(true);
    try {
      setProbe(await api.testApplicationRule(snapshot.process, target.trim()));
    } catch (err) {
      setProbeError(err instanceof Error ? err.message : String(err));
    } finally {
      setProbing(false);
    }
  };
  const selected = snapshot.ruleMatches.find(match => match.result === 'selected')?.rule;
  const hostname = (value: string) => value.toLowerCase().replace(/\.$/, '');
  const observed = selected && snapshot.activity.find(event => {
    const host = event.destination.replace(/^\[|\]?:[^:]+$/g, '');
    return event.ruleId === selected.id
      && (!snapshot.ruleDestination || hostname(host) === hostname(snapshot.ruleDestination));
  });

  return <div className="inspection-view inspection-rules" key="rules">
    <p className="inspection-help">{scopeDescription[snapshot.ruleScope]} Preview checks the saved match. A live test opens one TCP connection through the active NetPilot forwarder. The default port is 443.</p>
    {!forwarding && <button className="btn btn-secondary btn-sm" type="button" disabled={startingProxy} onClick={startProxy}>
      {startingProxy ? 'Starting proxy…' : 'Start local SOCKS5 proxy'}
    </button>}
    <form className="inspection-rule-query" onSubmit={event => { event.preventDefault(); verify(target.trim()); }}>
      <label>Destination (optional)
        <input value={target} onChange={event => { setTarget(event.target.value); setProbe(null); setProbeError(''); }}
          disabled={probing} placeholder="example.com or 192.0.2.1:443" maxLength={300} />
      </label>
      <button className="btn btn-secondary btn-sm" type="submit">Preview match</button>
      <button className="btn btn-primary btn-sm" type="button" disabled={probing || !forwarding || !target.trim()} onClick={testRoute}>
        {probing ? 'Testing…' : 'Test live route'}
      </button>
    </form>
    {probe && <p className="inspection-rule-evidence" role="status">
      Connected to <b>{probe.peer}</b> through <b>{probe.adapter}</b> using source IP <b>{probe.sourceIp}</b>. Rule <b>{probe.ruleId}</b> · {probe.adapter === probe.fallback ? 'fallback adapter' : 'preferred route'}.
    </p>}
    {probeError && <p className="inspection-alert" role="alert">{probeError}</p>}
    <p className="inspection-help">{snapshot.ruleDestination ? `Match preview for ${snapshot.ruleDestination}` : 'Match preview for this process. Enter a destination to include destination rules.'}</p>
    {snapshot.ruleMatches.length > 0 ? <ul className="inspection-rule-list">
      {snapshot.ruleMatches.map(({ rule, matchedBy, result }) => <li key={rule.id}>
        <div className="inspection-rule-heading">
          <strong>{rule.appName || rule.domainPattern || rule.appPath || rule.id}</strong>
          <span className={`status-pill ${result === 'selected' ? 'pass' : 'skipped'}`}>{ruleResult[result]}</span>
        </div>
        <div className="inspection-muted">Priority {rule.priority} · Matched by {matchedBy}</div>
        <p>Route {rule.preferredInterface === 'net-merge' || rule.preferredInterface === 'ALL' ? 'Engine default' : rule.preferredInterface}
          {rule.fallbackInterface && ` → ${rule.fallbackInterface === 'net-merge' ? 'Engine default' : rule.fallbackInterface}`}
          {rule.customDns && ` · DNS ${rule.customDns}`}
          {rule.bandwidthLimitMbps > 0 && ` · ${rule.bandwidthLimitMbps} Mbps cap`}
          {rule.dataQuotaMb > 0 && (rule.dataUsedMb >= rule.dataQuotaMb ? ' · Quota reached' : ` · ${rule.dataQuotaMb} MB quota`)}
        </p>
      </li>)}
    </ul> : <p className="inspection-empty">No saved rules match this preview.</p>}
    {selected && <p className="inspection-help">
      {observed ? `Observed this rule on ${observed.adapter} at ${clock.format(new Date(observed.at))} for ${observed.destination}.`
        : 'No matching app traffic observed yet. Keep Inspect open and start a new connection in the selected app. Its rule and adapter appear under Forwarded activity.'}
    </p>}
  </div>;
}

export function AppInspection({ process, onClose }: { process: ProcessInfo; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [snapshot, setSnapshot] = useState<ProcessInspection | null>(null);
  const [error, setError] = useState('');
  const [ruleQuery, setRuleQuery] = useState({ destination: '' });
  const [view, setView] = useState<'sockets' | 'activity' | 'rules'>('sockets');

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const data = await api.inspectApplication(process, controller.signal, ruleQuery.destination);
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
  }, [process, ruleQuery]);

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
          <ExecutablePath path={process.executable} />
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
              <dt><Route size={14} aria-hidden="true" /> Last forwarded adapter</dt>
              <dd>{snapshot.activity[0]?.adapter || '—'}</dd>
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
                <button type="button" className="tab-btn" aria-pressed={view === 'rules'} onClick={() => setView('rules')}>
                  <Route size={14} aria-hidden="true" /> Routing rules
                </button>
              </div>
              {view === 'rules' ? <RuleVerification snapshot={snapshot} verify={destination => setRuleQuery({ destination })} /> : view === 'sockets' ? (
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
                        <thead><tr><th scope="col">Time</th><th scope="col">Destination</th><th scope="col">Adapter</th><th scope="col">Rule ID</th></tr></thead>
                        <tbody>{snapshot.activity.map(item => (
                          <tr key={`${item.protocol}-${item.destination}-${item.adapter}-${item.dnsQuestion}-${item.ruleId}`}>
                            <td className="font-mono inspection-time"><time dateTime={item.at}>{clock.format(new Date(item.at))}</time></td>
                            <td>
                              <span className="inspection-protocol">{item.protocol}</span>
                              <div className="inspection-endpoint font-mono">{item.destination}</div>
                              {item.dnsQuestion && <div className="inspection-endpoint inspection-remote font-mono">{item.dnsQuestion}</div>}
                            </td>
                            <td>{item.adapter}</td>
                            <td className="font-mono">{item.ruleId || 'Engine default'}</td>
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
                  <dt>Previewed rule DNS</dt>
                  <dd><AddressList values={[snapshot.savedRuleDns]} empty="" /></dd>
                  <dd className="inspection-muted">Applies to matching DNS traffic forwarded by NetPilot.</dd>
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
