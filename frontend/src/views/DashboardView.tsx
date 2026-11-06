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
  Zap,
  ShieldAlert,
  GitMerge,
  Power,
  CheckCircle2,
  PauseCircle,
} from 'lucide-react';

interface AdapterWithHistory extends InterfaceInfo {
  rxHistory: number[];
  txHistory: number[];
}

export const DashboardView: React.FC = () => {
  const [interfaces, setInterfaces] = useState<AdapterWithHistory[]>([]);
  const [nmStatus, setNmStatus] = useState<NetMergeStatus | null>(null);
  const [apps, setApps] = useState<ProcessInfo[]>([]);
  const [rules, setRules] = useState<RuleRecord[]>([]);
  const [appSearch, setAppSearch] = useState<string>('');
  const [busyMode, setBusyMode] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string>('');

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
    const [ifaces, appList, ruleList, nm] = await Promise.all([
      api.getInterfaces(),
      api.getApplications(),
      api.getRules(),
      api.getNetMergeStatus(),
    ]);

    setRules(ruleList);
    setApps(appList);
    setNmStatus(nm);

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
  };

  useEffect(() => {
    loadData().catch((e) => console.error(e));
    const interval = setInterval(() => loadData().catch((e) => console.error(e)), 2000);
    return () => clearInterval(interval);
  }, []);

  const isMergeActive = nmStatus?.isRunning ?? false;
  const routingMode = nmStatus?.routingMode || nmStatus?.config?.routingMode || 'speed';
  const primaryAdapter = nmStatus?.config?.primaryAdapter || interfaces[0]?.name || '';
  const secondaryAdapter = nmStatus?.config?.secondaryAdapter || interfaces[1]?.name || interfaces[0]?.name || '';

  const handleToggleMerge = async () => {
    if (busyMode) return;
    setBusyMode(true);
    setErrorMsg('');
    try {
      await api.toggleNetMerge(!isMergeActive);
      await loadData().catch((e) => console.error(e));
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setBusyMode(false);
    }
  };

  const handleSwitchRoutingMode = async (mode: 'speed' | 'backup', pri?: string, sec?: string) => {
    if (busyMode) return;
    setBusyMode(true);
    try {
      await api.setNetMergeRoutingMode(
        mode,
        pri !== undefined ? pri : primaryAdapter,
        sec !== undefined ? sec : secondaryAdapter,
      );
      await loadData().catch((e) => console.error(e));
    } finally {
      setBusyMode(false);
    }
  };

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
      {/* Beszel-Style Top Control & Mode Banner */}
      <div className="card" style={{
        marginBottom: 0,
        padding: '14px 18px',
        background: '#121215',
        border: `1px solid ${isMergeActive ? 'rgba(16, 185, 129, 0.3)' : '#232329'}`,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        flexWrap: 'wrap',
        gap: '14px',
      }}>
        {/* Left: Adapter Status */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
          <div style={{
            width: '40px',
            height: '40px',
            borderRadius: '8px',
            background: isMergeActive ? 'rgba(16, 185, 129, 0.14)' : '#18181b',
            border: `1px solid ${isMergeActive ? 'rgba(16, 185, 129, 0.35)' : '#27272a'}`,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}>
            {isMergeActive ? (
              <CheckCircle2 size={20} color="#10b981" />
            ) : (
              <PauseCircle size={20} color="#a1a1aa" />
            )}
          </div>

          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ fontSize: '14px', fontWeight: 700, color: '#fafafa' }}>
                {isMergeActive
                  ? 'Active Adapter: net-merge'
                  : 'Default Adapter: System Default (Standby)'}
              </span>
              <span className={`status-pill ${isMergeActive ? 'pass' : 'blocked'}`}>
                {isMergeActive ? 'ACTIVE' : 'INACTIVE · DO NOTHING'}
              </span>
            </div>
            <div style={{ fontSize: '11.5px', color: '#a1a1aa', marginTop: '2px' }}>
              {isMergeActive
                ? routingMode === 'backup'
                  ? `Backup Failover Mode: Traffic uses Primary (${primaryAdapter || 'auto'}) and switches to Secondary (${secondaryAdapter || 'auto'}) only on failure.`
                  : 'Speed Mode: Utilizing all healthy network adapters simultaneously for maximum combined bandwidth.'
                : 'Net-Merge is off. Your OS uses its unmodified System Default adapter; NetPilot is standing by without altering any routes.'}
            </div>
          </div>
        </div>

        {/* Right: Mode Selector (Speed vs Backup) + Master Power Toggle */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
          {/* Speed vs Backup Mode Segmented Control */}
          <div style={{
            display: 'flex',
            alignItems: 'center',
            backgroundColor: '#09090b',
            border: '1px solid #27272a',
            borderRadius: '7px',
            padding: '3px',
            gap: '3px',
          }}>
            <button
              onClick={() => handleSwitchRoutingMode('speed')}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: '5px',
                padding: '5px 11px',
                borderRadius: '5px',
                fontSize: '11.5px',
                fontWeight: 600,
                cursor: 'pointer',
                border: 'none',
                backgroundColor: routingMode !== 'backup' ? '#27272a' : 'transparent',
                color: routingMode !== 'backup' ? '#38bdf8' : '#a1a1aa',
              }}
              title="Speed Mode: Use all adapters simultaneously for higher combined speed"
            >
              <Zap size={12} />
              <span>Speed Mode</span>
            </button>

            <button
              onClick={() => handleSwitchRoutingMode('backup')}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: '5px',
                padding: '5px 11px',
                borderRadius: '5px',
                fontSize: '11.5px',
                fontWeight: 600,
                cursor: 'pointer',
                border: 'none',
                backgroundColor: routingMode === 'backup' ? '#27272a' : 'transparent',
                color: routingMode === 'backup' ? '#fbbf24' : '#a1a1aa',
              }}
              title="Backup Mode: Use primary adapter, switch to secondary only if primary fails"
            >
              <ShieldAlert size={12} />
              <span>Backup Mode</span>
            </button>
          </div>

          {/* Primary / Secondary Adapter Selector when in Backup Mode */}
          {routingMode === 'backup' && interfaces.length > 0 && (
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '11px' }}>
              <span style={{ color: '#a1a1aa' }}>Primary:</span>
              <select
                value={primaryAdapter}
                onChange={e => handleSwitchRoutingMode('backup', e.target.value, secondaryAdapter)}
                style={{ padding: '4px 8px', fontSize: '11px' }}
              >
                {interfaces.map(i => (
                  <option key={i.name} value={i.name}>{i.name}</option>
                ))}
              </select>
              <span style={{ color: '#a1a1aa' }}>Backup:</span>
              <select
                value={secondaryAdapter}
                onChange={e => handleSwitchRoutingMode('backup', primaryAdapter, e.target.value)}
                style={{ padding: '4px 8px', fontSize: '11px' }}
              >
                {interfaces.map(i => (
                  <option key={i.name} value={i.name}>{i.name}</option>
                ))}
              </select>
            </div>
          )}

          <button
            onClick={handleToggleMerge}
            disabled={busyMode}
            className={`btn ${isMergeActive ? 'btn-danger' : 'btn-emerald'}`}
            style={{ padding: '6px 14px' }}
          >
            <Power size={13} />
            <span>{busyMode ? (isMergeActive ? 'Stopping...' : 'Starting...') : (isMergeActive ? 'Stop Net-Merge' : 'Start Net-Merge')}</span>
          </button>
        </div>
      </div>

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
                {isMergeActive ? 'net-merge' : 'System Default'}
              </strong>
            </span>
            <span>
              Active Flows: <strong className="font-mono" style={{ color: '#fafafa' }}>{totalSlotsActive}</strong>
            </span>
          </div>
        </div>

        <table className="data-table">
          <thead>
            <tr>
              <th style={{ width: '18%' }}>Adapter</th>
              <th style={{ width: '22%' }}>Addresses (LAN / WAN)</th>
              <th style={{ width: '16%' }}>Role & Weight</th>
              <th style={{ width: '18%' }}>Download Load</th>
              <th style={{ width: '14%' }}>Upload Load</th>
              <th style={{ width: '12%', textAlign: 'right' }}>Trend</th>
            </tr>
          </thead>
          <tbody>
            {interfaces.map((iface) => {
              const rx = iface.rxMbps || 0;
              const tx = iface.txMbps || 0;
              const rxStr = rx >= 1 ? `${rx.toFixed(2)} MB/s` : `${(rx * 1000).toFixed(0)} KB/s`;
              const txStr = tx >= 1 ? `${tx.toFixed(2)} MB/s` : `${(tx * 1000).toFixed(0)} KB/s`;

              const nmSlot = (nmStatus?.nics || []).find(s => s.Name === iface.name);
              const weightPct = nmSlot ? Math.round((nmSlot.Weight || 1) * 100) : 100;
              const capMbps = iface.rxLimitMbps && iface.rxLimitMbps > 0 ? iface.rxLimitMbps : 100;
              const rxBarPct = Math.min(100, Math.max(rx > 0 ? 4 : 0, (rx / capMbps) * 100));
              const txBarPct = Math.min(100, Math.max(tx > 0 ? 4 : 0, (tx / capMbps) * 100));

              let roleLabel = 'Balanced';
              let roleColor = '#38bdf8';
              if (!isMergeActive) {
                roleLabel = 'System Default';
                roleColor = '#a1a1aa';
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
              }

              return (
                <tr key={iface.name}>
                  {/* Adapter Name + Beszel Status Dot */}
                  <td>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '9px' }}>
                      <span className={`beszel-dot ${iface.isUp ? 'online' : 'down'}`} />
                      <div>
                        <div style={{ fontWeight: 600, fontSize: '13px', color: '#fafafa' }}>
                          {iface.name}
                        </div>
                        <div style={{ fontSize: '10.5px', color: '#71717a' }}>
                          {iface.isUp ? 'Online' : 'Offline'}
                          {nmSlot ? ` · ${nmSlot.Active} flows` : ''}
                        </div>
                      </div>
                    </div>
                  </td>

                  {/* LAN & WAN IPs */}
                  <td className="font-mono" style={{ fontSize: '11px' }}>
                    <div style={{ color: '#d4d4d8' }}>{iface.ip || 'Unassigned'}</div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: iface.wanIp ? '#10b981' : '#71717a', fontSize: '10.5px' }}>
                      <Globe size={10} />
                      <span>{iface.wanIp || 'Local subnet'}</span>
                      {iface.location && <span style={{ color: '#71717a' }}>({iface.location})</span>}
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
                      {i.name} ({i.ip || 'No IP'})
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
                      {i.name}
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
