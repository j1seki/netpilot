import { useEffect, useMemo, useState } from 'react';
import { Activity, ArrowDown, ArrowRight, ArrowUp, Edit3, GitMerge, Pencil, Plus, Sliders } from 'lucide-react';
import { api, type InterfaceInfo, type NetMergeStatus, type RuleRecord } from '../api/wails';
import { Sparkline } from '../components/Charts';
import { NetMergeControl } from '../components/NetMergeControl';
import { RuleEditor, type RuleDraft } from '../components/RuleEditor';
import { SimpleTrafficChart, type TrafficPoint } from '../components/SimpleTrafficChart';
import { formatProcessRate } from '../processes';
import type { MainTab } from '../App';
import './SimpleView.css';

type TrafficSample = { host: TrafficPoint; routed: TrafficPoint; adapterDownloads: Record<string, number> };
const sampleLimit = 26;
const bytesPerSecondToMbps = 8 / 1_000_000;

function ruleTitle(rule: RuleRecord) {
  return rule.appName || rule.appPath.split(/[\\/]/).filter(Boolean).pop() || rule.domainPattern || 'Routing policy';
}

export function SimpleView({ interfaces, interfacesLoaded, interfacesError, status, statusError, refreshStatus, openAdvanced }: {
  interfaces: InterfaceInfo[];
  interfacesLoaded: boolean;
  interfacesError: string;
  status: NetMergeStatus | null;
  statusError: string;
  refreshStatus: () => Promise<void>;
  openAdvanced: (tab: MainTab) => void;
}) {
  const [samples, setSamples] = useState<TrafficSample[]>([]);
  const [rules, setRules] = useState<RuleRecord[] | null>(null);
  const [rulesError, setRulesError] = useState('');
  const [draft, setDraft] = useState<RuleDraft | null>(null);
  const [pending, setPending] = useState(false);
  const [actionError, setActionError] = useState('');
  const [dryRun, setDryRun] = useState(false);
  const running = status?.isRunning ?? false;
  const backup = (status?.routingMode ?? status?.config.routingMode) === 'backup';
  const slots = status?.nics ?? [];
  const present = new Map(slots.filter(slot => slot.Present).map(slot => [slot.Name, slot]));
  const links = interfaces.filter(link => !link.isVirtual || present.has(link.name));
  const activeCount = slots.reduce((total, slot) => total + (slot.Active || 0), 0);
  const onlineCount = links.filter(link => link.isUp && (link.ip || present.has(link.name))).length;
  const routedTraffic = slots.reduce((sum, slot) => ({
    rx: sum.rx + (slot.RateIn || 0) * bytesPerSecondToMbps,
    tx: sum.tx + (slot.RateOut || 0) * bytesPerSecondToMbps,
  }), { rx: 0, tx: 0 });
  const hostTraffic = links.reduce((sum, link) => ({
    rx: sum.rx + (link.rxMbps || 0),
    tx: sum.tx + (link.txMbps || 0),
  }), { rx: 0, tx: 0 });
  const traffic = running ? routedTraffic : hostTraffic;
  const trafficHistory = useMemo(() => samples.map(sample => running ? sample.routed : sample.host), [samples, running]);

  useEffect(() => {
    if (!interfacesLoaded && !status) return;
    setSamples(previous => [...previous.slice(1 - sampleLimit), {
      host: hostTraffic,
      routed: routedTraffic,
      adapterDownloads: Object.fromEntries(links.map(link => [link.name, link.rxMbps || 0])),
    }]);
  }, [interfaces, interfacesLoaded, status]);

  const refreshRules = async () => {
    try {
      setRules(await api.getRules());
      setRulesError('');
    } catch (error) {
      setRulesError(error instanceof Error ? error.message : String(error));
    }
  };

  useEffect(() => {
    void refreshRules();
    api.getExecutionMode().then(mode => setDryRun(mode.currentMode === 'DRY_RUN')).catch(() => {});
  }, []);

  const toggleRouting = async () => {
    if (!status || pending || statusError || (dryRun && !running)) return;
    const next = !running;
    setPending(true);
    setActionError('');
    try {
      await api.toggleNetMerge(next);
      await refreshStatus();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : String(error));
    } finally {
      setPending(false);
    }
  };

  const summary = statusError || !status ? undefined
    : dryRun && !running ? 'Dry-Run is active. Enable Live Mode in Settings to start routing.'
    : running ? (backup ? 'Backup failover' : 'Speed merge') : undefined;
  const adapterName = (name: string) => {
    if (name === 'ALL' || name === 'net-merge') return 'NetPilot default';
    if (name === 'FASTEST') return 'Fastest adapter';
    return links.find(link => link.name === name)?.displayName || name;
  };

  return (
    <main className="app-content">
      <div className="simple-workspace">
        <div className="simple-routing simple-motion" aria-busy={pending}>
          <NetMergeControl
            running={running}
            busy={pending}
            disabled={!status || !!statusError || (dryRun && !running)}
            statusLabel={statusError ? 'Status unavailable' : !status ? 'Checking status…' : undefined}
            summary={summary}
            onToggle={() => void toggleRouting()}
            actions={<button type="button" className="btn btn-secondary" onClick={() => openAdvanced(dryRun && !running ? 'settings' : 'netmerge')}>
              <Sliders size={13} aria-hidden="true" />{dryRun && !running ? 'Open Settings' : 'Routing Settings'}
            </button>}
          />
        </div>
        {(actionError || statusError) && <p className="diagnostic-feedback diagnostic-error" role="alert">{actionError || statusError}</p>}

        <div className="simple-overview">
          <section className="card simple-table-card simple-motion" aria-labelledby="simple-adapters">
            <div className="card-title simple-card-heading">
              <h2 id="simple-adapters"><GitMerge size={15} aria-hidden="true" />Network Adapters{interfacesLoaded && ` (${links.length})`}</h2>
              {interfacesLoaded && <span className="rule-editor-note">{onlineCount} connected</span>}
            </div>
            {interfacesError && <p className="diagnostic-feedback diagnostic-error" role="alert">{interfacesError}</p>}
            {links.length ? <div className="simple-table-scroll">
              <table className="data-table">
                <thead><tr><th scope="col">Adapter</th><th scope="col">Download</th><th scope="col">Upload</th><th scope="col">Trend</th></tr></thead>
                <tbody>{links.map(link => {
                  const slot = present.get(link.name);
                  const reachable = link.isUp && (!!link.ip || !!slot) && (slot?.State !== 'no internet' || slot.State6 === 'up');
                  const state = !link.isUp ? 'Offline'
                    : !link.ip && !slot ? 'Waiting for address'
                    : !reachable ? 'No internet'
                    : !running && link.isDefault ? 'System Default'
                    : running && slot && backup ? (slot.Active > 0 ? 'In use' : 'Available')
                    : running && slot ? 'Routing enabled' : 'Connected';
                  return <tr key={link.name}>
                    <td>
                      <div className="simple-adapter-name">
                        <span className={`beszel-dot ${reachable ? 'online' : 'standby'}`} aria-hidden="true" />
                        <strong title={link.name}>{link.displayName || link.name}</strong>
                      </div>
                      <span className="rule-editor-note">{state}{link.latencyWebMs > 0 && ` · ${Math.round(link.latencyWebMs)} ms`}</span>
                    </td>
                    <td className="font-mono simple-download">{formatProcessRate(link.rxMbps || 0)}</td>
                    <td className="font-mono simple-upload">{formatProcessRate(link.txMbps || 0)}</td>
                    <td><div aria-hidden="true"><Sparkline data={samples.map(sample => sample.adapterDownloads[link.name] || 0)}
                      width={72} height={18} color="var(--accent-primary)" /></div></td>
                  </tr>;
                })}</tbody>
              </table>
            </div> : !interfacesError && <p className="adapter-visibility-note">{interfacesLoaded ? 'No network adapters detected.' : 'Loading adapters…'}</p>}
          </section>

          <section className="card simple-traffic simple-motion" aria-labelledby="simple-traffic">
            <div className="card-title simple-card-heading">
              <h2 id="simple-traffic"><Activity size={15} aria-hidden="true" />{statusError ? 'Last Observed Traffic' : running ? 'Forwarded Traffic' : 'Adapter Traffic'}</h2>
              {running && !statusError && <span className="rule-editor-note">{activeCount.toLocaleString()} active {activeCount === 1 ? 'flow' : 'flows'}</span>}
            </div>
            <div className="simple-traffic-rates">
              <div className="simple-traffic-rate">
                <span className="simple-rate-icon simple-download"><ArrowDown size={15} aria-hidden="true" /></span>
                <div><span>Download</span><strong className="font-mono">{formatProcessRate(traffic.rx)}</strong></div>
              </div>
              <div className="simple-traffic-rate">
                <span className="simple-rate-icon simple-upload"><ArrowUp size={15} aria-hidden="true" /></span>
                <div><span>Upload</span><strong className="font-mono">{formatProcessRate(traffic.tx)}</strong></div>
              </div>
            </div>
            <div className="simple-traffic-monitor">
              <SimpleTrafficChart samples={trafficHistory} capacity={sampleLimit} />
            </div>
          </section>
        </div>

        <section className="card simple-table-card simple-policies simple-motion" aria-labelledby="simple-policy-title">
          <div className="card-title simple-card-heading">
            <h2 id="simple-policy-title"><Edit3 size={14} aria-hidden="true" />Application Routing Policies{rules && ` (${rules.length})`}</h2>
            <button type="button" className="btn btn-primary btn-sm" onClick={() => setDraft({})}><Plus size={12} aria-hidden="true" />Add App</button>
          </div>
          <p className="adapter-visibility-note">{status?.systemAppRules ? 'Rules apply as apps open new connections.'
            : status?.socksRunning ? 'Rules apply to apps using the NetPilot proxy.'
              : running ? 'The running engine uses its own routing configuration.'
                : 'Saved rules apply when the NetPilot engine is routing.'}</p>
          {rulesError && <p className="diagnostic-feedback diagnostic-error" role="alert">{rulesError} <button type="button" className="btn btn-secondary btn-sm" onClick={() => void refreshRules()}>Retry</button></p>}
          {rules?.length ? <div className="simple-table-scroll">
            <table className="data-table">
              <thead><tr><th scope="col">Application</th><th scope="col">Configured route</th><th scope="col">State</th><th scope="col">Actions</th></tr></thead>
              <tbody>{rules.map(rule => <tr key={rule.id}>
                <td><strong>{ruleTitle(rule)}</strong></td>
                <td><span className="simple-rule-route font-mono">{adapterName(rule.preferredInterface)}
                  {rule.fallbackInterface && <><ArrowRight size={12} aria-hidden="true" />{adapterName(rule.fallbackInterface)}</>}</span></td>
                <td><span className={`status-pill ${rule.enabled ? 'pass' : 'blocked'}`}>{rule.enabled ? 'Enabled' : 'Paused'}</span></td>
                <td><button type="button" className="btn btn-secondary btn-sm" aria-label={`Edit ${ruleTitle(rule)}`}
                  onClick={() => setDraft({ rule })}><Pencil size={12} aria-hidden="true" />Edit</button></td>
              </tr>)}</tbody>
            </table>
          </div> : rules !== null && <p className="adapter-visibility-note">No app rules yet.</p>}
        </section>
      </div>
      {draft && <RuleEditor simple draft={draft} interfaces={links} eligibleVirtual={[...present.keys()]}
        close={() => setDraft(null)} saved={() => { setDraft(null); void refreshRules(); }} />}
    </main>
  );
}
