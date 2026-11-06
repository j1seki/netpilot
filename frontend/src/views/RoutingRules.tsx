import React, { useEffect, useState } from 'react';
import { api, RuleRecord, InterfaceInfo } from '../api/wails';
import {
  Plus,
  Trash2,
  ArrowRight,
  ToggleLeft,
  ToggleRight,
  Zap,
  Globe,
  Gauge,
  ShieldAlert,
  Pencil,
  Copy,
  Info,
} from 'lucide-react';

interface ExampleRule {
  key: string;
  appName: string;
  domainPattern: string;
  preferredInterface: string;
  fallbackInterface: string;
  priority: number;
  bandwidthLimitMbps: number;
  dataQuotaMb: number;
  customDns: string;
  description: string;
}

// Examples are deliberately not written to the database until the user clicks
// "Add as disabled". They teach what a policy does without silently changing
// routing on first launch.
const EXAMPLE_RULES: ExampleRule[] = [
  {
    key: 'video-calls',
    appName: 'Zoom / Teams',
    domainPattern: '*.zoom.us, *.teams.microsoft.com',
    preferredInterface: 'FASTEST',
    fallbackInterface: 'net-merge',
    priority: 90,
    bandwidthLimitMbps: 0,
    dataQuotaMb: 0,
    customDns: '',
    description: 'Keep video calls on the lowest-latency link; if that link fails, fall back to the merged adapter.',
  },
  {
    key: 'streaming',
    appName: 'Streaming apps',
    domainPattern: '*.netflix.com, *.googlevideo.com, *.primevideo.com',
    preferredInterface: 'net-merge',
    fallbackInterface: '',
    priority: 50,
    bandwidthLimitMbps: 0,
    dataQuotaMb: 0,
    customDns: '',
    description: 'Spread streaming traffic across every healthy adapter for maximum combined throughput.',
  },
  {
    key: 'large-downloads',
    appName: 'Steam / qBittorrent',
    domainPattern: '',
    preferredInterface: 'net-merge',
    fallbackInterface: '',
    priority: 30,
    bandwidthLimitMbps: 50,
    dataQuotaMb: 0,
    customDns: '',
    description: 'Use the merged link but cap it at 50 Mbps so large downloads do not starve browsing or calls.',
  },
];

