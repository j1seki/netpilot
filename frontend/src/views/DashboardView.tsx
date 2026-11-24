import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  api,
  InterfaceInfo,
  NetMergeStatus,
  ProcessInfo,
  RuleRecord,
} from '../api/wails';
import { Sparkline } from '../components/Charts';
import { ProcessIcon } from '../components/ProcessIcon';
import { formatProcessRate, groupProcesses, type ProcessMetrics, type ProcessSort, type ProcessSortKey } from '../processes';
import { ExecutablePath } from '../components/ExecutablePath';
import { RuleEditor, type RuleDraft } from '../components/RuleEditor';
import { AppInspection } from '../components/AppInspection';
import { AdapterGroupButton, AdapterHideButton, useAdapterVisibility } from '../components/AdapterVisibility';
import {
  Plus,
  ArrowDown,
  ArrowUp,
  X,
  Globe,
  Trash2,
  Edit3,
  Pencil,
  Search,
  Activity,
  ChevronRight,
  ScanSearch,
  GitMerge,
  Eye,
  EyeOff,
  Router,
  Wifi,
} from 'lucide-react';


interface AdapterWithHistory extends InterfaceInfo {
  rxHistory: number[];
  txHistory: number[];
}

const processColumns: [ProcessSortKey, string][] = [
  ['name', 'Process'], ['pid', 'PID'], ['networkMbps', 'Network'], ['cpuPercent', 'CPU'], ['memoryBytes', 'RAM'],
];

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function ProcessMetricCells({ process, grouped = false }: { process: ProcessMetrics; grouped?: boolean }) {
  return <>
    <td className="font-mono process-pid" title={grouped ? 'Lowest matching PID in this group' : undefined}>{process.pid}</td>
    <td className="font-mono process-metric" title={grouped ? 'Sum of measured process rates in this group'
      : { 'windows-etw': 'Host TCP and UDP via Windows ETW', 'linux-tcp': 'Host TCP socket counters', netpilot: 'Attributed NetPilot forwarding' }[process.networkSource ?? 'netpilot']}>
      {formatProcessRate(process.networkMbps)}
    </td>
    <td className="font-mono process-metric">{process.cpuPercent == null ? '—' : `${process.cpuPercent.toFixed(1)}%`}</td>
    <td className="font-mono process-metric">{process.memoryBytes == null ? '—' : `${(process.memoryBytes / 2 ** 20).toFixed(1)} MiB`}</td>
  </>;
}

