import { useEffect, useState } from 'react';
import {
  Activity, AppWindow, ArrowDown, ArrowRight, ArrowUp, Cable, GitMerge,
  LoaderCircle, Pencil, Play, Plus, Settings2, Shield, Square, Wifi, Zap,
} from 'lucide-react';
import { api, type InterfaceInfo, type NetMergeStatus, type RuleRecord } from '../api/wails';
import { RuleEditor, type RuleDraft } from '../components/RuleEditor';
import type { MainTab } from '../App';
import './SimpleView.css';

type Traffic = { rx: number; tx: number };
type TrafficSample = { host: Traffic; routed: Traffic; links: Record<string, Traffic> };
const sampleLimit = 26;
const bytesPerSecondToMbps = 8 / 1_000_000;

function formatRate(mbps: number) {
  if (mbps >= 1000) return { value: (mbps / 1000).toFixed(2), unit: 'Gbps' };
  if (mbps >= 1) return { value: mbps.toFixed(mbps >= 10 ? 1 : 2), unit: 'Mbps' };
  return { value: (mbps * 1000).toFixed(0), unit: 'Kbps' };
}

function Rate({ mbps, direction }: { mbps: number; direction: 'download' | 'upload' }) {
  const Icon = direction === 'download' ? ArrowDown : ArrowUp;
  const { value, unit } = formatRate(mbps);
  return <div className={`simple-rate simple-rate--${direction}`}>
    <span className="simple-rate-label"><Icon size={13} aria-hidden="true" />{direction === 'download' ? 'Download' : 'Upload'}</span>
    <strong>{value} <small>{unit}</small></strong>
  </div>;
}

