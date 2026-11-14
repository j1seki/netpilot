import React, { useEffect, useState } from 'react';
import {
  api,
  AdapterLabelRecord,
  DNSResult,
  InterfaceInfo,
  PingResult,
  PortResult,
} from '../api/wails';
import {
  Activity,
  AlertCircle,
  CheckCircle2,
  Globe,
  Power,
  Radio,
  Save,
  Search,
  Server,
  Trash2,
  Wifi,
} from 'lucide-react';

const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'NS', 'TXT', 'PTR', 'ALL'];
const PING_COUNTS = [1, 4, 8, 12, 20];

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export const Tools: React.FC = () => {
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [labels, setLabels] = useState<Record<string, string>>({});
  const [labelDraft, setLabelDraft] = useState<Record<string, string>>({});
  const [errorMsg, setErrorMsg] = useState('');
  const [successMsg, setSuccessMsg] = useState('');

  const [pingHost, setPingHost] = useState('1.1.1.1');
  const [pingCount, setPingCount] = useState(4);
  const [pingTimeout, setPingTimeout] = useState(1000);
  const [pingResult, setPingResult] = useState<PingResult | null>(null);
  const [pingBusy, setPingBusy] = useState(false);

  const [dnsHost, setDnsHost] = useState('example.com');
  const [dnsType, setDnsType] = useState('A');
  const [dnsResult, setDnsResult] = useState<DNSResult | null>(null);
  const [dnsBusy, setDnsBusy] = useState(false);

  const [portHost, setPortHost] = useState('1.1.1.1');
  const [port, setPort] = useState(443);
  const [portTimeout, setPortTimeout] = useState(3000);
  const [portResult, setPortResult] = useState<PortResult | null>(null);
  const [portBusy, setPortBusy] = useState(false);

  const [wolMac, setWolMac] = useState('');
  const [wolBroadcast, setWolBroadcast] = useState('255.255.255.255');
  const [wolPort, setWolPort] = useState(9);
  const [wolBusy, setWolBusy] = useState(false);

  const flash = (msg: string) => {
    setErrorMsg('');
    setSuccessMsg(msg);
    setTimeout(() => setSuccessMsg(''), 4000);
  };

  const load = async () => {
    const [ifacesRes, labelsRes] = await Promise.allSettled([
      api.getInterfaces(),
      api.getAdapterLabels(),
    ]);
    const problems: string[] = [];
    if (ifacesRes.status === 'fulfilled') setInterfaces(ifacesRes.value);
    else problems.push(`adapters: ${errorText(ifacesRes.reason)}`);

    if (labelsRes.status === 'fulfilled') {
      const map: Record<string, string> = {};
      for (const l of labelsRes.value as AdapterLabelRecord[]) map[l.interfaceName] = l.label;
      setLabels(map);
      setLabelDraft(map);
    } else {
      problems.push(`labels: ${errorText(labelsRes.reason)}`);
    }
    setErrorMsg(problems.length ? problems.join('; ') : '');
  };

  useEffect(() => {
    load().catch(e => console.error(e));
  }, []);

  const runPing = async () => {
    setPingBusy(true);
    setPingResult(null);
    try {
      setPingResult(await api.pingHost(pingHost.trim(), pingCount, pingTimeout));
    } catch (err) {
      setErrorMsg(`Ping failed: ${errorText(err)}`);
    } finally {
      setPingBusy(false);
    }
  };

  const runDns = async () => {
    setDnsBusy(true);
    setDnsResult(null);
    try {
      setDnsResult(await api.lookupDns(dnsHost.trim(), dnsType));
    } catch (err) {
      setErrorMsg(`DNS lookup failed: ${errorText(err)}`);
    } finally {
      setDnsBusy(false);
    }
  };

  const runPort = async () => {
    setPortBusy(true);
    setPortResult(null);
    try {
      setPortResult(await api.checkTcpPort(portHost.trim(), port, portTimeout));
    } catch (err) {
      setErrorMsg(`Port check failed: ${errorText(err)}`);
    } finally {
      setPortBusy(false);
    }
  };

  const runWol = async () => {
    setWolBusy(true);
    try {
      await api.wakeOnLan(wolMac.trim(), wolBroadcast.trim(), wolPort);
      flash(`Magic packet sent to ${wolMac.trim()}`);
    } catch (err) {
      setErrorMsg(`Wake-on-LAN failed: ${errorText(err)}`);
    } finally {
      setWolBusy(false);
    }
  };

  const saveLabel = async (name: string) => {
    try {
      const value = (labelDraft[name] || '').trim();
      await api.saveAdapterLabel(name, value);
      setLabels(prev => {
        const next = { ...prev };
        if (value) next[name] = value;
        else delete next[name];
        return next;
      });
      flash(value ? `Renamed ${name} to "${value}"` : `Cleared friendly name for ${name}`);
    } catch (err) {
      setErrorMsg(`Could not save adapter name: ${errorText(err)}`);
    }
  };

  const clearLabel = async (name: string) => {
    try {
      await api.deleteAdapterLabel(name);
      setLabels(prev => {
        const next = { ...prev };
        delete next[name];
        return next;
      });
      setLabelDraft(prev => ({ ...prev, [name]: '' }));
      flash(`Cleared friendly name for ${name}`);
    } catch (err) {
      setErrorMsg(`Could not clear adapter name: ${errorText(err)}`);
    }
  };

  const latencyColor = (ms: number) =>
    ms <= 0 ? '#71717a' : ms < 50 ? '#34d399' : ms < 150 ? '#fbbf24' : '#fb7185';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
      {errorMsg && (
        <div style={{
          padding: '9px 12px',
          borderRadius: '7px',
          background: 'rgba(244, 63, 94, 0.12)',
          border: '1px solid rgba(244, 63, 94, 0.3)',
          color: '#fb7185',
          fontSize: '11.5px',
          display: 'flex',
          alignItems: 'center',
          gap: '7px',
        }}>
          <AlertCircle size={14} />
          <span>{errorMsg}</span>
        </div>
      )}

      {successMsg && (
        <div style={{
          padding: '9px 12px',
          borderRadius: '7px',
          background: 'rgba(16, 185, 129, 0.12)',
          border: '1px solid rgba(16, 185, 129, 0.3)',
          color: '#34d399',
          fontSize: '11.5px',
          display: 'flex',
          alignItems: 'center',
          gap: '7px',
        }}>
          <CheckCircle2 size={14} />
          <span>{successMsg}</span>
        </div>
      )}

      <div className="grid-2" style={{ gap: '14px', alignItems: 'start' }}>
        {/* Ping */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '7px' }}>
              <Radio size={14} color="#10b981" />
              <span>Ping (ICMP Echo)</span>
            </div>
            {pingResult && (
              <span className={`status-pill ${pingResult.received > 0 && !pingResult.error ? 'pass' : 'fail'}`}>
                {pingResult.received}/{pingResult.sent} replies
              </span>
            )}
          </div>

          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', marginBottom: '10px' }}>
            <input
              type="text"
              value={pingHost}
              onChange={e => setPingHost(e.target.value)}
              placeholder="hostname or IP"
              style={{ flex: '1 1 150px' }}
              onKeyDown={e => { if (e.key === 'Enter') runPing(); }}
            />
            <select value={pingCount} onChange={e => setPingCount(Number(e.target.value))}>
              {PING_COUNTS.map(c => <option key={c} value={c}>{c} probes</option>)}
            </select>
            <select value={pingTimeout} onChange={e => setPingTimeout(Number(e.target.value))}>
              <option value={500}>0.5s timeout</option>
              <option value={1000}>1s timeout</option>
              <option value={2000}>2s timeout</option>
              <option value={5000}>5s timeout</option>
            </select>
            <button className="btn btn-primary btn-sm" onClick={runPing} disabled={pingBusy || !pingHost.trim()}>
              {pingBusy ? 'Pinging...' : 'Ping'}
            </button>
          </div>

          {pingResult && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {pingResult.error && (
                <div style={{ color: '#fb7185', fontSize: '11.5px' }}>{pingResult.error}</div>
              )}
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '8px' }}>
                {[
                  { label: 'Loss', value: `${pingResult.packetLoss.toFixed(0)}%`, color: pingResult.packetLoss > 0 ? '#fb7185' : '#34d399' },
                  { label: 'Min', value: `${pingResult.minMs.toFixed(1)} ms`, color: latencyColor(pingResult.minMs) },
                  { label: 'Avg', value: `${pingResult.avgMs.toFixed(1)} ms`, color: latencyColor(pingResult.avgMs) },
                  { label: 'Max', value: `${pingResult.maxMs.toFixed(1)} ms`, color: latencyColor(pingResult.maxMs) },
                ].map(item => (
                  <div key={item.label} style={{ backgroundColor: 'rgba(0,0,0,0.2)', padding: '6px 8px', borderRadius: '4px' }}>
                    <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>{item.label}</div>
                    <div className="font-mono" style={{ fontSize: '13px', fontWeight: 700, color: item.color }}>{item.value}</div>
                  </div>
                ))}
              </div>
              {pingResult.output && (
                <details>
                  <summary style={{ cursor: 'pointer', fontSize: '11px', color: 'var(--text-muted)' }}>Raw output</summary>
                  <pre className="font-mono" style={{
                    marginTop: '6px',
                    maxHeight: '150px',
                    overflow: 'auto',
                    fontSize: '10.5px',
                    whiteSpace: 'pre-wrap',
                    color: '#a1a1aa',
                    backgroundColor: '#09090b',
                    borderRadius: '5px',
                    padding: '8px',
                  }}>{pingResult.output}</pre>
                </details>
              )}
            </div>
          )}
        </div>

        {/* DNS */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '7px' }}>
              <Globe size={14} color="#38bdf8" />
              <span>DNS Lookup</span>
            </div>
            {dnsResult && !dnsResult.error && (
              <span className="status-pill pass">{dnsResult.records.length} records · {dnsResult.durationMs.toFixed(0)} ms</span>
            )}
          </div>

          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', marginBottom: '10px' }}>
            <input
              type="text"
              value={dnsHost}
              onChange={e => setDnsHost(e.target.value)}
              placeholder="hostname or IP"
              style={{ flex: '1 1 150px' }}
              onKeyDown={e => { if (e.key === 'Enter') runDns(); }}
            />
            <select value={dnsType} onChange={e => setDnsType(e.target.value)}>
              {RECORD_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
            </select>
            <button className="btn btn-primary btn-sm" onClick={runDns} disabled={dnsBusy || !dnsHost.trim()}>
              {dnsBusy ? 'Resolving...' : 'Resolve'}
            </button>
          </div>

          {dnsResult && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
              {dnsResult.error && <div style={{ color: '#fb7185', fontSize: '11.5px' }}>{dnsResult.error}</div>}
              {dnsResult.records.length === 0 && !dnsResult.error && (
                <div style={{ color: 'var(--text-muted)', fontSize: '11.5px' }}>No records returned.</div>
              )}
              {dnsResult.records.map((record, idx) => (
                <div key={`${record}-${idx}`} className="font-mono" style={{
                  fontSize: '11.5px',
                  color: '#d4d4d8',
                  backgroundColor: '#09090b',
                  border: '1px solid #232329',
                  borderRadius: '4px',
                  padding: '4px 8px',
                  wordBreak: 'break-all',
                }}>
                  {record}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="grid-2" style={{ gap: '14px', alignItems: 'start' }}>
        {/* TCP port */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '7px' }}>
              <Server size={14} color="#c084fc" />
              <span>TCP Port Check</span>
            </div>
            {portResult && (
              <span className={`status-pill ${portResult.open ? 'pass' : 'fail'}`}>
                {portResult.open ? 'OPEN' : 'CLOSED'}
              </span>
            )}
          </div>

          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', marginBottom: '10px' }}>
            <input
              type="text"
              value={portHost}
              onChange={e => setPortHost(e.target.value)}
              placeholder="hostname or IP"
              style={{ flex: '1 1 150px' }}
              onKeyDown={e => { if (e.key === 'Enter') runPort(); }}
            />
            <input
              type="number"
              min={1}
              max={65535}
              value={port}
              onChange={e => setPort(Number(e.target.value))}
              style={{ width: '90px' }}
              aria-label="port"
            />
            <select value={portTimeout} onChange={e => setPortTimeout(Number(e.target.value))}>
              <option value={1000}>1s timeout</option>
              <option value={3000}>3s timeout</option>
              <option value={10000}>10s timeout</option>
            </select>
            <button className="btn btn-primary btn-sm" onClick={runPort} disabled={portBusy || !portHost.trim()}>
              {portBusy ? 'Checking...' : 'Check'}
            </button>
          </div>

          {portResult && (
            <div style={{ fontSize: '11.5px', display: 'flex', flexDirection: 'column', gap: '4px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '6px', color: portResult.open ? '#34d399' : '#fb7185' }}>
                {portResult.open ? <CheckCircle2 size={13} /> : <AlertCircle size={13} />}
                <span>
                  {portHost}:{port} is {portResult.open ? 'open' : 'closed or filtered'}
                  {portResult.open ? ` · handshake ${portResult.latencyMs.toFixed(1)} ms` : ''}
                </span>
              </div>
              {portResult.error && <div className="font-mono" style={{ color: 'var(--text-muted)', fontSize: '10.5px', wordBreak: 'break-all' }}>{portResult.error}</div>}
            </div>
          )}
        </div>

        {/* Wake-on-LAN */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '7px' }}>
              <Power size={14} color="#f59e0b" />
              <span>Wake-on-LAN</span>
            </div>
            <span className="status-pill warn">MAGIC PACKET</span>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginBottom: '10px' }}>
            <div style={{ display: 'flex', gap: '8px' }}>
              <input
                type="text"
                value={wolMac}
                onChange={e => setWolMac(e.target.value)}
                placeholder="MAC address (00:11:22:33:44:55)"
                style={{ flex: 1 }}
              />
              <input
                type="number"
                min={1}
                max={65535}
                value={wolPort}
                onChange={e => setWolPort(Number(e.target.value))}
                style={{ width: '80px' }}
                aria-label="Wake-on-LAN port"
              />
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              <input
                type="text"
                value={wolBroadcast}
                onChange={e => setWolBroadcast(e.target.value)}
                placeholder="Broadcast address (255.255.255.255)"
                style={{ flex: 1 }}
              />
              <button className="btn btn-primary btn-sm" onClick={runWol} disabled={wolBusy || !wolMac.trim()}>
                {wolBusy ? 'Sending...' : 'Send'}
              </button>
            </div>
          </div>
          <p style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
            Wake-on-LAN must be enabled in the target machine's BIOS/UEFI and adapter settings. Use a subnet-directed
            broadcast address (for example 192.168.1.255) when the target is on another VLAN.
          </p>
        </div>
      </div>

      {/* Adapter friendly names */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '7px' }}>
            <Wifi size={14} color="#34d399" />
            <span>Adapter Friendly Names ({interfaces.length})</span>
          </div>
          <button className="btn btn-secondary btn-sm" onClick={() => load().catch(e => console.error(e))}>
            <Search size={12} />
            <span>Refresh</span>
          </button>
        </div>

        <p style={{ fontSize: '11.5px', color: 'var(--text-muted)', marginBottom: '10px' }}>
          Give adapters a recognizable name (for example "Home Fiber" or "5G Hotspot"). The system interface name stays
          unchanged for routing rules and the net-merge engine.
        </p>

        <table className="data-table">
          <thead>
            <tr>
              <th>Adapter</th>
              <th>System Name</th>
              <th>Friendly Name</th>
              <th style={{ textAlign: 'right' }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {interfaces.length === 0 ? (
              <tr>
                <td colSpan={4} style={{ textAlign: 'center', padding: '16px', color: 'var(--text-muted)' }}>
                  No adapters detected.
                </td>
              </tr>
            ) : (
              interfaces.map(iface => (
                <tr key={iface.name}>
                  <td>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <span className={`beszel-dot ${iface.isUp ? 'online' : 'down'}`} />
                      <div>
                        <div style={{ fontWeight: 600, color: '#fafafa' }}>{labels[iface.name] || iface.name}</div>
                        <div style={{ fontSize: '10.5px', color: '#71717a' }}>
                          {iface.ip || 'Unassigned'}{iface.isDefault ? ' · OS default' : ''}
                        </div>
                      </div>
                    </div>
                  </td>
                  <td className="font-mono" style={{ fontSize: '11px', color: '#a1a1aa' }}>{iface.name}</td>
                  <td>
                    <input
                      type="text"
                      value={labelDraft[iface.name] ?? ''}
                      onChange={e => setLabelDraft(prev => ({ ...prev, [iface.name]: e.target.value }))}
                      placeholder="Optional friendly name"
                      style={{ width: '100%', minWidth: '140px' }}
                      onKeyDown={e => { if (e.key === 'Enter') saveLabel(iface.name); }}
                    />
                  </td>
                  <td style={{ textAlign: 'right' }}>
                    <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
                      <button
                        className="btn btn-primary btn-sm"
                        onClick={() => saveLabel(iface.name)}
                        title="Save friendly name"
                      >
                        <Save size={12} />
                      </button>
                      <button
                        className="btn btn-secondary btn-sm"
                        onClick={() => clearLabel(iface.name)}
                        title="Clear friendly name"
                        disabled={!labels[iface.name]}
                      >
                        <Trash2 size={12} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '10.5px', color: 'var(--text-muted)' }}>
        <Activity size={12} />
        Diagnostics use the host's native network stack and do not change any routes or adapter settings.
      </div>
    </div>
  );
};