export const DashboardView: React.FC = () => {
  const [interfaces, setInterfaces] = useState<AdapterWithHistory[]>([]);
  const [nmStatus, setNmStatus] = useState<NetMergeStatus | null>(null);
  const [apps, setApps] = useState<ProcessInfo[]>([]);
  const [traffic, setTraffic] = useState<{ source?: ProcessInfo['networkSource']; detail: string } | null>(null);
  const refreshSequence = useRef({ started: 0, applied: 0 });
  const [inspectedApp, setInspectedApp] = useState<ProcessInfo | null>(null);
  const [rules, setRules] = useState<RuleRecord[]>([]);
  const [appSearch, setAppSearch] = useState<string>('');
  const [openProcessGroups, setOpenProcessGroups] = useState<Map<string, boolean>>(() => new Map());
  const [processSort, setProcessSort] = useState<ProcessSort>({ key: 'name', direction: 'ascending' });
  const sortProcessesBy = (key: ProcessSortKey) => {
    setProcessSort(prev => ({ key, direction: prev.key === key
      ? (prev.direction === 'ascending' ? 'descending' : 'ascending')
      : (key === 'name' || key === 'pid' ? 'ascending' : 'descending') }));
  };
  const visibility = useAdapterVisibility(interfaces);
  const [errorMsg, setErrorMsg] = useState<string>('');
  // WAN IPs are hidden by default so a screen share or screenshot cannot leak
  // the user's public addresses by accident.
  const [showWan, setShowWan] = useState<boolean>(false);

  const [ruleDraft, setRuleDraft] = useState<RuleDraft | null>(null);
  const [ruleError, setRuleError] = useState('');

  const loadData = async () => {
    const request = ++refreshSequence.current.started;
    // Load each section independently. A single failing endpoint (for example net-merge
    // status on a host where the daemon is not installed) must never blank out the
    // adapter and process tables the way a Promise.all rejection used to.
    const [ifacesRes, appsRes, rulesRes, nmRes, trafficRes] = await Promise.allSettled([
      api.getInterfaces(),
      api.getApplications(),
      api.getRules(),
      api.getNetMergeStatus(),
      api.getProcessTrafficStatus(),
    ]);
    if (request < refreshSequence.current.applied) return;
    refreshSequence.current.applied = request;

    const problems: string[] = [];

    if (ifacesRes.status === 'fulfilled') {
      const ifaces = ifacesRes.value;
      setInterfaces(prev => {
        const previous = new Map(prev.map(adapter => [adapter.name, adapter]));
        return ifaces.map((cur) => {
          const existing = previous.get(cur.name);
          const rx = cur.rxMbps || 0;
          const tx = cur.txMbps || 0;

          const rxHist = existing
            ? [...existing.rxHistory.slice(1), rx]
            : [0, 0, 0, 0, 0, 0, 0, 0, rx];
          const txHist = existing
            ? [...existing.txHistory.slice(1), tx]
            : [0, 0, 0, 0, 0, 0, 0, 0, tx];

          return {
            ...cur,
            rxHistory: rxHist,
            txHistory: txHist,
          };
        });
      });
    } else {
      problems.push(`adapters: ${errorText(ifacesRes.reason)}`);
    }

    if (appsRes.status === 'fulfilled') {
      setApps(appsRes.value);
    } else {
      problems.push(`processes: ${errorText(appsRes.reason)}`);
    }
    if (trafficRes.status === 'fulfilled') setTraffic(trafficRes.value);

    if (rulesRes.status === 'fulfilled') {
      setRules(rulesRes.value);
    } else {
      problems.push(`rules: ${errorText(rulesRes.reason)}`);
    }

    if (nmRes.status === 'fulfilled') {
      setNmStatus(nmRes.value);
    } else {
      problems.push(`net-merge: ${errorText(nmRes.reason)}`);
    }

    setErrorMsg(problems.length > 0 ? `Some live data could not be loaded — ${problems.join('; ')}` : '');
  };

  useEffect(() => {
    loadData().catch((e) => console.error(e));
    const interval = setInterval(() => loadData().catch((e) => console.error(e)), 2000);
    return () => clearInterval(interval);
  }, []);

  const isMergeActive = nmStatus?.isRunning ?? false;
  const systemAppRules = nmStatus?.systemAppRules ?? false;
  const routingMode = nmStatus?.routingMode || nmStatus?.config?.routingMode || 'speed';

  // Primary/secondary only mean something in Backup mode. In Speed mode the
  // primary link is net-merge itself, and when NetPilot is off the OS owns the
  // default route. Default to the OS default adapter, never array position.
  const osDefaultAdapter = interfaces.find(i => i.isDefault);
  const eligibleAdapters = interfaces.filter(i => !i.isVirtual && i.isUp);
  const primaryAdapter =
    nmStatus?.config?.primaryAdapter ||
    osDefaultAdapter?.name ||
    eligibleAdapters[0]?.name ||
    interfaces[0]?.name ||
    '';
  const secondaryAdapter =
    nmStatus?.config?.secondaryAdapter ||
    eligibleAdapters.find(i => i.name !== primaryAdapter)?.name ||
    interfaces.find(i => i.name !== primaryAdapter)?.name ||
    '';

  const openModalForRule = (rule?: RuleRecord, appName?: string, appPath?: string) => {
    setRuleDraft({ rule, appName, appPath });
  };

  const updateRule = async (action: () => Promise<unknown>) => {
    setRuleError('');
    try {
      await action();
      await loadData();
    } catch (err) {
      setRuleError(errorText(err));
    }
  };

  const getAppRule = (app: ProcessInfo): RuleRecord | undefined => {
    const basename = (name: string) => name.trim().toLowerCase().split(/[\\/]/).pop()?.replace(/\.exe$/, '');
    const name = basename(app.name);
    const executable = app.executable.trim().toLowerCase();
    return rules.find(rule => rule.id === app.ruleMatched) ?? rules.find(rule =>
      (executable && executable === rule.appPath.trim().toLowerCase()) || (name && basename(rule.appName) === name)
    );
  };

  const formatRouteSummary = (rule?: RuleRecord): string => {
    if (!isMergeActive) {
      return 'System Default (Standby)';
    }
    if (!systemAppRules || !rule || !rule.enabled) {
      return 'net-merge (Default Adapter)';
    }
    return formatConfiguredRoute(rule);
  };

  const formatConfiguredRoute = (rule: RuleRecord): string => {
    if (rule.preferredInterface === 'FASTEST') {
      return 'Fastest Adapter (Lowest RTT)';
    }
    if (rule.preferredInterface === 'ALL' || rule.preferredInterface === 'net-merge') {
      return 'Engine default';
    }
    if (rule.fallbackInterface && rule.fallbackInterface !== rule.preferredInterface) {
      return `${rule.preferredInterface} → ${rule.fallbackInterface}`;
    }
    return rule.preferredInterface;
  };

  const q = appSearch.trim().toLowerCase();
  const processGroups = useMemo(() => groupProcesses(apps, q, processSort), [apps, q, processSort]);
  const hasProcessRates = apps.some(app => app.networkMbps != null);
  const integratedActive = nmStatus?.socksRunning || (nmStatus?.isRunning && nmStatus.runningSource === 'builtin');
  const networkMessage = traffic?.source
    ? traffic.detail
    : [traffic?.detail, integratedActive || hasProcessRates ? 'NetPilot forwarding rates are available when traffic can be attributed.' : 'No attributable traffic yet.'].filter(Boolean).join(' ');

  const inspectButton = (app: ProcessInfo) => (
    <button type="button" className="btn btn-secondary btn-sm process-inspect"
      onClick={() => setInspectedApp(app)} aria-label={`Inspect ${app.name} (PID ${app.pid})`}>
      <ScanSearch size={12} /> Inspect
    </button>
  );

  // Compute total load across slots
  const totalSlotsActive = (nmStatus?.nics || []).reduce((acc, s) => acc + (s.Active || 0), 0);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
      {errorMsg && (
        <div style={{
          padding: '10px 14px',
          borderRadius: '8px',
          background: 'rgba(244, 63, 94, 0.14)',
          border: '1px solid rgba(244, 63, 94, 0.35)',
          color: '#fb7185',
          fontSize: '12px',
          fontWeight: 600,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}>
          <span>{errorMsg}</span>
          <button onClick={() => setErrorMsg('')} style={{ background: 'transparent', border: 'none', color: '#fb7185', cursor: 'pointer' }}>
            <X size={14} />
          </button>
        </div>
      )}
      {inspectedApp && <AppInspection process={inspectedApp} onClose={() => setInspectedApp(null)} />}
      {/* Beszel-Style "All Systems / Network Adapters" Table Card */}
      <div className="card netpilot-enter" style={{ marginBottom: 0, padding: '0', overflow: 'hidden' }}>
        <div className="adapter-card-heading" style={{
          padding: '13px 16px',
          borderBottom: '1px solid #232329',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <GitMerge size={15} color="#10b981" />
            <span style={{ fontSize: '13px', fontWeight: 600, color: '#fafafa' }}>
              Network Adapters ({interfaces.length})
            </span>
          </div>

          <div className="adapter-card-controls" style={{ display: 'flex', alignItems: 'center', gap: '14px', fontSize: '11px', color: '#a1a1aa' }}>
            <span>
              Default Route:{' '}
              <strong style={{ color: isMergeActive ? '#10b981' : '#fafafa' }}>
                {isMergeActive ? 'net-merge' : `System Default${osDefaultAdapter ? ` (${osDefaultAdapter.name})` : ''}`}
              </strong>
            </span>
            <span>
              Active Flows: <strong className="font-mono" style={{ color: '#fafafa' }}>{totalSlotsActive}</strong>
            </span>
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => setShowWan(v => !v)}
              title={showWan ? 'Hide WAN/public IP addresses' : 'Show WAN/public IP addresses'}
              aria-label={showWan ? 'Hide WAN IP addresses' : 'Show WAN IP addresses'}
              style={{ display: 'flex', alignItems: 'center', gap: '5px', padding: '3px 8px', fontSize: '10.5px' }}
            >
              {showWan ? <EyeOff size={12} /> : <Eye size={12} />}
              <span>{showWan ? 'Hide WAN' : 'Show WAN'}</span>
            </button>
          </div>
        </div>

        <p className="adapter-visibility-note">Groups keep the list compact. Hiding an adapter only changes this view.</p>
        <div className="adapter-table-scroll">
        <table className="data-table">
          <thead>
            <tr>
              <th style={{ width: '15%' }}>Adapter</th>
              <th style={{ width: '20%' }}>Addresses (LAN / WAN)</th>
              <th style={{ width: '12%' }}>Latency</th>
              <th style={{ width: '14%' }}>Role & Weight</th>
              <th style={{ width: '16%' }}>Download Load</th>
              <th style={{ width: '13%' }}>Upload Load</th>
              <th style={{ width: '10%', textAlign: 'right' }}>Trend</th>
              <th><span className="sr-only">Visibility</span></th>
            </tr>
          </thead>
          {visibility.sections.map(section => <tbody key={section.group}>
            <tr className="adapter-group-row"><td colSpan={8}>
              <div className="adapter-group-heading">
                <AdapterGroupButton {...section} count={section.adapters.length} />
                {section.group === 'hidden' && <button className="btn btn-secondary btn-sm" onClick={visibility.restore}>Show all hidden</button>}
              </div>
            </td></tr>
            {section.open && section.adapters.map((iface) => {
              const rx = iface.rxMbps || 0;
              const tx = iface.txMbps || 0;
              const rxStr = rx >= 1 ? `${rx.toFixed(2)} Mbps` : `${(rx * 1000).toFixed(0)} Kbps`;
              const txStr = tx >= 1 ? `${tx.toFixed(2)} Mbps` : `${(tx * 1000).toFixed(0)} Kbps`;

              const nmSlot = (nmStatus?.nics || []).find(s => s.Name === iface.name);
              const weightPct = nmSlot ? Math.round((nmSlot.Weight || 1) * 100) : 100;
              const rxCap = iface.rxLimitMbps || 100;
              const txCap = iface.txLimitMbps || 100;
              const rxBarPct = Math.min(100, Math.max(rx > 0 ? 4 : 0, (rx / rxCap) * 100));
              const txBarPct = Math.min(100, Math.max(tx > 0 ? 4 : 0, (tx / txCap) * 100));

              let roleLabel = 'Merged Pool';
              let roleColor = '#38bdf8';
              if (!isMergeActive) {
                roleLabel = iface.isDefault ? 'OS Default' : 'Standby';
                roleColor = iface.isDefault ? '#10b981' : '#71717a';
              } else if (routingMode === 'backup') {
                if (iface.name === nmStatus?.activeAdapter) {
                  roleLabel = iface.name === primaryAdapter ? 'Primary Active' : 'Failover Active';
                  roleColor = '#10b981';
                } else if (iface.name === primaryAdapter) {
                  roleLabel = 'Primary Standby';
                  roleColor = '#a1a1aa';
                } else if (iface.name === secondaryAdapter) {
                  roleLabel = 'Failover Standby';
                  roleColor = '#f59e0b';
                } else {
                  roleLabel = 'Backup Pool';
                  roleColor = '#a1a1aa';
                }
              } else {
                // Speed mode has no primary/secondary: every healthy adapter is
                // used at once. The OS default is still highlighted for context.
                roleLabel = iface.isDefault ? 'Merged · OS Default' : 'Merged Pool';
                roleColor = iface.isDefault ? '#38bdf8' : '#a1a1aa';
              }

              const latencyColor = (ms: number) => ms <= 0 ? '#71717a' : ms < 50 ? '#34d399' : ms < 150 ? '#fbbf24' : '#fb7185';
              const latencyText = (ms: number) => {
                if (!(ms > 0)) return '—';
                if (ms < 1) return `${ms.toFixed(2)} ms`;
                if (ms < 10) return `${ms.toFixed(1)} ms`;
                return `${ms.toFixed(0)} ms`;
              };

              return (
                <tr key={iface.name}>
                  {/* Adapter Name + Beszel Status Dot */}
                  <td>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '9px' }}>
                      <span className={`beszel-dot ${iface.isUp ? 'online' : 'down'}`} />
                      <div>
                        <div style={{ fontWeight: 600, fontSize: '13px', color: '#fafafa', display: 'flex', alignItems: 'center', gap: '5px', flexWrap: 'wrap' }}>
                          <span>{iface.displayName || iface.name}</span>
                          {iface.label && iface.label !== iface.name && (
                            <span className="font-mono" style={{ fontSize: '9.5px', color: '#71717a', fontWeight: 500 }}>
                              ({iface.name})
                            </span>
                          )}
                          {iface.isDefault && (
                            <span className="status-pill pass" style={{ fontSize: '9px', padding: '1px 5px' }} title="Adapter carrying the OS default route">
                              OS DEFAULT
                            </span>
                          )}
                          {iface.isVirtual && (
                            <span className="status-pill warn" style={{ fontSize: '9px', padding: '1px 5px' }} title="VPN, tunnel or software adapter. Never auto-selected as a physical primary.">
                              VPN/VIRTUAL
                            </span>
                          )}
                        </div>
                        <div style={{ fontSize: '10.5px', color: '#71717a' }}>
                          {iface.isUp ? 'Online' : 'Offline'}
                          {nmSlot ? ` · ${nmSlot.Active} flows` : ''}
                        </div>
                      </div>
                    </div>
                  </td>

                  {/* LAN & WAN IPs (WAN hidden until the eye toggle is used) */}
                  <td className="font-mono" style={{ fontSize: '11px' }}>
                    <div style={{ color: '#d4d4d8' }}>{iface.ip || 'Unassigned'}</div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: iface.wanIp ? '#10b981' : '#71717a', fontSize: '10.5px' }}>
                      <Globe size={10} />
                      <span title={showWan ? (iface.wanIp || 'Local subnet') : 'WAN IP hidden — click Show WAN in the table header'}>
                        {showWan ? (iface.wanIp || 'Local subnet') : (iface.wanIp ? '••••••••' : 'Local subnet')}
                      </span>
                      {showWan && iface.location && <span style={{ color: '#71717a' }}>({iface.location})</span>}
                      {showWan && iface.isp && <span style={{ color: '#71717a' }}>· {iface.isp}</span>}
                    </div>
                  </td>

                  {/* Router / web latency */}
                  <td className="font-mono" style={{ fontSize: '10.5px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: latencyColor(iface.latencyRouterMs) }} title="ICMP round-trip to this adapter's default gateway">
                      <Router size={10} />
                      <span>{latencyText(iface.latencyRouterMs)}</span>
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: latencyColor(iface.latencyWebMs), marginTop: '2px' }} title="TCP handshake to a public anycast resolver, bound to this adapter">
                      <Wifi size={10} />
                      <span>{latencyText(iface.latencyWebMs)}</span>
                    </div>
                  </td>

                  {/* Role & Share Bar */}
                  <td>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '5px', maxWidth: '150px' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '11px' }}>
                        <span style={{ color: roleColor, fontWeight: 600 }}>{roleLabel}</span>
                        <span className="font-mono" style={{ color: '#a1a1aa' }}>{weightPct}%</span>
                      </div>
                      <div className="beszel-meter">
                        <div
                          className="beszel-meter-fill"
                          style={{
                            width: `${Math.min(100, weightPct)}%`,
                            backgroundColor: roleColor,
                          }}
                        />
                      </div>
                    </div>
                  </td>

                  {/* Download Meter */}
                  <td>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '5px', maxWidth: '170px' }}>
                      <div className="font-mono" style={{ display: 'flex', justifyContent: 'space-between', fontSize: '11.5px' }}>
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '3px', color: '#38bdf8', fontWeight: 600 }}>
                          <ArrowDown size={11} />
                          {rxStr}
                        </span>
                        {iface.rxLimitMbps && iface.rxLimitMbps > 0 ? (
                          <span style={{ color: '#71717a', fontSize: '10px' }}>cap {iface.rxLimitMbps}M</span>
                        ) : null}
                      </div>
                      <div className="beszel-meter">
                        <div
                          className="beszel-meter-fill"
                          style={{
                            width: `${rxBarPct}%`,
                            backgroundColor: '#38bdf8',
                          }}
                        />
                      </div>
                    </div>
                  </td>

                  {/* Upload Meter */}
                  <td>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '5px', maxWidth: '140px' }}>
                      <div className="font-mono" style={{ display: 'flex', justifyContent: 'space-between', fontSize: '11.5px' }}>
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '3px', color: '#f59e0b', fontWeight: 600 }}>
                          <ArrowUp size={11} />
                          {txStr}
                        </span>
                        {(iface.txLimitMbps ?? 0) > 0 && (
                          <span style={{ color: '#71717a', fontSize: '10px' }}>cap {iface.txLimitMbps}M</span>
                        )}
                      </div>
                      <div className="beszel-meter">
                        <div
                          className="beszel-meter-fill"
                          style={{
                            width: `${txBarPct}%`,
                            backgroundColor: '#f59e0b',
                          }}
                        />
                      </div>
                    </div>
                  </td>

                  {/* Sparkline */}
                  <td style={{ textAlign: 'right' }}>
                    <div style={{ display: 'inline-block' }}>
                      <Sparkline data={iface.rxHistory} color="#10b981" width={86} height={22} />
                    </div>
                  </td>
                  <td><AdapterHideButton name={iface.name} hidden={section.group === 'hidden'} toggle={() => visibility.setHidden(iface.name, section.group !== 'hidden')} /></td>
                </tr>
              );
            })}
          </tbody>)}
        </table>
        </div>
        {interfaces.length === 0 && <p className="adapter-visibility-note">No network adapters found yet.</p>}
      </div>

      {/* Two-Column Split Application Management */}
      <div className="dashboard-app-grid">
        {/* LEFT SEGMENT: Configured App Policies */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '7px' }}>
              <Edit3 size={14} color="#10b981" />
              <span>Application Routing Policies ({rules.length})</span>
            </div>

            <button
              className="btn btn-primary btn-sm"
              onClick={() => openModalForRule(undefined, '', '')}
            >
              <Plus size={12} />
              <span>Add Policy</span>
            </button>
          </div>

          <p style={{ fontSize: '11.5px', color: '#a1a1aa', marginBottom: '12px' }}>
            {systemAppRules
              ? 'Default adapter is net-merge. Applications below can be pinned to specific adapters, failover pairs, or bandwidth caps.'
              : isMergeActive
                ? 'The tunnel is running in a separate process with its own routing configuration. NetPilot app rules are available through the in-app SOCKS5 proxy.'
                : 'All applications use System Default in Standby. Start Net-Merge or the local SOCKS5 proxy to route traffic through NetPilot.'}
          </p>

          {ruleError && <p role="alert" style={{ color: '#fb7185' }}>{ruleError}</p>}
          {rules.length === 0 ? (
            <div style={{
              padding: '26px 16px',
              textAlign: 'center',
              background: '#09090b',
              border: '1px dashed #27272a',
              borderRadius: '7px',
              color: '#71717a',
              fontSize: '12px',
            }}>
              No custom application policies configured. Default traffic routes via{' '}
              <strong style={{ color: isMergeActive ? '#10b981' : '#fafafa' }}>
                {isMergeActive ? 'net-merge' : 'System Default'}
              </strong>.
            </div>
          ) : (
            <table className="data-table">
              <thead>
                <tr>
                  <th>Application</th>
                  <th>Configured route</th>
                  <th>Caps / DNS</th>
                  <th>State</th>
                  <th style={{ textAlign: 'right' }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {rules.map(rule => {
                  const isRunningNow = apps.some(
                    a => a.name.toLowerCase() === rule.appName.toLowerCase()
                  );
                  return (
                    <tr key={rule.id}>
                      <td>
                        <div style={{ fontWeight: 600, color: '#fafafa', display: 'flex', alignItems: 'center', gap: '6px' }}>
                          <span>{rule.appName}</span>
                          {isRunningNow && (
                            <span className="beszel-dot online" style={{ width: '6px', height: '6px' }} title="Running" />
                          )}
                        </div>
                        <ExecutablePath path={rule.appPath} />
                      </td>

                      <td>
                        <span style={{
                          fontSize: '11px',
                          fontWeight: 600,
                          padding: '2px 7px',
                          borderRadius: '5px',
                          background: isMergeActive ? 'rgba(16, 185, 129, 0.12)' : '#18181b',
                          color: isMergeActive ? '#34d399' : '#a1a1aa',
                          border: `1px solid ${isMergeActive ? 'rgba(16, 185, 129, 0.25)' : '#27272a'}`,
                        }}>
                          {formatConfiguredRoute(rule)}
                        </span>
                      </td>

                      <td className="font-mono" style={{ fontSize: '11px', color: '#a1a1aa' }}>
                        {rule.bandwidthLimitMbps > 0 ? `${rule.bandwidthLimitMbps} Mbps` : 'Unlimited'}
                        {rule.customDns ? ` · ${rule.customDns}` : ''}
                      </td>

                      <td>
                        <button
                          onClick={() => updateRule(() => api.saveRule({ ...rule, enabled: !rule.enabled }))}
                          className={`status-pill ${rule.enabled ? 'pass' : 'blocked'}`}
                          style={{ cursor: 'pointer' }}
                        >
                          {rule.enabled ? 'ENABLED' : 'PAUSED'}
                        </button>
                      </td>

                      <td style={{ textAlign: 'right' }}>
                        <button type="button" className="btn btn-secondary btn-sm" onClick={() => openModalForRule(rule)}>
                          <Pencil size={12} /> Edit
                        </button>
                        <button
                          onClick={() => updateRule(() => api.deleteRule(rule.id))}
                          style={{
                            background: 'transparent',
                            border: 'none',
                            color: '#fb7185',
                            cursor: 'pointer',
                            padding: '4px',
                          }}
                          title="Delete policy" aria-label="Delete policy"
                        >
                          <Trash2 size={13} />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>

        <section className="card process-panel" aria-labelledby="active-processes-title">
          <div className="card-title process-panel-header">
            <div className="panel-identity">
              <span className="panel-icon"><Activity size={16} aria-hidden="true" /></span>
              <h2 id="active-processes-title">Active Processes</h2>
              <span className="process-panel-count tabular-nums">{apps.length}</span>
            </div>

            <div className="process-search">
              <Search size={14} aria-hidden="true" />
              <input
                type="search"
                placeholder="Search processes"
                aria-label="Search processes by name, path or PID"
                value={appSearch}
                onChange={e => setAppSearch(e.target.value)}
              />
            </div>
          </div>

          <div className="process-legend">
            <p>{networkMessage}</p>
            <div className="process-legend-values">
              <span><b className="font-mono">0</b> Measured idle</span>
              <span><b className="font-mono">—</b> Unavailable</span>
            </div>
          </div>
          <div className="process-table-scroll" role="region" aria-label="Active Processes" tabIndex={0}>
            <table className="data-table process-table">
              <thead>
                <tr>
                  {processColumns.map(([key, label]) => {
                    const active = processSort.key === key;
                    const direction = active ? processSort.direction : undefined;
                    const next = direction === 'ascending' || (!active && key !== 'name' && key !== 'pid') ? 'descending' : 'ascending';
                    return <th key={key} scope="col" aria-sort={direction}>
                      <button type="button" className="process-sort-header" onClick={() => sortProcessesBy(key)} aria-label={`Sort by ${label}, ${next}`}>
                        {label}<span aria-hidden="true" className="process-sort-triangle" data-direction={direction}>{direction === 'ascending' ? '▲' : '▼'}</span>
                      </button>
                    </th>;
                  })}
                  <th scope="col">Adapter</th>
                  <th scope="col" style={{ textAlign: 'right' }}>Rule</th>
                </tr>
              </thead>
              <tbody>
                {processGroups.map(group => {
                  const first = group.processes.find(app => app.pid === group.pid) ?? group.processes[0];
                  const grouped = group.processes.length > 1;
                  const expanded = openProcessGroups.get(group.key) ?? Boolean(q);
                  return <React.Fragment key={group.key}>
                    {grouped && (
                      <tr className="process-group-row">
                        <td>
                          <div className="process-name">
                            <ProcessIcon pid={first.pid} executable={first.executable} />
                            <div className="process-identity">
                              <div className="process-name-text" title={group.name}>{group.name}</div>
                            </div>
                            {inspectButton(first)}
                            <button type="button" className="process-group-button" aria-expanded={expanded}
                              aria-label={`${expanded ? 'Collapse' : 'Expand'} ${group.name} (${group.processes.length} processes)`}
                              onClick={() => setOpenProcessGroups(prev => new Map(prev).set(group.key, !(prev.get(group.key) ?? Boolean(q))))}>
                              <span className="process-count">{group.processes.length}</span>
                              <ChevronRight size={14} aria-hidden="true" />
                            </button>
                          </div>
                        </td>
                        <ProcessMetricCells process={group} grouped />
                        <td colSpan={2}></td>
                      </tr>
                    )}
                    {(!grouped || expanded) && group.processes.map(app => {
                      const matchedRule = getAppRule(app);
                      return (
                        <tr key={app.pid} className={grouped ? 'process-child-row' : undefined}>
                          <td>
                            <div className="process-name">
                              <ProcessIcon pid={app.pid} executable={app.executable} />
                              <div className="process-identity">
                                <div className="process-name-text" title={app.name}>{app.name}</div>
                                {(app.socketCount ?? 0) > 0 && (
                                  <div style={{ fontSize: '10px', color: '#71717a' }}>
                                    {app.socketCount} sockets
                                  </div>
                                )}
                              </div>
                              {inspectButton(app)}
                            </div>
                          </td>
                          <ProcessMetricCells process={app} />
                          <td>
                            {!isMergeActive ? (
                              <span style={{ fontSize: '11px', color: '#71717a' }}>
                                System Default
                              </span>
                            ) : systemAppRules && matchedRule ? (
                              <span style={{ fontSize: '11px', color: '#38bdf8', fontWeight: 600 }}>
                                {app.assignedNic === 'Blocked' ? 'Blocked' : formatRouteSummary(matchedRule)}
                              </span>
                            ) : (
                              <span style={{ fontSize: '11px', color: '#10b981', fontWeight: 500 }}>
                                net-merge
                              </span>
                            )}
                          </td>
                          <td style={{ textAlign: 'right' }}>
                            <button
                              className="btn btn-secondary btn-sm"
                              style={{ padding: '3px 8px', fontSize: '11px', whiteSpace: 'nowrap' }}
                              onClick={() => openModalForRule(matchedRule, app.name, app.executable)}
                            >
                              {matchedRule ? 'Edit' : '+ Rule'}
                            </button>
                          </td>
                        </tr>
                      );
                    })}
                  </React.Fragment>;
                })}
                {!processGroups.length && <tr><td colSpan={7}>{q ? 'No matching processes.' : 'No processes found yet.'}</td></tr>}
              </tbody>
            </table>
          </div>
        </section>
      </div>

      {ruleDraft && <RuleEditor draft={ruleDraft} interfaces={interfaces} close={() => setRuleDraft(null)} saved={() => {
        setRuleDraft(null);
        void loadData();
      }} />}
    </div>
  );
};
