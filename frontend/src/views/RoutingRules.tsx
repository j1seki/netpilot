import React, { useEffect, useState } from 'react';
import { api, RuleRecord, InterfaceInfo } from '../api/wails';
import { Plus, Trash2, ArrowRight, ToggleLeft, ToggleRight, Zap, Globe, Gauge, ShieldAlert } from 'lucide-react';

export const RoutingRules: React.FC = () => {
  const [rules, setRules] = useState<RuleRecord[]>([]);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [isModalOpen, setIsModalOpen] = useState(false);

  const [formId, setFormId] = useState('');
  const [formApp, setFormApp] = useState('');
  const [formDomain, setFormDomain] = useState('');
  const [formPrefNic, setFormPrefNic] = useState('net-merge');
  const [formFbNic, setFormFbNic] = useState('');
  const [formPriority, setFormPriority] = useState(50);
  const [formLimitMbps, setFormLimitMbps] = useState(0);
  const [formDataQuotaMb, setFormDataQuotaMb] = useState(0);
  const [formCustomDns, setFormCustomDns] = useState('');

  const loadData = async () => {
    const [rList, ifaces] = await Promise.all([
      api.getRules(),
      api.getInterfaces(),
    ]);
    setRules(rList);
    setInterfaces(ifaces);
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleToggle = async (r: RuleRecord) => {
    const updated = { ...r, enabled: !r.enabled };
    await api.saveRule(updated);
    loadData();
  };

  const handleDelete = async (id: string) => {
    await api.deleteRule(id);
    loadData();
  };

  const handleSave = async () => {
    const newRule: RuleRecord = {
      id: formId || `rule_${Date.now()}`,
      appName: formApp,
      appPath: '',
      domainPattern: formDomain,
      preferredInterface: formPrefNic,
      fallbackInterface: formFbNic,
      priority: formPriority,
      bandwidthLimitMbps: formLimitMbps,
      dataQuotaMb: formDataQuotaMb,
      dataUsedMb: 0,
      customDns: formCustomDns,
      enabled: true,
    };
    await api.saveRule(newRule);
    setIsModalOpen(false);
    resetForm();
    loadData();
  };

  const resetForm = () => {
    setFormId('');
    setFormApp('');
    setFormDomain('');
    setFormPrefNic('net-merge');
    setFormFbNic('');
    setFormPriority(50);
    setFormLimitMbps(0);
    setFormDataQuotaMb(0);
    setFormCustomDns('');
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h2 style={{ fontSize: '14.5px', fontWeight: 700 }}>Traffic Routing Rules & Policies</h2>
          <p style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
            Interface steering, fastest-adapter dynamic routing, speed limits, data quotas, and custom per-app DNS.
          </p>
        </div>
        <button
          className="btn btn-primary btn-sm"
          onClick={() => {
            resetForm();
            setIsModalOpen(true);
          }}
          style={{ padding: '6px 12px', fontSize: '11.5px' }}
        >
          <Plus size={13} /> New Routing Policy
        </button>
      </div>

      <div className="card" style={{ marginBottom: 0 }}>
        <table className="data-table">
          <thead>
            <tr>
              <th>Priority</th>
              <th>Application / Pattern</th>
              <th>Preferred Link</th>
              <th>Fallback Link</th>
              <th>Speed Limit & Quota</th>
              <th>DNS Resolver</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {rules.length === 0 ? (
              <tr>
                <td colSpan={8} style={{ textAlign: 'center', padding: '32px', color: 'var(--text-muted)' }}>
                  No routing policies configured yet. Click "+ New Routing Policy" to create one.
                </td>
              </tr>
            ) : (
              rules.map(r => (
                <tr key={r.id} style={{ opacity: r.enabled ? 1 : 0.5 }}>
                  <td className="font-mono" style={{ fontWeight: 700, color: '#60a5fa' }}>
                    {r.priority}
                  </td>
                  <td>
                    <div style={{ fontWeight: 600 }}>{r.appName || 'All Applications'}</div>
                    {r.domainPattern && (
                      <div className="font-mono" style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                        Dest: {r.domainPattern}
                      </div>
                    )}
                  </td>
                  <td>
                    {r.preferredInterface === 'FASTEST' ? (
                      <span style={{ background: 'rgba(234, 179, 8, 0.2)', color: '#facc15', padding: '3px 8px', borderRadius: '4px', fontSize: '11.5px', fontWeight: 700, display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                        <Zap size={12} /> FASTEST (Lowest RTT)
                      </span>
                    ) : (
                      <span style={{ background: 'rgba(59, 130, 246, 0.15)', color: '#93c5fd', padding: '3px 8px', borderRadius: '4px', fontSize: '12px', fontWeight: 600 }}>
                        {r.preferredInterface}
                      </span>
                    )}
                  </td>
                  <td>
                    {r.fallbackInterface ? (
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', color: 'var(--text-secondary)', fontSize: '12px' }}>
                        <ArrowRight size={11} /> {r.fallbackInterface}
                      </span>
                    ) : (
                      <span style={{ color: 'var(--text-muted)', fontSize: '11px' }}>None (Direct)</span>
                    )}
                  </td>
                  <td>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                      <span className="font-mono" style={{ fontSize: '12px', display: 'flex', alignItems: 'center', gap: '4px' }}>
                        <Gauge size={12} color="#94a3b8" />
                        {r.bandwidthLimitMbps > 0 ? `${r.bandwidthLimitMbps} Mbps` : 'Unlimited speed'}
                      </span>
                      {r.dataQuotaMb > 0 ? (
                        <span className="font-mono" style={{ fontSize: '11px', color: '#f59e0b', display: 'flex', alignItems: 'center', gap: '4px' }}>
                          <ShieldAlert size={11} />
                          Quota: {r.dataQuotaMb >= 1024 ? `${(r.dataQuotaMb / 1024).toFixed(1)} GB` : `${r.dataQuotaMb} MB`}
                        </span>
                      ) : (
                        <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>No data cap</span>
                      )}
                    </div>
                  </td>
                  <td>
                    {r.customDns ? (
                      <span className="font-mono" style={{ background: 'rgba(16, 185, 129, 0.15)', color: '#6ee7b7', padding: '3px 7px', borderRadius: '4px', fontSize: '11.5px', display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                        <Globe size={11} /> {r.customDns}
                      </span>
                    ) : (
                      <span style={{ color: 'var(--text-muted)', fontSize: '11px' }}>System Default</span>
                    )}
                  </td>
                  <td>
                    <button
                      onClick={() => handleToggle(r)}
                      style={{ background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center' }}
                    >
                      {r.enabled ? <ToggleRight size={22} color="#10b981" /> : <ToggleLeft size={22} color="#64748b" />}
                    </button>
                  </td>
                  <td>
                    <button
                      onClick={() => handleDelete(r.id)}
                      style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#ef4444' }}
                      title="Delete Rule"
                    >
                      <Trash2 size={14} />
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* New Policy Modal */}
      {isModalOpen && (
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
            width: '520px',
            boxShadow: '0 20px 40px rgba(0,0,0,0.5)',
            maxHeight: '90vh',
            overflowY: 'auto',
          }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: '6px' }}>
              Create Network Routing Policy
            </h3>
            <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '16px' }}>
              Route traffic from specified applications or domains to a designated network link with speed limits, data quotas, and custom DNS.
            </p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '18px' }}>
              <div>
                <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                  Application Name
                </label>
                <input
                  type="text"
                  placeholder="e.g. Zoom, Chrome, Slack, qBittorrent, tailscaled"
                  value={formApp}
                  onChange={e => setFormApp(e.target.value)}
                  style={{ width: '100%', padding: '8px 12px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'white', fontSize: '13px' }}
                />
              </div>

              <div>
                <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                  Domain Pattern (Optional)
                </label>
                <input
                  type="text"
                  placeholder="e.g. *.ts.net, *.github.com, 10.0.0.0/8"
                  value={formDomain}
                  onChange={e => setFormDomain(e.target.value)}
                  style={{ width: '100%', padding: '8px 12px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'white', fontSize: '13px' }}
                />
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                <div>
                  <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Preferred Interface
                  </label>
                  <select
                    value={formPrefNic}
                    onChange={e => setFormPrefNic(e.target.value)}
                    style={{ width: '100%' }}
                  >
                    <option value="net-merge">net-merge (Default Merged Adapter)</option>
                    <option value="FASTEST">FASTEST (Lowest Latency Adapter)</option>
                    {interfaces.map(i => (
                      <option key={i.name} value={i.name}>{i.name} ({i.ip || 'No IP'})</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Fallback Interface
                  </label>
                  <select
                    value={formFbNic}
                    onChange={e => setFormFbNic(e.target.value)}
                    style={{ width: '100%', padding: '8px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'white', fontSize: '13px' }}
                  >
                    <option value="">None (No Fallback)</option>
                    {interfaces.map(i => (
                      <option key={i.name} value={i.name}>{i.name}</option>
                    ))}
                  </select>
                </div>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                <div>
                  <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Speed Limit (Mbps, 0=unlimited)
                  </label>
                  <input
                    type="number"
                    min="0"
                    step="0.5"
                    value={formLimitMbps}
                    onChange={e => setFormLimitMbps(parseFloat(e.target.value) || 0)}
                    style={{ width: '100%', padding: '8px 12px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'white', fontSize: '13px' }}
                  />
                </div>

                <div>
                  <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Data Cap (MB, 0=unlimited)
                  </label>
                  <input
                    type="number"
                    min="0"
                    step="100"
                    value={formDataQuotaMb}
                    onChange={e => setFormDataQuotaMb(parseFloat(e.target.value) || 0)}
                    style={{ width: '100%', padding: '8px 12px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'white', fontSize: '13px' }}
                  />
                </div>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                <div>
                  <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Custom DNS Resolver per App
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. 1.1.1.1, 8.8.8.8, 9.9.9.9"
                    value={formCustomDns}
                    onChange={e => setFormCustomDns(e.target.value)}
                    style={{ width: '100%', padding: '8px 12px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'white', fontSize: '13px' }}
                  />
                  <div style={{ display: 'flex', gap: '6px', marginTop: '4px' }}>
                    <button
                      type="button"
                      onClick={() => setFormCustomDns('1.1.1.1')}
                      style={{ fontSize: '10px', padding: '2px 6px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '3px', color: '#60a5fa', cursor: 'pointer' }}
                    >
                      Cloudflare (1.1.1.1)
                    </button>
                    <button
                      type="button"
                      onClick={() => setFormCustomDns('8.8.8.8')}
                      style={{ fontSize: '10px', padding: '2px 6px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '3px', color: '#60a5fa', cursor: 'pointer' }}
                    >
                      Google (8.8.8.8)
                    </button>
                    <button
                      type="button"
                      onClick={() => setFormCustomDns('9.9.9.9')}
                      style={{ fontSize: '10px', padding: '2px 6px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '3px', color: '#60a5fa', cursor: 'pointer' }}
                    >
                      Quad9
                    </button>
                  </div>
                </div>

                <div>
                  <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Priority (1-100)
                  </label>
                  <input
                    type="number"
                    min="1"
                    max="100"
                    value={formPriority}
                    onChange={e => setFormPriority(parseInt(e.target.value) || 10)}
                    style={{ width: '100%', padding: '8px 12px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'white', fontSize: '13px' }}
                  />
                </div>
              </div>
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
              <button className="btn btn-secondary btn-sm" onClick={() => setIsModalOpen(false)}>
                Cancel
              </button>
              <button className="btn btn-primary btn-sm" onClick={handleSave}>
                Save Policy
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
