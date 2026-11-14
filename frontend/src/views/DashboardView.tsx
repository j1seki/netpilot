import React, { useEffect, useState } from 'react';
import {
  api,
  InterfaceInfo,
  NetMergeStatus,
  ProcessInfo,
  RuleRecord,
} from '../api/wails';
import { Sparkline } from '../components/Charts';
import {
  Plus,
  ArrowDown,
  ArrowUp,
  X,
  Globe,
  Trash2,
  Edit3,
  Search,
  Activity,
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

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export const DashboardView: React.FC = () => {
  const [interfaces, setInterfaces] = useState<AdapterWithHistory[]>([]);
  const [nmStatus, setNmStatus] = useState<NetMergeStatus | null>(null);
  const [apps, setApps] = useState<ProcessInfo[]>([]);
  const [rules, setRules] = useState<RuleRecord[]>([]);
  const [appSearch, setAppSearch] = useState<string>('');
  const [errorMsg, setErrorMsg] = useState<string>('');
  // WAN IPs are hidden by default so a screen share or screenshot cannot leak
  // the user's public addresses by accident.
  const [showWan, setShowWan] = useState<boolean>(false);

  // Modal for Add / Edit App Rule
  const [showModal, setShowModal] = useState<boolean>(false);
  const [editingRuleId, setEditingRuleId] = useState<string>('');
  const [modalApp, setModalApp] = useState<string>('');
  const [modalPath, setModalPath] = useState<string>('');
  const [modalPreferred, setModalPreferred] = useState<string>('net-merge');
  const [modalFallback, setModalFallback] = useState<string>('');
  const [modalDns, setModalDns] = useState<string>('');
  const [modalLimit, setModalLimit] = useState<string>('');
  const [modalQuota, setModalQuota] = useState<string>('');

  const loadData = async () => {
    // Load each section independently. A single failing endpoint (for example net-merge
    // status on a host where the daemon is not installed) must never blank out the
    // adapter and process tables the way a Promise.all rejection used to.
    const [ifacesRes, appsRes, rulesRes, nmRes] = await Promise.allSettled([
      api.getInterfaces(),
      api.getApplications(),
      api.getRules(),
      api.getNetMergeStatus(),
    ]);

    const problems: string[] = [];

    if (ifacesRes.status === 'fulfilled') {
      const ifaces = ifacesRes.value;
      setInterfaces(prev => {
        return ifaces.map((cur) => {
          const existing = prev.find(p => p.name === cur.name);
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

  const openModalForRule = (rule?: RuleRecord, defaultAppName?: string, defaultPath?: string) => {
    if (rule) {
      setEditingRuleId(rule.id);
      setModalApp(rule.appName);
      setModalPath(rule.appPath || '');
      setModalPreferred(rule.preferredInterface || 'net-merge');
      setModalFallback(rule.fallbackInterface || '');
      setModalDns(rule.customDns || '');
      setModalLimit(rule.bandwidthLimitMbps > 0 ? String(rule.bandwidthLimitMbps) : '');
      setModalQuota(rule.dataQuotaMb > 0 ? String(rule.dataQuotaMb) : '');
    } else {
      setEditingRuleId('');
      setModalApp(defaultAppName || '');
      setModalPath(defaultPath || '');
      setModalPreferred('net-merge');
      setModalFallback(interfaces[0]?.name || '');
      setModalDns('');
      setModalLimit('');
      setModalQuota('');
    }
    setShowModal(true);
  };

  const handleSaveRule = async () => {
    const trimmed = modalApp.trim();
    if (!trimmed) return;

    const existing = rules.find(
      r => r.id === editingRuleId || r.appName.toLowerCase() === trimmed.toLowerCase()
    );

    const rule: RuleRecord = {
      id: existing ? existing.id : `rule-${Date.now()}`,
      appName: trimmed,
      appPath: modalPath.trim(),
      domainPattern: existing?.domainPattern || '',
      preferredInterface: modalPreferred,
      fallbackInterface: modalFallback,
      priority: existing?.priority || 100,
      customDns: modalDns.trim() || undefined,
      bandwidthLimitMbps: modalLimit ? parseFloat(modalLimit) : 0,
      dataQuotaMb: modalQuota ? parseFloat(modalQuota) : 0,
      dataUsedMb: existing?.dataUsedMb || 0,
      enabled: true,
    };
    await api.saveRule(rule);
    setShowModal(false);
    await loadData().catch((e) => console.error(e));
  };

  const handleDeleteRule = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    await api.deleteRule(id);
    await loadData().catch((e) => console.error(e));
  };

  const handleToggleRule = async (rule: RuleRecord, e: React.MouseEvent) => {
    e.stopPropagation();
    await api.saveRule({ ...rule, enabled: !rule.enabled });
    await loadData().catch((e) => console.error(e));
  };

  const getAppRule = (appName: string): RuleRecord | undefined => {
    return rules.find(
      r =>
        r.appName.toLowerCase() === appName.toLowerCase() ||
        appName.toLowerCase().includes(r.appName.toLowerCase())
    );
  };

  const formatRouteSummary = (rule?: RuleRecord): string => {
    if (!isMergeActive) {
      return 'System Default (Standby)';
    }
    if (!rule || !rule.enabled) {
      return 'net-merge (Default Adapter)';
    }
    if (rule.preferredInterface === 'FASTEST') {
      return 'Fastest Adapter (Lowest RTT)';
    }
    if (rule.preferredInterface === 'ALL' || rule.preferredInterface === 'net-merge') {
      return 'net-merge (Multi-Link)';
    }
    if (rule.fallbackInterface && rule.fallbackInterface !== rule.preferredInterface) {
      return `${rule.preferredInterface} → ${rule.fallbackInterface}`;
    }
    return rule.preferredInterface;
  };

  const filteredApps = apps.filter(a => {
    if (!appSearch.trim()) return true;
    const q = appSearch.toLowerCase();
    return a.name.toLowerCase().includes(q) || (a.executable && a.executable.toLowerCase().includes(q));
  });

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
      {/* Beszel-Style "All Systems / Network Adapters" Table Card */}
      <div className="card" style={{ marginBottom: 0, padding: '0', overflow: 'hidden' }}>
        <div style={{
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

          <div style={{ display: 'flex', alignItems: 'center', gap: '14px', fontSize: '11px', color: '#a1a1aa' }}>
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

        <table className="data-table">
          <thead>
            <tr>
              <th style={{ width: '15%' }}>Adapter</th>
              <th style={{ width: '20%' }}>Addresses (LAN / WAN)</th>
              <th style={{ width: '14%' }}>Role & Weight</th>
              <th style={{ width: '12%' }}>Latency</th>
              <th style={{ width: '16%' }}>Download Load</th>
              <th style={{ width: '13%' }}>Upload Load</th>
              <th style={{ width: '10%', textAlign: 'right' }}>Trend</th>
            </tr>
          </thead>
          <tbody>
            {interfaces.map((iface) => {
              const rx = iface.rxMbps || 0;
              const tx = iface.txMbps || 0;
              const rxStr = rx >= 1 ? `${rx.toFixed(2)} Mbps` : `${(rx * 1000).toFixed(0)} Kbps`;
              const txStr = tx >= 1 ? `${tx.toFixed(2)} Mbps` : `${(tx * 1000).toFixed(0)} Kbps`;

              const nmSlot = (nmStatus?.nics || []).find(s => s.Name === iface.name);
              const weightPct = nmSlot ? Math.round((nmSlot.Weight || 1) * 100) : 100;
              const capMbps = iface.rxLimitMbps && iface.rxLimitMbps > 0 ? iface.rxLimitMbps : 100;
              const rxBarPct = Math.min(100, Math.max(rx > 0 ? 4 : 0, (rx / capMbps) * 100));
              const txBarPct = Math.min(100, Math.max(tx > 0 ? 4 : 0, (tx / capMbps) * 100));

              let roleLabel = 'Merged Pool';
              let roleColor = '#38bdf8';
              if (!isMergeActive) {
                roleLabel = iface.isDefault ? 'OS Default' : 'Standby';
                roleColor = iface.isDefault ? '#10b981' : '#71717a';
              } else if (routingMode === 'backup') {
                if (iface.name === primaryAdapter) {
                  roleLabel = 'Primary Active';
                  roleColor = '#10b981';
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
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Two-Column Split Application Management */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: '1.15fr 1fr',
        gap: '14px',
        alignItems: 'start',
      }}>
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
            {isMergeActive
              ? 'Default adapter is net-merge. Applications below can be pinned to specific adapters, failover pairs, or bandwidth caps.'
              : 'Net-Merge is currently inactive (Standby). All applications use System Default and NetPilot does not modify any routes until activated.'}
          </p>

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
                  <th>Effective Adapter</th>
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
                    <tr
                      key={rule.id}
                      onClick={() => openModalForRule(rule)}
                      style={{ cursor: 'pointer' }}
                    >
                      <td>
                        <div style={{ fontWeight: 600, color: '#fafafa', display: 'flex', alignItems: 'center', gap: '6px' }}>
                          <span>{rule.appName}</span>
                          {isRunningNow && (
                            <span className="beszel-dot online" style={{ width: '6px', height: '6px' }} title="Running" />
                          )}
                        </div>
                        {rule.appPath && (
                          <div className="font-mono" style={{ fontSize: '10px', color: '#71717a' }}>
                            {rule.appPath}
                          </div>
                        )}
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
                          {formatRouteSummary(rule)}
                        </span>
                      </td>

                      <td className="font-mono" style={{ fontSize: '11px', color: '#a1a1aa' }}>
                        {rule.bandwidthLimitMbps > 0 ? `${rule.bandwidthLimitMbps} Mbps` : 'Unlimited'}
                        {rule.customDns ? ` · ${rule.customDns}` : ''}
                      </td>

                      <td>
                        <button
                          onClick={e => handleToggleRule(rule, e)}
                          className={`status-pill ${rule.enabled ? 'pass' : 'blocked'}`}
                          style={{ cursor: 'pointer' }}
                        >
                          {rule.enabled ? 'ENABLED' : 'PAUSED'}
                        </button>
                      </td>

                      <td style={{ textAlign: 'right' }}>
                        <button
                          onClick={e => handleDeleteRule(rule.id, e)}
                          style={{
                            background: 'transparent',
                            border: 'none',
                            color: '#fb7185',
                            cursor: 'pointer',
                            padding: '4px',
                          }}
                          title="Delete Policy"
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

        {/* RIGHT SEGMENT: Live Running Processes */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '7px' }}>
              <Activity size={14} color="#38bdf8" />
              <span>Active Processes ({apps.length})</span>
            </div>

            <div style={{ position: 'relative', width: '180px' }}>
              <Search size={12} color="#71717a" style={{ position: 'absolute', left: '9px', top: '8px' }} />
              <input
                type="text"
                placeholder="Search processes..."
                value={appSearch}
                onChange={e => setAppSearch(e.target.value)}
                style={{ width: '100%', paddingLeft: '26px', paddingRight: '8px', paddingTop: '4px', paddingBottom: '4px', fontSize: '11.5px' }}
              />
            </div>
          </div>

          <div style={{ maxHeight: '340px', overflowY: 'auto' }}>
            <table className="data-table">
              <thead>
                <tr>
                  <th>Process</th>
                  <th>PID</th>
                  <th>Adapter</th>
                  <th style={{ textAlign: 'right' }}>Rule</th>
                </tr>
              </thead>
              <tbody>
                {filteredApps.slice(0, 50).map(app => {
                  const matchedRule = getAppRule(app.name);
                  return (
                    <tr key={`${app.pid}-${app.name}`}>
                      <td>
                        <div style={{ fontWeight: 600, color: '#fafafa' }}>{app.name}</div>
                        {app.socketCount !== undefined && app.socketCount > 0 && (
                          <div style={{ fontSize: '10px', color: '#71717a' }}>
                            {app.socketCount} sockets
                          </div>
                        )}
                      </td>
                      <td className="font-mono" style={{ fontSize: '11px', color: '#71717a' }}>
                        {app.pid}
                      </td>
                      <td>
                        {!isMergeActive ? (
                          <span style={{ fontSize: '11px', color: '#71717a' }}>
                            System Default
                          </span>
                        ) : matchedRule ? (
                          <span style={{ fontSize: '11px', color: '#38bdf8', fontWeight: 600 }}>
                            {formatRouteSummary(matchedRule)}
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
                          style={{ padding: '3px 8px', fontSize: '11px' }}
                          onClick={() => openModalForRule(matchedRule, app.name, app.executable)}
                        >
                          {matchedRule ? 'Edit' : '+ Rule'}
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* Add / Modify App Policy Modal */}
      {showModal && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0, 0, 0, 0.75)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 1000,
        }}>
          <div style={{
            background: '#121215',
            border: '1px solid #27272a',
            borderRadius: '10px',
            width: '450px',
            padding: '18px 20px',
            display: 'flex',
            flexDirection: 'column',
            gap: '12px',
            boxShadow: '0 20px 40px rgba(0, 0, 0, 0.6)',
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3 style={{ fontSize: '14px', fontWeight: 700, color: '#fafafa' }}>
                {editingRuleId ? `Edit Policy: ${modalApp}` : 'New Application Policy'}
              </h3>
              <button
                onClick={() => setShowModal(false)}
                style={{ background: 'transparent', border: 'none', color: '#a1a1aa', cursor: 'pointer' }}
              >
                <X size={16} />
              </button>
            </div>

            <div>
              <label style={{ fontSize: '11px', color: '#a1a1aa', display: 'block', marginBottom: '4px' }}>
                Executable / Process Name *
              </label>
              <input
                type="text"
                placeholder="e.g. chrome.exe, steam.exe, qbittorrent.exe"
                value={modalApp}
                onChange={e => setModalApp(e.target.value)}
                style={{ width: '100%' }}
              />
            </div>

            <div>
              <label style={{ fontSize: '11px', color: '#a1a1aa', display: 'block', marginBottom: '4px' }}>
                Executable Path (Optional)
              </label>
              <input
                type="text"
                placeholder="e.g. C:\Program Files\App\app.exe"
                value={modalPath}
                onChange={e => setModalPath(e.target.value)}
                style={{ width: '100%' }}
              />
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
              <div>
                <label style={{ fontSize: '11px', color: '#a1a1aa', display: 'block', marginBottom: '4px' }}>
                  Preferred Adapter
                </label>
                <select
                  value={modalPreferred}
                  onChange={e => setModalPreferred(e.target.value)}
                  style={{ width: '100%' }}
                >
                  <option value="net-merge">net-merge (Default Merged Adapter)</option>
                  <option value="FASTEST">Fastest Adapter (Lowest Latency)</option>
                  {interfaces.map(i => (
                    <option key={i.name} value={i.name}>
                      {i.name} ({i.ip || 'No IP'}){i.isDefault ? ' · OS default' : ''}{i.isVirtual ? ' · VPN/virtual' : ''}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label style={{ fontSize: '11px', color: '#a1a1aa', display: 'block', marginBottom: '4px' }}>
                  Failover Backup Adapter
                </label>
                <select
                  value={modalFallback}
                  onChange={e => setModalFallback(e.target.value)}
                  style={{ width: '100%' }}
                >
                  <option value="">net-merge / System Default</option>
                  {interfaces.map(i => (
                    <option key={i.name} value={i.name}>
                      {i.name}{i.isDefault ? ' · OS default' : ''}{i.isVirtual ? ' · VPN/virtual' : ''}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
              <div>
                <label style={{ fontSize: '11px', color: '#a1a1aa', display: 'block', marginBottom: '4px' }}>
                  Bandwidth Limit (Mbps)
                </label>
                <input
                  type="number"
                  placeholder="Unlimited"
                  value={modalLimit}
                  onChange={e => setModalLimit(e.target.value)}
                  style={{ width: '100%' }}
                />
              </div>

              <div>
                <label style={{ fontSize: '11px', color: '#a1a1aa', display: 'block', marginBottom: '4px' }}>
                  Monthly Data Quota (MB)
                </label>
                <input
                  type="number"
                  placeholder="Unlimited"
                  value={modalQuota}
                  onChange={e => setModalQuota(e.target.value)}
                  style={{ width: '100%' }}
                />
              </div>
            </div>

            <div>
              <label style={{ fontSize: '11px', color: '#a1a1aa', display: 'block', marginBottom: '4px' }}>
                Custom DNS Server (Optional)
              </label>
              <input
                type="text"
                placeholder="e.g. 1.1.1.1 or 8.8.8.8"
                value={modalDns}
                onChange={e => setModalDns(e.target.value)}
                style={{ width: '100%' }}
              />
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px', marginTop: '4px' }}>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => setShowModal(false)}
              >
                Cancel
              </button>
              <button
                className="btn btn-primary btn-sm"
                onClick={handleSaveRule}
              >
                Save Policy
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