export const RoutingRules: React.FC = () => {
  const [rules, setRules] = useState<RuleRecord[]>([]);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [notice, setNotice] = useState('');
  const [formError, setFormError] = useState('');

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
    const [rList, ifaces] = await Promise.allSettled([
      api.getRules(),
      api.getInterfaces(),
    ]);
    if (rList.status === 'fulfilled') setRules(rList.value);
    if (ifaces.status === 'fulfilled') setInterfaces(ifaces.value);
  };

  useEffect(() => {
    loadData().catch((e) => console.error(e));
  }, []);

  const flashNotice = (msg: string) => {
    setNotice(msg);
    setTimeout(() => setNotice(''), 5000);
  };

  const handleToggle = async (r: RuleRecord) => {
    try {
      await api.saveRule({ ...r, enabled: !r.enabled });
      await loadData();
    } catch (err: any) {
      flashNotice(`Could not update policy: ${err?.message || String(err)}`);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.deleteRule(id);
      await loadData();
    } catch (err: any) {
      flashNotice(`Could not delete policy: ${err?.message || String(err)}`);
    }
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
    setFormError('');
  };

  const openEdit = (r: RuleRecord) => {
    setFormId(r.id);
    setFormApp(r.appName);
    setFormDomain(r.domainPattern || '');
    setFormPrefNic(r.preferredInterface || 'net-merge');
    setFormFbNic(r.fallbackInterface || '');
    setFormPriority(r.priority || 50);
    setFormLimitMbps(r.bandwidthLimitMbps || 0);
    setFormDataQuotaMb(r.dataQuotaMb || 0);
    setFormCustomDns(r.customDns || '');
    setFormError('');
    setIsModalOpen(true);
  };

  const handleSave = async () => {
    const appName = formApp.trim();
    if (!appName) {
      setFormError('An application or process name is required.');
      return;
    }
    const existing = rules.find(r => r.id === formId);
    const newRule: RuleRecord = {
      id: formId || `rule_${Date.now()}`,
      appName,
      appPath: existing?.appPath || '',
      domainPattern: formDomain.trim(),
      preferredInterface: formPrefNic,
      fallbackInterface: formFbNic,
      priority: formPriority,
      bandwidthLimitMbps: formLimitMbps,
      dataQuotaMb: formDataQuotaMb,
      dataUsedMb: existing?.dataUsedMb || 0,
      customDns: formCustomDns.trim(),
      // Editing preserves the current state; new policies start enabled.
      enabled: existing ? existing.enabled : true,
    };
    try {
      await api.saveRule(newRule);
      setIsModalOpen(false);
      resetForm();
      flashNotice(existing ? `Updated policy for ${appName}.` : `Created policy for ${appName}.`);
      await loadData();
    } catch (err: any) {
      setFormError(err?.message || String(err));
    }
  };

  const addExample = async (ex: ExampleRule) => {
    const rule: RuleRecord = {
      id: `example_${ex.key}_${Date.now()}`,
      appName: ex.appName,
      appPath: '',
      domainPattern: ex.domainPattern,
      preferredInterface: ex.preferredInterface,
      fallbackInterface: ex.fallbackInterface,
      priority: ex.priority,
      bandwidthLimitMbps: ex.bandwidthLimitMbps,
      dataQuotaMb: ex.dataQuotaMb,
      dataUsedMb: 0,
      customDns: ex.customDns,
      // Added disabled on purpose: the user reviews it before turning it on.
      enabled: false,
    };
    try {
      await api.saveRule(rule);
      flashNotice(`Added "${ex.appName}" as a disabled policy. Review it, then toggle it on.`);
      await loadData();
    } catch (err: any) {
      flashNotice(`Could not add example: ${err?.message || String(err)}`);
    }
  };

  const adapterLabel = (name: string) => {
    if (name === 'net-merge') return 'net-merge (Default Merged Adapter)';
    if (name === 'FASTEST') return 'FASTEST (Lowest Latency Adapter)';
    const info = interfaces.find(i => i.name === name);
    return `${name}${info?.isDefault ? ' · OS default' : ''}${info?.isVirtual ? ' · VPN/virtual' : ''}`;
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      {notice && (
        <div style={{
          padding: '9px 12px',
          borderRadius: '7px',
          background: 'rgba(56, 189, 248, 0.12)',
          border: '1px solid rgba(56, 189, 248, 0.3)',
          color: '#7dd3fc',
          fontSize: '11.5px',
          display: 'flex',
          alignItems: 'center',
          gap: '7px',
        }}>
          <Info size={14} />
          <span>{notice}</span>
        </div>
      )}

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
                  No routing policies configured yet. Click "+ New Routing Policy" or add a ready-made example below.
                </td>
              </tr>
            ) : (
              rules.map(r => (
                <tr key={r.id} style={{ opacity: r.enabled ? 1 : 0.62 }}>
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
                      title={r.enabled ? 'Disable this policy' : 'Enable this policy'}
                      aria-label={r.enabled ? 'Disable policy' : 'Enable policy'}
                    >
                      {r.enabled ? <ToggleRight size={22} color="#10b981" /> : <ToggleLeft size={22} color="#64748b" />}
                    </button>
                  </td>
                  <td>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                      <button
                        onClick={() => openEdit(r)}
                        style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#93c5fd', padding: '3px' }}
                        title="Edit policy"
                        aria-label="Edit policy"
                      >
                        <Pencil size={13} />
                      </button>
                      <button
                        onClick={() => handleDelete(r.id)}
                        style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#ef4444', padding: '3px' }}
                        title="Delete policy"
                        aria-label="Delete policy"
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Ready-made examples, always disabled until explicitly added */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Copy size={14} color="#38bdf8" /> Example Policies (disabled by default)
          </span>
          <span className="status-pill blocked">NOT ACTIVE</span>
        </div>
        <p style={{ fontSize: '11px', color: 'var(--text-secondary)', marginBottom: '10px' }}>
          These are examples only. Click <strong>Add as disabled</strong> to copy one into your policy list,
          review the details, then enable it when you are ready.
        </p>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          {EXAMPLE_RULES.map(ex => (
            <div
              key={ex.key}
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                gap: '12px',
                padding: '9px 11px',
                background: '#09090b',
                border: '1px dashed #27272a',
                borderRadius: '6px',
              }}
            >
              <div style={{ display: 'flex', flexDirection: 'column', gap: '2px', minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '7px', flexWrap: 'wrap' }}>
                  <span style={{ fontWeight: 700, fontSize: '12px', color: '#fafafa' }}>{ex.appName}</span>
                  <span className="status-pill blocked" style={{ fontSize: '9.5px' }}>EXAMPLE</span>
                  <span style={{ fontSize: '10.5px', color: '#93c5fd' }}>
                    {ex.preferredInterface === 'FASTEST' ? 'FASTEST' : ex.preferredInterface}
                    {ex.fallbackInterface ? ` → ${ex.fallbackInterface}` : ''}
                  </span>
                  {ex.bandwidthLimitMbps > 0 && (
                    <span style={{ fontSize: '10.5px', color: '#f59e0b' }}>{ex.bandwidthLimitMbps} Mbps cap</span>
                  )}
                </div>
                <div style={{ fontSize: '10.5px', color: '#a1a1aa' }}>{ex.description}</div>
                {ex.domainPattern && (
                  <div className="font-mono" style={{ fontSize: '10px', color: '#71717a' }}>{ex.domainPattern}</div>
                )}
              </div>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => addExample(ex)}
                style={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: '5px' }}
                title="Copy this example into your policies with the toggle OFF"
              >
                <Plus size={12} /> Add as disabled
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* New / Edit Policy Modal */}
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
              {formId ? 'Edit Network Routing Policy' : 'Create Network Routing Policy'}
            </h3>
            <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '16px' }}>
              Route traffic from specified applications or domains to a designated network link with speed limits, data quotas, and custom DNS.
            </p>

            {formError && (
              <div style={{
                marginBottom: '12px',
                padding: '7px 10px',
                borderRadius: '5px',
                background: 'rgba(244, 63, 94, 0.14)',
                border: '1px solid rgba(244, 63, 94, 0.3)',
                color: '#fb7185',
                fontSize: '11.5px',
              }}>
                {formError}
              </div>
            )}

            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '18px' }}>
              <div>
                <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                  Application Name *
                </label>
                <input
                  type="text"
                  placeholder="e.g. Zoom, Chrome, Slack, qBittorrent, tailscaled"
                  value={formApp}
                  onChange={e => setFormApp(e.target.value)}
                  style={{ width: '100%' }}
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
                  style={{ width: '100%' }}
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
                      <option key={i.name} value={i.name}>{adapterLabel(i.name)}</option>
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
                    style={{ width: '100%' }}
                  >
                    <option value="">None (No Fallback)</option>
                    {interfaces.map(i => (
                      <option key={i.name} value={i.name}>{adapterLabel(i.name)}</option>
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
                    style={{ width: '100%' }}
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
                    style={{ width: '100%' }}
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
                    style={{ width: '100%' }}
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
                    style={{ width: '100%' }}
                  />
                </div>
              </div>
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
              <button className="btn btn-secondary btn-sm" onClick={() => setIsModalOpen(false)}>
                Cancel
              </button>
              <button
                className="btn btn-primary btn-sm"
                onClick={handleSave}
                disabled={!formApp.trim()}
                title={!formApp.trim() ? 'Enter an application name first' : 'Save policy'}
              >
                {formId ? 'Save Changes' : 'Save Policy'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
