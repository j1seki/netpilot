import React, { useEffect, useState } from 'react';
import {
  api,
  InterfaceInfo,
  ProcessInfo,
  RuleRecord,
} from '../api/wails';
import { Sparkline } from '../components/Charts';
import {
  Plus,
  ArrowDown,
  ArrowUp,
  X,
  Network,
  Globe,
  Trash2,
  Edit3,
  Search,
  Activity,
} from 'lucide-react';

interface AdapterWithHistory extends InterfaceInfo {
  rxHistory: number[];
  txHistory: number[];
  isDefault?: boolean;
}

export const DashboardView: React.FC = () => {
  const [interfaces, setInterfaces] = useState<AdapterWithHistory[]>([]);
  const [apps, setApps] = useState<ProcessInfo[]>([]);
  const [rules, setRules] = useState<RuleRecord[]>([]);
  const [appSearch, setAppSearch] = useState<string>('');

  // Modal for Add / Edit App Rule
  const [showModal, setShowModal] = useState<boolean>(false);
  const [editingRuleId, setEditingRuleId] = useState<string>('');
  const [modalApp, setModalApp] = useState<string>('');
  const [modalPath, setModalPath] = useState<string>('');
  const [modalPreferred, setModalPreferred] = useState<string>('FASTEST');
  const [modalFallback, setModalFallback] = useState<string>('');
  const [modalDns, setModalDns] = useState<string>('');
  const [modalLimit, setModalLimit] = useState<string>('');
  const [modalQuota, setModalQuota] = useState<string>('');

  const loadData = async () => {
    const [ifaces, appList, ruleList] = await Promise.all([
      api.getInterfaces(),
      api.getApplications(),
      api.getRules(),
    ]);

    setRules(ruleList);
    setApps(appList);

    setInterfaces(prev => {
      return ifaces.map((cur, idx) => {
        const existing = prev.find(p => p.name === cur.name);
        const rx = cur.rxMbps || 0;
        const tx = cur.txMbps || 0;

        const rxHist = existing
          ? [...existing.rxHistory.slice(1), rx]
          : [0, 0, 0, 0, 0, 0, rx];
        const txHist = existing
          ? [...existing.txHistory.slice(1), tx]
          : [0, 0, 0, 0, 0, 0, tx];

        return {
          ...cur,
          rxHistory: rxHist,
          txHistory: txHist,
          isDefault: existing?.isDefault !== undefined ? existing.isDefault : idx === 0,
        };
      });
    });
  };

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 2000);
    return () => clearInterval(interval);
  }, []);

  const openModalForRule = (rule?: RuleRecord, defaultAppName?: string, defaultPath?: string) => {
    if (rule) {
      setEditingRuleId(rule.id);
      setModalApp(rule.appName);
      setModalPath(rule.appPath || '');
      setModalPreferred(rule.preferredInterface || 'FASTEST');
      setModalFallback(rule.fallbackInterface || '');
      setModalDns(rule.customDns || '');
      setModalLimit(rule.bandwidthLimitMbps > 0 ? String(rule.bandwidthLimitMbps) : '');
      setModalQuota(rule.dataQuotaMb > 0 ? String(rule.dataQuotaMb) : '');
    } else {
      setEditingRuleId('');
      setModalApp(defaultAppName || '');
      setModalPath(defaultPath || '');
      setModalPreferred('FASTEST');
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
    await loadData();
  };

  const handleDeleteRule = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    await api.deleteRule(id);
    await loadData();
  };

  const handleToggleRule = async (rule: RuleRecord, e: React.MouseEvent) => {
    e.stopPropagation();
    await api.saveRule({ ...rule, enabled: !rule.enabled });
    await loadData();
  };

  const getAppRule = (appName: string): RuleRecord | undefined => {
    return rules.find(
      r =>
        r.appName.toLowerCase() === appName.toLowerCase() ||
        appName.toLowerCase().includes(r.appName.toLowerCase())
    );
  };

  const defaultIfaceName = interfaces.find(i => i.isDefault)?.name || interfaces[0]?.name || 'system default';
  const allIfaceNames = interfaces.map(i => i.name).join(' + ') || 'active adapters';

  const formatRouteSummary = (rule?: RuleRecord): string => {
    if (!rule || !rule.enabled) {
      return `System Default (${defaultIfaceName})`;
    }
    if (rule.preferredInterface === 'FASTEST') {
      return `Fastest Adapter (${allIfaceNames})`;
    }
    if (rule.preferredInterface === 'ALL') {
      return `Merged (${allIfaceNames})`;
    }
    if (rule.fallbackInterface && rule.fallbackInterface !== rule.preferredInterface) {
      return `${rule.preferredInterface} (backup: ${rule.fallbackInterface})`;
    }
    return rule.preferredInterface;
  };

  const filteredApps = apps.filter(a => {
    if (!appSearch.trim()) return true;
    const q = appSearch.toLowerCase();
    return a.name.toLowerCase().includes(q) || (a.executable && a.executable.toLowerCase().includes(q));
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
      {/* Live Network Adapters Grid */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: `repeat(${Math.max(1, Math.min(interfaces.length, 3))}, 1fr)`,
        gap: '10px',
      }}>
        {interfaces.map((iface) => {
          const rx = iface.rxMbps || 0;
          const tx = iface.txMbps || 0;
          const rxStr = rx >= 1 ? `${rx.toFixed(2)} Mbit/s` : `${(rx * 1000).toFixed(0)} kbit/s`;
          const txStr = tx >= 1 ? `${tx.toFixed(2)} Mbit/s` : `${(tx * 1000).toFixed(0)} kbit/s`;

          return (
            <div
              key={iface.name}
              style={{
                background: '#121620',
                border: '1px solid #1e2636',
                borderRadius: '7px',
                padding: '11px 13px',
                display: 'flex',
                flexDirection: 'column',
                gap: '8px',
              }}
            >
              {/* Adapter Header */}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                    <Network size={14} color="#38bdf8" />
                    <span style={{ fontSize: '13.5px', fontWeight: 700, color: '#f1f5f9' }}>
                      {iface.name}
                    </span>
                  </div>

                  {/* Local IP & Real WAN IP */}
                  <div className="font-mono" style={{ fontSize: '11px', color: '#94a3b8', marginTop: '3px', display: 'flex', flexDirection: 'column', gap: '1px' }}>
                    <div>
                      <span style={{ color: '#64748b' }}>LAN: </span>
                      <span>{iface.ip || 'Unassigned'}</span>
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                      <Globe size={10} color={iface.wanIp ? '#34d399' : '#64748b'} />
                      <span style={{ color: '#64748b' }}>WAN: </span>
                      <span style={{ color: iface.wanIp ? '#34d399' : '#64748b', fontWeight: iface.wanIp ? 600 : 400 }}>
                        {iface.wanIp || 'Private / Local Subnet'}
                      </span>
                      {iface.location && (
                        <span style={{ color: '#64748b', fontSize: '10px' }}>({iface.location})</span>
                      )}
                    </div>
                  </div>
                </div>

                <div style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                  <span className={`status-pill ${iface.isUp ? 'pass' : 'fail'}`} style={{ fontSize: '10px' }}>
                    {iface.isUp ? 'ONLINE' : 'DOWN'}
                  </span>

                  {iface.isDefault ? (
                    <span style={{
                      fontSize: '10px',
                      fontWeight: 600,
                      padding: '2px 6px',
                      borderRadius: '4px',
                      background: 'rgba(2, 132, 199, 0.2)',
                      color: '#38bdf8',
                      border: '1px solid rgba(56, 189, 248, 0.3)',
                    }}>
                      PRIMARY
                    </span>
                  ) : (
                    <button
                      onClick={() => {
                        setInterfaces(prev => prev.map(p => ({ ...p, isDefault: p.name === iface.name })));
                      }}
                      style={{
                        fontSize: '10px',
                        fontWeight: 500,
                        padding: '2px 6px',
                        borderRadius: '4px',
                        background: 'transparent',
                        color: '#64748b',
                        border: '1px solid #263042',
                        cursor: 'pointer',
                      }}
                    >
                      Set Primary
                    </button>
                  )}
                </div>
              </div>

              {/* Live Throughput Row */}
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', paddingTop: '2px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                    <ArrowDown size={13} color="#38bdf8" />
                    <span className="font-mono" style={{ color: '#38bdf8', fontWeight: 700, fontSize: '12.5px' }}>
                      {rxStr}
                    </span>
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                    <ArrowUp size={13} color="#eab308" />
                    <span className="font-mono" style={{ color: '#eab308', fontWeight: 700, fontSize: '12.5px' }}>
                      {txStr}
                    </span>
                  </div>
                </div>

                <Sparkline data={iface.rxHistory} color="#38bdf8" width={90} height={22} />
              </div>

              {/* Footer: Limits & Quotas if set */}
              <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                fontSize: '10.5px',
                color: '#64748b',
                borderTop: '1px solid rgba(255, 255, 255, 0.05)',
                paddingTop: '5px',
              }}>
                <span>
                  {iface.rxLimitMbps && iface.rxLimitMbps > 0
                    ? `Speed Cap: ↓ ${iface.rxLimitMbps} Mbps`
                    : 'Speed Cap: Unlimited'}
                </span>
                <span>
                  {iface.dataQuotaMb && iface.dataQuotaMb > 0
                    ? `Used: ${(iface.dataUsedMb || 0).toFixed(0)} / ${iface.dataQuotaMb.toFixed(0)} MB`
                    : 'No Data Cap'}
                </span>
              </div>
            </div>
          );
        })}
      </div>

      {/* Two-Column Split Application Management:
          1. Configured App Policies (Apps Added to Modify Settings)
          2. Live Running Host Processes (Click + to Add) */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: '1.15fr 1fr',
        gap: '12px',
        alignItems: 'start',
      }}>
        {/* LEFT SEGMENT: Configured App Policies (Added Apps) */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '7px' }}>
              <Edit3 size={14} color="#38bdf8" />
              <span>Configured App Policies ({rules.length})</span>
            </div>

            <button
              className="btn btn-primary btn-sm"
              onClick={() => openModalForRule(undefined, '', '')}
            >
              <Plus size={12} />
              <span>Manually Add App</span>
            </button>
          </div>

          <p style={{ fontSize: '11px', color: '#64748b', marginBottom: '10px' }}>
            Applications added here have custom adapter binding, failover backup, DNS, or bandwidth caps applied.
          </p>

          {rules.length === 0 ? (
            <div style={{
              padding: '24px 16px',
              textAlign: 'center',
              background: '#0b0e14',
              border: '1px dashed #263042',
              borderRadius: '6px',
              color: '#64748b',
              fontSize: '11.5px',
            }}>
              No custom application policies added yet. Click <strong>"Manually Add App"</strong> above or select any process from the <strong>Running Processes</strong> list on the right.
            </div>
          ) : (
            <table className="data-table">
              <thead>
                <tr>
                  <th>Application</th>
                  <th>Assigned Route</th>
                  <th>Limits / DNS</th>
                  <th>Status</th>
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
                        <div style={{ fontWeight: 600, color: '#f1f5f9', display: 'flex', alignItems: 'center', gap: '6px' }}>
                          <span>{rule.appName}</span>
                          {isRunningNow && (
                            <span style={{
                              width: '6px',
                              height: '6px',
                              borderRadius: '50%',
                              backgroundColor: '#10b981',
                              display: 'inline-block',
                            }} title="Process is currently running" />
                          )}
                        </div>
                        {rule.appPath && (
                          <div className="font-mono" style={{ fontSize: '10px', color: '#64748b' }}>
                            {rule.appPath}
                          </div>
                        )}
                      </td>

                      <td>
                        <span style={{
                          fontSize: '10.5px',
                          fontWeight: 600,
                          padding: '2px 6px',
                          borderRadius: '4px',
                          background: 'rgba(2, 132, 199, 0.15)',
                          color: '#38bdf8',
                          border: '1px solid rgba(56, 189, 248, 0.25)',
                        }}>
                          → {formatRouteSummary(rule)}
                        </span>
                      </td>

                      <td className="font-mono" style={{ fontSize: '10.5px', color: '#94a3b8' }}>
                        {rule.bandwidthLimitMbps > 0 ? `${rule.bandwidthLimitMbps} Mbps` : 'No cap'}
                        {rule.customDns ? ` • DNS: ${rule.customDns}` : ''}
                      </td>

                      <td>
                        <button
                          onClick={e => handleToggleRule(rule, e)}
                          className={`status-pill ${rule.enabled ? 'pass' : 'blocked'}`}
                          style={{ cursor: 'pointer', border: 'none' }}
                        >
                          {rule.enabled ? 'ACTIVE' : 'DISABLED'}
                        </button>
                      </td>

                      <td style={{ textAlign: 'right' }}>
                        <button
                          onClick={e => handleDeleteRule(rule.id, e)}
                          style={{
                            background: 'transparent',
                            border: 'none',
                            color: '#f87171',
                            cursor: 'pointer',
                            padding: '4px',
                          }}
                          title="Remove App Policy"
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
              <Activity size={14} color="#10b981" />
              <span>Running Host Processes ({apps.length})</span>
            </div>

            <div style={{ position: 'relative', width: '170px' }}>
              <Search size={12} color="#64748b" style={{ position: 'absolute', left: '8px', top: '7px' }} />
              <input
                type="text"
                placeholder="Filter processes..."
                value={appSearch}
                onChange={e => setAppSearch(e.target.value)}
                style={{ width: '100%', paddingLeft: '24px', paddingRight: '8px', paddingTop: '4px', paddingBottom: '4px', fontSize: '11px' }}
              />
            </div>
          </div>

          <div style={{ maxHeight: '340px', overflowY: 'auto' }}>
            <table className="data-table">
              <thead>
                <tr>
                  <th>Process</th>
                  <th>PID</th>
                  <th>Active Policy</th>
                  <th style={{ textAlign: 'right' }}>Configure</th>
                </tr>
              </thead>
              <tbody>
                {filteredApps.slice(0, 50).map(app => {
                  const matchedRule = getAppRule(app.name);
                  return (
                    <tr key={`${app.pid}-${app.name}`}>
                      <td>
                        <div style={{ fontWeight: 600, color: '#f1f5f9' }}>{app.name}</div>
                        {app.socketCount !== undefined && app.socketCount > 0 && (
                          <div style={{ fontSize: '10px', color: '#64748b' }}>
                            {app.socketCount} open sockets
                          </div>
                        )}
                      </td>
                      <td className="font-mono" style={{ fontSize: '11px', color: '#64748b' }}>
                        {app.pid}
                      </td>
                      <td>
                        {matchedRule ? (
                          <span style={{ fontSize: '10.5px', color: '#38bdf8', fontWeight: 600 }}>
                            {formatRouteSummary(matchedRule)}
                          </span>
                        ) : (
                          <span style={{ fontSize: '10.5px', color: '#64748b' }}>
                            Default ({defaultIfaceName})
                          </span>
                        )}
                      </td>
                      <td style={{ textAlign: 'right' }}>
                        <button
                          className="btn btn-secondary btn-sm"
                          style={{ padding: '3px 8px', fontSize: '10.5px' }}
                          onClick={() => openModalForRule(matchedRule, app.name, app.executable)}
                        >
                          {matchedRule ? 'Modify' : '+ Add Rule'}
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
          background: 'rgba(0, 0, 0, 0.72)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 1000,
        }}>
          <div style={{
            background: '#121620',
            border: '1px solid #263042',
            borderRadius: '8px',
            width: '440px',
            padding: '16px 18px',
            display: 'flex',
            flexDirection: 'column',
            gap: '12px',
            boxShadow: '0 12px 32px rgba(0, 0, 0, 0.5)',
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3 style={{ fontSize: '13.5px', fontWeight: 700, color: '#f1f5f9' }}>
                {editingRuleId ? `Modify Policy: ${modalApp}` : 'Add Application Routing Policy'}
              </h3>
              <button
                onClick={() => setShowModal(false)}
                style={{ background: 'transparent', border: 'none', color: '#94a3b8', cursor: 'pointer' }}
              >
                <X size={15} />
              </button>
            </div>

            <div>
              <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '4px' }}>
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
              <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '4px' }}>
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
                <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '4px' }}>
                  Preferred Network Adapter
                </label>
                <select
                  value={modalPreferred}
                  onChange={e => setModalPreferred(e.target.value)}
                  style={{ width: '100%' }}
                >
                  <option value="FASTEST">Fastest Adapter (Lowest Latency)</option>
                  <option value="ALL">Multi-WAN Merged (All Adapters)</option>
                  {interfaces.map(i => (
                    <option key={i.name} value={i.name}>
                      {i.name} ({i.ip || 'No IP'})
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '4px' }}>
                  Failover Backup Adapter
                </label>
                <select
                  value={modalFallback}
                  onChange={e => setModalFallback(e.target.value)}
                  style={{ width: '100%' }}
                >
                  <option value="">None (System Default)</option>
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
                <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '4px' }}>
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
                <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '4px' }}>
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
              <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '4px' }}>
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
                Save Application Policy
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
