import React, { useEffect, useState } from 'react';
import { api, RuleRecord, InterfaceInfo, NetMergeStatus } from '../api/wails';
import { ExecutablePath } from '../components/ExecutablePath';
import { RuleEditor, type RuleDraft } from '../components/RuleEditor';
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
  priority: number;
  bandwidthLimitMbps: number;
  dataQuotaMb: number;
  customDns: string;
  description: string;
}

// Examples fill an editor draft. The user supplies the actual adapter before saving.
const EXAMPLE_RULES: ExampleRule[] = [
  {
    key: 'video-calls',
    appName: 'Zoom / Teams',
    domainPattern: '*.zoom.us, *.teams.microsoft.com',
    priority: 90,
    bandwidthLimitMbps: 0,
    dataQuotaMb: 0,
    customDns: '',
    description: 'Choose a low-latency link for video calls. Add a second adapter for failover.',
  },
  {
    key: 'streaming',
    appName: 'Streaming apps',
    domainPattern: '*.netflix.com, *.googlevideo.com, *.primevideo.com',
    priority: 50,
    bandwidthLimitMbps: 0,
    dataQuotaMb: 0,
    customDns: '',
    description: 'Keep streaming traffic on your chosen link.',
  },
  {
    key: 'large-downloads',
    appName: 'Steam / qBittorrent',
    domainPattern: '',
    priority: 30,
    bandwidthLimitMbps: 50,
    dataQuotaMb: 0,
    customDns: '',
    description: 'Cap large downloads at 50 Mbps to leave bandwidth for browsing and calls.',
  },
];

export const RoutingRules: React.FC<{ active: boolean; goToProcesses: () => void }> = ({ active, goToProcesses }) => {
  const [rules, setRules] = useState<RuleRecord[]>([]);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [draft, setDraft] = useState<RuleDraft | null>(null);
  const [notice, setNotice] = useState('');
  const [status, setStatus] = useState<NetMergeStatus | null>(null);

  const loadData = async () => {
    const [rList, ifaces, merge] = await Promise.allSettled([
      api.getRules(),
      api.getInterfaces(),
      api.getNetMergeStatus(),
    ]);
    if (rList.status === 'fulfilled') setRules(rList.value);
    if (ifaces.status === 'fulfilled') setInterfaces(ifaces.value);
    if (merge.status === 'fulfilled') setStatus(merge.value);
  };

  useEffect(() => {
    if (!active) return;
    void loadData();
    const interval = setInterval(loadData, 5000);
    return () => clearInterval(interval);
  }, [active]);

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
            Pin app or destination traffic to a detected adapter and configure limits or DNS.
          </p>
        </div>
        <button
          className="btn btn-primary btn-sm"
          onClick={() => setDraft({})}
          style={{ padding: '6px 12px', fontSize: '11.5px' }}
        >
          <Plus size={13} /> New Routing Policy
        </button>
      </div>

      <div className="card" style={{ marginBottom: 0 }}>
        <p className="rule-editor-note">To verify a live route, open a process in Inspect, select Routing rules, and run Test live route.</p>
        <button className="btn btn-secondary btn-sm" type="button" onClick={goToProcesses}>Open Active Processes</button>
      </div>
      <details className="card" style={{ marginBottom: 0 }}>
        <summary>Policy precedence and settings that interact</summary>
        <p>{status?.systemAppRules ? 'System-wide NetPilot forwarding applies matching rules to new flows.'
          : status?.socksRunning ? 'Rules apply to clients using the NetPilot SOCKS5 proxy.'
          : status?.isRunning ? 'The separate tunnel engine uses its own routing. Start the NetPilot SOCKS5 proxy to use these rules with supported clients.'
          : 'Rules are saved. Start the NetPilot system tunnel or SOCKS5 proxy to apply them to new connections.'}</p>
        <ul>
          <li>Higher-priority rules win. Application name, path and destination are independent matches.</li>
          <li>A pinned adapter takes precedence over Speed or Backup for new matching flows.</li>
          <li>Adapter inclusion, address family and link health can make a pin unusable. Add an explicit fallback to keep new flows working.</li>
          <li>A rule shares its speed cap across matched connections. Adapter caps also apply. Once usage reaches a rule’s quota, new matching connections are blocked.</li>
          <li>Custom DNS uses the selected routing policy. IPv6 policy can block IPv6 resolvers. OS resolver services, caches, VPN routes and encrypted DNS can bypass per-app DNS.</li>
          <li>Mobile and Net-Merge share MTU, IPv6, scheduling, and reachability probes. Restore previous reinstates the saved Mobile snapshot. Applying a full connection profile can also change routing mode.</li>
          <li>Custom reachability probes affect adapter health, including Fastest selection and Backup failover. Backup adapter choices set the failover order.</li>
        </ul>
      </details>

      <div className="card" style={{ marginBottom: 0, overflowX: 'auto' }}>
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
                  No routing policies configured yet. Click "+ New Routing Policy" or load an example below.
                </td>
              </tr>
            ) : (
              rules.map(r => (
                <tr key={r.id} style={{ opacity: r.enabled ? 1 : 0.62 }}>
                  <td className="font-mono" style={{ fontWeight: 700, color: '#60a5fa' }}>
                    {r.priority}
                  </td>
                  <td>
                    <div style={{ fontWeight: 600 }}>{r.appName || (r.appPath ? 'Executable match' : 'Destination match')}</div>
                    <ExecutablePath path={r.appPath} />
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
                      <span style={{ color: 'var(--text-muted)', fontSize: '11px' }}>None</span>
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
                        onClick={() => setDraft({ rule: r })}
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

        </div>
        <p style={{ fontSize: '11px', color: 'var(--text-secondary)', marginBottom: '10px' }}>
          Load an example, select an adapter and save it as a disabled policy.
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
                onClick={() => setDraft({ example: {
                  appName: ex.appName, domainPattern: ex.domainPattern, priority: ex.priority,
                  bandwidthLimitMbps: ex.bandwidthLimitMbps, dataQuotaMb: ex.dataQuotaMb, customDns: ex.customDns, enabled: false,
                } })}
                style={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: '5px' }}
                title="Load the example into the policy editor"
              >
                <Plus size={12} /> Load example
              </button>
            </div>
          ))}
        </div>
      </div>

      {draft && <RuleEditor draft={draft} interfaces={interfaces} close={() => setDraft(null)} saved={() => {
        setDraft(null);
        flashNotice('Routing policy saved.');
        void loadData();
      }} />}
    </div>
  );
};