function TrafficChart({ samples, compact = false }: { samples: Traffic[]; compact?: boolean }) {
  const count = compact ? 18 : sampleLimit;
  const points = samples.slice(-count);
  const max = Math.max(0.001, ...points.map(point => Math.max(point.rx, point.tx)));
  return (
    <div className={`simple-chart${compact ? ' simple-chart--compact' : ''}`} aria-hidden="true">
      {Array.from({ length: count }, (_, index) => {
        const point = points[index - count + points.length];
        return <div className="simple-chart-sample" key={index}>
          <span className="simple-chart-download" style={{ height: `${point ? Math.max(3, point.rx / max * 100) : 0}%` }} />
          <span className="simple-chart-upload" style={{ height: `${point ? Math.max(3, point.tx / max * 100) : 0}%` }} />
        </div>;
      })}
    </div>
  );
}

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
  const [pending, setPending] = useState<boolean | null>(null);
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

  useEffect(() => {
    if (!interfacesLoaded && !status) return;
    setSamples(previous => [...previous.slice(1 - sampleLimit), {
      host: hostTraffic,
      routed: routedTraffic,
      links: Object.fromEntries(links.map(link => [link.name, { rx: link.rxMbps || 0, tx: link.txMbps || 0 }])),
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
    if (!status || pending !== null || statusError) return;
    const next = !running;
    setPending(next);
    setActionError('');
    try {
      await api.toggleNetMerge(next);
      await refreshStatus();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : String(error));
    } finally {
      setPending(null);
    }
  };

  const state = pending === true ? 'Starting routing' : pending === false ? 'Stopping routing'
    : statusError ? 'Routing status unavailable' : !status ? 'Checking routing'
      : running ? 'Routing is on' : 'Routing is off';
  const detail = statusError || !status ? ''
    : dryRun && !running ? 'Dry-Run is active. Enable Live Mode in Settings to start routing.'
    : running ? backup ? 'New connections use the primary link and switch to a backup if needed.'
      : 'New connections are shared across available links.'
      : 'Your system is choosing how connections are routed.';
  const adapterName = (name: string) => {
    if (name === 'ALL' || name === 'net-merge') return 'NetPilot default';
    if (name === 'FASTEST') return 'Fastest adapter';
    return links.find(link => link.name === name)?.displayName || name;
  };

  return (
    <main className="simple-content">
      <div className="simple-workspace">
        <section className="simple-hero" aria-labelledby="simple-routing" data-running={running}>
          <div className="simple-hero-controls">
            <span className="simple-mode"><span className="simple-beacon" />{!status ? 'Network routing' : backup ? 'Backup mode' : 'Speed mode'}</span>
            <h1 id="simple-routing" aria-live="polite">{state}</h1>
            {detail && <p>{detail}</p>}
            <div className="simple-hero-actions">
              <button type="button" className="simple-start" disabled={!status || !!statusError || pending !== null || (dryRun && !running)}
                onClick={() => void toggleRouting()}>
                {pending !== null ? <LoaderCircle size={17} className="simple-spin" aria-hidden="true" />
                  : running ? <Square size={15} aria-hidden="true" /> : <Play size={16} aria-hidden="true" />}
                {pending === true ? 'Starting…' : pending === false ? 'Stopping…' : running ? 'Stop routing' : 'Start routing'}
              </button>
              <button type="button" className="simple-link" onClick={() => openAdvanced(dryRun && !running ? 'settings' : 'netmerge')}>
                <Settings2 size={16} aria-hidden="true" />{dryRun && !running ? 'Open settings' : 'Routing settings'}
              </button>
            </div>
            {(actionError || statusError) && <p className="simple-error" role="alert">{actionError || statusError}</p>}
          </div>
          <div className="simple-traffic">
            <div className="simple-traffic-heading">
              <h2><Activity size={17} aria-hidden="true" />{statusError ? 'Last observed traffic' : running ? 'Forwarded traffic' : 'Adapter traffic'}</h2>
              {running && !statusError && <span title="Open connections reported by the routing engine">{activeCount.toLocaleString()} active {activeCount === 1 ? 'flow' : 'flows'}</span>}
            </div>
            <div className="simple-traffic-rates">
              <Rate mbps={traffic.rx} direction="download" />
              <Rate mbps={traffic.tx} direction="upload" />
            </div>
            <TrafficChart samples={samples.map(sample => running ? sample.routed : sample.host)} />
          </div>
        </section>

        <section className="simple-section" aria-labelledby="simple-adapters">
          <div className="simple-section-heading">
            <h2 id="simple-adapters">Connections</h2>
            {interfacesLoaded && <p>{onlineCount} connected</p>}
          </div>
          {interfacesError && <p className="simple-error" role="alert">{interfacesError}</p>}
          {links.length ? <div className="simple-adapter-grid">
            {links.map(link => {
              const slot = present.get(link.name);
              const isWireless = /wi[ -]?fi|wireless|wlan|^wl/i.test(`${link.name} ${link.displayName ?? ''}`);
              const Icon = isWireless ? Wifi : Cable;
              const state = !link.isUp ? 'Offline'
                : !link.ip && !slot ? 'Waiting for address'
                : slot?.State === 'no internet' && slot.State6 !== 'up' ? 'No internet'
                : !running && link.isDefault ? 'System default'
                : running && slot && backup ? (slot.Active > 0 ? 'In use' : 'Available')
                : running && slot ? 'Routing enabled' : 'Connected';
              return <article className="simple-adapter" key={link.name}>
                <div className="simple-adapter-heading">
                  <span className="simple-adapter-icon"><Icon size={19} aria-hidden="true" /></span>
                  <div className="simple-adapter-title">
                    <h3 title={link.name}>{link.displayName || link.name}</h3>
                    <p data-online={link.isUp && (!!link.ip || !!slot) && state !== 'No internet'}><span />{state}</p>
                  </div>
                  {link.latencyWebMs > 0 && <span className="simple-latency" title="Internet latency"><Zap size={12} aria-hidden="true" />{Math.round(link.latencyWebMs)} ms</span>}
                </div>
                <div className="simple-adapter-rates">
                  <Rate mbps={link.rxMbps || 0} direction="download" />
                  <Rate mbps={link.txMbps || 0} direction="upload" />
                  <TrafficChart compact samples={samples.flatMap(sample => sample.links[link.name] ? [sample.links[link.name]] : [])} />
                </div>
              </article>;
            })}
          </div> : !interfacesError && <p className="simple-empty">{interfacesLoaded ? 'No network connections detected.' : 'Looking for connections…'}</p>}
        </section>

        <section className="simple-section simple-policies" aria-labelledby="simple-policy-title">
          <div className="simple-section-heading">
            <div>
              <h2 id="simple-policy-title">Application routing</h2>
              <p>{status?.systemAppRules ? 'Rules apply as apps open new connections.'
                : status?.socksRunning ? 'Rules apply to apps using the NetPilot proxy.'
                  : running ? 'The running engine uses its own routing configuration.'
                    : 'Saved rules apply when the NetPilot engine is routing.'}</p>
            </div>
            <button type="button" className="simple-add" onClick={() => setDraft({})}><Plus size={16} aria-hidden="true" /> Add app</button>
          </div>
          {rulesError && <p className="simple-error" role="alert">{rulesError} <button type="button" className="simple-link" onClick={() => void refreshRules()}>Retry</button></p>}
          {rules?.length ? <ul className="simple-rule-list">
            {rules.map(rule => <li key={rule.id}>
              <button type="button" className="simple-rule" onClick={() => setDraft({ rule })}>
                <span className="simple-rule-icon"><AppWindow size={17} aria-hidden="true" /></span>
                <span className="simple-rule-identity"><strong>{ruleTitle(rule)}</strong><span>{rule.enabled ? 'Enabled' : 'Paused'}</span></span>
                <span className="simple-rule-route"><GitMerge size={14} aria-hidden="true" />{adapterName(rule.preferredInterface)}
                  {rule.fallbackInterface && <><ArrowRight size={13} aria-hidden="true" />{adapterName(rule.fallbackInterface)}</>}</span>
                <Pencil size={15} className="simple-edit-icon" aria-hidden="true" />
              </button>
            </li>)}
          </ul> : rules !== null && <div className="simple-policy-empty">
            <span><Shield size={21} aria-hidden="true" /></span>
            <div><strong>No app rules yet</strong><p>Add an app to choose which connection it uses.</p></div>
          </div>}
        </section>
      </div>
      {draft && <RuleEditor simple draft={draft} interfaces={links} eligibleVirtual={slots.filter(slot => slot.Present).map(slot => slot.Name)}
        close={() => setDraft(null)} saved={() => { setDraft(null); void refreshRules(); }} />}
    </main>
  );
}
