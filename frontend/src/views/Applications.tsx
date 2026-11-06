import React, { useEffect, useState } from 'react';
import { api, ProcessInfo, InterfaceInfo } from '../api/wails';
import { Search, Globe, Video, MessageSquare, DownloadCloud, Cpu, RefreshCw, SlidersHorizontal, Zap, Gauge, ShieldAlert } from 'lucide-react';

export const Applications: React.FC = () => {
  const [apps, setApps] = useState<ProcessInfo[]>([]);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [search, setSearch] = useState('');
  const [selectedApp, setSelectedApp] = useState<ProcessInfo | null>(null);
  const [rulePrefNic, setRulePrefNic] = useState('FASTEST');
  const [ruleLimitMbps, setRuleLimitMbps] = useState(0);
  const [ruleDataQuotaMb, setRuleDataQuotaMb] = useState(0);
  const [ruleCustomDns, setRuleCustomDns] = useState('');

  const loadData = async () => {
    const [appList, ifaces] = await Promise.all([
      api.getApplications(),
      api.getInterfaces(),
    ]);
    setApps(appList);
    setInterfaces(ifaces);
  };

  useEffect(() => {
    loadData().catch((e) => console.error(e));
    const interval = setInterval(() => loadData().catch((e) => console.error(e)), 3000);
    return () => clearInterval(interval);
  }, []);

  const getIcon = (hint: string) => {
    switch (hint) {
      case 'globe': return <Globe size={16} color="#38bdf8" />;
      case 'video': return <Video size={16} color="#4ade80" />;
      case 'message-square': return <MessageSquare size={16} color="#fbbf24" />;
      case 'download-cloud': return <DownloadCloud size={16} color="#c084fc" />;
      default: return <Cpu size={16} color="#94a3b8" />;
    }
  };

  const filtered = apps.filter(a =>
    a.name.toLowerCase().includes(search.toLowerCase()) ||
    a.executable.toLowerCase().includes(search.toLowerCase())
  );

  const handleCreateRule = async () => {
    if (!selectedApp) return;
    await api.saveRule({
      id: `rule_${selectedApp.name.toLowerCase().replace(/[^a-z0-9]/g, '_')}`,
      appName: selectedApp.name,
      appPath: selectedApp.executable,
      domainPattern: '',
      preferredInterface: rulePrefNic,
      fallbackInterface: rulePrefNic === 'FASTEST' ? '' : (interfaces.find(i => i.name !== rulePrefNic)?.name || ''),
      priority: 60,
      bandwidthLimitMbps: ruleLimitMbps,
      dataQuotaMb: ruleDataQuotaMb,
      dataUsedMb: 0,
      customDns: ruleCustomDns,
      enabled: true,
    });
    setSelectedApp(null);
    loadData().catch((e) => console.error(e));
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <div style={{ position: 'relative' }}>
            <Search size={14} color="var(--text-muted)" style={{ position: 'absolute', left: '10px', top: '8px' }} />
            <input
              type="text"
              placeholder="Search live processes..."
              value={search}
              onChange={e => setSearch(e.target.value)}
              style={{
                backgroundColor: 'var(--bg-secondary)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                padding: '6px 10px 6px 30px',
                color: 'var(--text-primary)',
                fontSize: '12px',
                width: '260px',
              }}
            />
          </div>
          <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
            Showing {filtered.length} running processes
          </span>
        </div>
        <button className="btn btn-secondary btn-sm" onClick={loadData} style={{ padding: '5px 10px', fontSize: '11.5px' }}>
          <RefreshCw size={12} /> Refresh
        </button>
      </div>

      <div className="card" style={{ marginBottom: 0 }}>
        <table className="data-table">
          <thead>
            <tr>
              <th>App Name</th>
              <th>PID / Sockets</th>
              <th>Assigned Interface</th>
              <th>Speed Limit & Quota</th>
              <th>DNS Resolver</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map(app => (
              <tr key={app.pid}>
                <td>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                    {getIcon(app.iconHint)}
                    <div>
                      <div style={{ fontWeight: 600 }}>{app.name}</div>
                      <div className="font-mono" style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                        {app.executable}
                      </div>
                    </div>
                  </div>
                </td>
                <td className="font-mono" style={{ color: 'var(--text-muted)', fontSize: '12px' }}>
                  PID {app.pid} {app.socketCount !== undefined ? `• ${app.socketCount} sockets` : ''}
                </td>
                <td>
                  <span style={{
                    background: app.assignedNic === 'FASTEST' ? 'rgba(234, 179, 8, 0.2)' : 'rgba(59, 130, 246, 0.15)',
                    color: app.assignedNic === 'FASTEST' ? '#facc15' : '#93c5fd',
                    padding: '3px 8px',
                    borderRadius: '4px',
                    fontSize: '12px',
                    fontWeight: 600,
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '4px',
                  }}>
                    {app.assignedNic === 'FASTEST' ? <Zap size={11} /> : null}
                    {app.assignedNic}
                  </span>
                  {app.ruleMatched && (
                    <span style={{ marginLeft: '6px', fontSize: '10.5px', color: '#c084fc', display: 'block', marginTop: '2px' }}>
                      (Policy Match: {app.ruleMatched})
                    </span>
                  )}
                </td>
                <td>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                    <span className="font-mono" style={{ fontSize: '11.5px', display: 'flex', alignItems: 'center', gap: '4px' }}>
                      <Gauge size={11} color="#94a3b8" />
                      {app.bandwidthLimitMbps && app.bandwidthLimitMbps > 0 ? `${app.bandwidthLimitMbps} Mbps` : 'Unlimited'}
                    </span>
                    {app.dataQuotaMb && app.dataQuotaMb > 0 ? (
                      <span className="font-mono" style={{ fontSize: '11px', color: app.quotaExceeded ? '#ef4444' : '#f59e0b', display: 'flex', alignItems: 'center', gap: '4px' }}>
                        <ShieldAlert size={11} />
                        {app.quotaExceeded ? 'EXCEEDED' : `Quota: ${app.dataQuotaMb} MB`}
                      </span>
                    ) : null}
                  </div>
                </td>
                <td>
                  {app.customDns ? (
                    <span className="font-mono" style={{ background: 'rgba(16, 185, 129, 0.15)', color: '#6ee7b7', padding: '2px 6px', borderRadius: '4px', fontSize: '11px', display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                      <Globe size={11} /> {app.customDns}
                    </span>
                  ) : (
                    <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>System Default</span>
                  )}
                </td>
                <td>
                  <button
                    className="btn btn-secondary btn-sm"
                    onClick={() => {
                      setSelectedApp(app);
                      setRulePrefNic(app.assignedNic || 'FASTEST');
                      setRuleLimitMbps(app.bandwidthLimitMbps || 0);
                      setRuleDataQuotaMb(app.dataQuotaMb || 0);
                      setRuleCustomDns(app.customDns || '');
                    }}
                  >
                    <SlidersHorizontal size={12} /> Assign Route
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Route Assignment Modal */}
      {selectedApp && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: 'rgba(0,0,0,0.7)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 999,
        }}>
          <div style={{
            backgroundColor: 'var(--bg-secondary)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-lg)',
            padding: '24px',
            width: '480px',
            boxShadow: '0 20px 40px rgba(0,0,0,0.5)',
          }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: '6px' }}>
              Assign Network Route for {selectedApp.name}
            </h3>
            <p style={{ fontSize: '12.5px', color: 'var(--text-secondary)', marginBottom: '16px' }}>
              Create an explicit policy setting interface steering, speed limits, data quota, and custom DNS for this process.
            </p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '20px' }}>
              <div>
                <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                  Preferred Network Interface
                </label>
                <select
                  value={rulePrefNic}
                  onChange={e => setRulePrefNic(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    backgroundColor: 'var(--bg-tertiary)',
                    border: '1px solid var(--border)',
                    borderRadius: 'var(--radius-sm)',
                    color: 'white',
                    fontSize: '13px',
                  }}
                >
                  <option value="FASTEST">⚡ FASTEST (Lowest Latency Adapter)</option>
                  {interfaces.map(i => (
                    <option key={i.name} value={i.name}>{i.name} ({i.ip || 'No IP'})</option>
                  ))}
                </select>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                <div>
                  <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Speed Limit (Mbps, 0=unlimited)
                  </label>
                  <input
                    type="number"
                    min="0"
                    step="1"
                    value={ruleLimitMbps}
                    onChange={e => setRuleLimitMbps(parseFloat(e.target.value) || 0)}
                    style={{
                      width: '100%',
                      padding: '8px 12px',
                      backgroundColor: 'var(--bg-tertiary)',
                      border: '1px solid var(--border)',
                      borderRadius: 'var(--radius-sm)',
                      color: 'white',
                      fontSize: '13px',
                    }}
                  />
                </div>

                <div>
                  <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Data Cap (MB, 0=unlimited)
                  </label>
                  <input
                    type="number"
                    min="0"
                    step="500"
                    value={ruleDataQuotaMb}
                    onChange={e => setRuleDataQuotaMb(parseFloat(e.target.value) || 0)}
                    style={{
                      width: '100%',
                      padding: '8px 12px',
                      backgroundColor: 'var(--bg-tertiary)',
                      border: '1px solid var(--border)',
                      borderRadius: 'var(--radius-sm)',
                      color: 'white',
                      fontSize: '13px',
                    }}
                  />
                </div>
              </div>

              <div>
                <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                  Custom DNS Resolver per App
                </label>
                <input
                  type="text"
                  placeholder="e.g. 1.1.1.1, 8.8.8.8, 9.9.9.9"
                  value={ruleCustomDns}
                  onChange={e => setRuleCustomDns(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    backgroundColor: 'var(--bg-tertiary)',
                    border: '1px solid var(--border)',
                    borderRadius: 'var(--radius-sm)',
                    color: 'white',
                    fontSize: '13px',
                  }}
                />
                <div style={{ display: 'flex', gap: '6px', marginTop: '4px' }}>
                  <button
                    type="button"
                    onClick={() => setRuleCustomDns('1.1.1.1')}
                    style={{ fontSize: '10px', padding: '2px 6px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '3px', color: '#60a5fa', cursor: 'pointer' }}
                  >
                    1.1.1.1 (Cloudflare)
                  </button>
                  <button
                    type="button"
                    onClick={() => setRuleCustomDns('8.8.8.8')}
                    style={{ fontSize: '10px', padding: '2px 6px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '3px', color: '#60a5fa', cursor: 'pointer' }}
                  >
                    8.8.8.8 (Google)
                  </button>
                  <button
                    type="button"
                    onClick={() => setRuleCustomDns('')}
                    style={{ fontSize: '10px', padding: '2px 6px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '3px', color: '#94a3b8', cursor: 'pointer' }}
                  >
                    Default
                  </button>
                </div>
              </div>
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
              <button className="btn btn-secondary btn-sm" onClick={() => setSelectedApp(null)}>
                Cancel
              </button>
              <button className="btn btn-primary btn-sm" onClick={handleCreateRule}>
                Save Routing Policy
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
