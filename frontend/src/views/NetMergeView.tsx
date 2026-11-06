import React, { useEffect, useState } from 'react';
import { api, InterfaceInfo, NetMergeStatus, NetMergeConfig } from '../api/wails';
import {
  Power,
  RefreshCw,
  ShieldAlert,
  Sliders,
  Server,
  Zap,
  CheckCircle2,
  PauseCircle,
  ArrowDown,
  ArrowUp,
} from 'lucide-react';

export const NetMergeView: React.FC = () => {
  const [status, setStatus] = useState<NetMergeStatus | null>(null);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [config, setConfig] = useState<NetMergeConfig | null>(null);
  const [loading, setLoading] = useState<boolean>(false);
  const [limitInputs, setLimitInputs] = useState<Record<string, string>>({});
  const [socksAddr, setSocksAddr] = useState<string>('127.0.0.1:1080');
  const [feedback, setFeedback] = useState<string>('');
  const [errorMsg, setErrorMsg] = useState<string>('');

  const loadStatus = async () => {
    try {
      const st = await api.getNetMergeStatus();
      setStatus(st);
      if (!config) {
        setConfig(st.config);
        setSocksAddr(st.config.socksAddress || '127.0.0.1:1080');
      }
      const lims: Record<string, string> = {};
      if (st.limits) {
        Object.entries(st.limits).forEach(([k, v]) => {
          lims[k] = String(v);
        });
      }
      setLimitInputs(prev => ({ ...lims, ...prev }));
    } catch (err: any) {
      setErrorMsg(`Could not read Net-Merge status: ${err?.message || String(err)}`);
    }
  };

  useEffect(() => {
    loadStatus();
    const interval = setInterval(loadStatus, 2000);
    return () => clearInterval(interval);
  }, []);

  // The OS default route is the honest default for Backup mode. It also lets the
  // UI explain why a VPN adapter should not be treated as a physical primary.
  useEffect(() => {
    const loadIfaces = () => {
      api.getInterfaces().then(setInterfaces).catch(() => {});
    };
    loadIfaces();
    const interval = setInterval(loadIfaces, 10000);
    return () => clearInterval(interval);
  }, []);

  const showToast = (msg: string) => {
    setFeedback(msg);
    setTimeout(() => setFeedback(''), 3500);
  };

  const handleToggleSystemWide = async () => {
    setLoading(true);
    setErrorMsg('');
    try {
      const next = !(status?.isRunning ?? false);
      await api.toggleNetMerge(next);
      showToast(
        next
          ? 'Net-Merge adapter started.'
          : 'Net-Merge stopped. Default adapter returned to System Default (Standby).'
      );
      await loadStatus();
    } catch (err: any) {
      const detail = err?.message || String(err);
      setErrorMsg(
        `Could not start Net-Merge: ${detail}` +
        (detail.toLowerCase().includes('appimage')
          ? ' AppImages cannot keep CAP_NET_ADMIN, so NetPilot will ask Polkit (pkexec) for permission each time it starts the system-wide adapter.'
          : '')
      );
    } finally {
      setLoading(false);
    }
  };

  const osDefaultName = interfaces.find(i => i.isDefault)?.name || '';
  const physicalNames = interfaces.filter(i => i.isUp && !i.isVirtual).map(i => i.name);

  const handleSwitchRoutingMode = async (mode: 'speed' | 'backup', primary?: string, secondary?: string) => {
    const pri = primary !== undefined
      ? primary
      : (config?.primaryAdapter || osDefaultName || physicalNames[0] || status?.nics?.[0]?.Name || '');
    const sec = secondary !== undefined
      ? secondary
      : (config?.secondaryAdapter || physicalNames.find(n => n !== pri) || status?.nics?.find(n => n.Name !== pri)?.Name || '');
    setErrorMsg('');
    try {
      await api.setNetMergeRoutingMode(mode, pri, sec);
      if (config) {
        setConfig({
          ...config,
          routingMode: mode,
          primaryAdapter: pri,
          secondaryAdapter: sec,
        });
      }
      showToast(
        mode === 'backup'
          ? `Switched to Backup Failover mode (Primary: ${pri || 'OS default'} → Secondary: ${sec || 'auto'}).`
          : 'Switched to Speed Aggregation mode (all healthy adapters used simultaneously; no primary/secondary).'
      );
      await loadStatus();
    } catch (err: any) {
      setErrorMsg(`Could not switch routing mode: ${err?.message || String(err)}`);
    }
  };

  const handleToggleAutostart = async () => {
    // Do not silently return when status has not loaded yet: the endpoint works
    // independently and a silent no-op is exactly what made this button look dead.
    const next = !(status?.autostart ?? false);
    setLoading(true);
    setErrorMsg('');
    try {
      await api.setNetMergeAutostart(next);
      showToast(`Sign-in autostart ${next ? 'enabled' : 'disabled'}.`);
      await loadStatus();
    } catch (err: any) {
      setErrorMsg(
        `Could not ${next ? 'enable' : 'disable'} sign-in autostart: ${err?.message || String(err)}. ` +
        'On Windows this needs administrator rights; on macOS the passwordless sudo rule must be installed once.'
      );
    } finally {
      setLoading(false);
    }
  };

  const handleApplyLimit = async (adapter: string) => {
    const raw = limitInputs[adapter] || '0';
    const mbps = parseFloat(raw) || 0;
    await api.setNetMergeLimit(adapter, mbps);
    showToast(
      mbps > 0
        ? `Speed cap for ${adapter} set to ${mbps} Mbit/s.`
        : `Speed cap removed for ${adapter}.`
    );
    await loadStatus();
  };

  const handleCleanup = async () => {
    setLoading(true);
    try {
      await api.cleanupNetMerge();
      showToast('Stale TUN interfaces and leftover routes cleaned up.');
      await loadStatus();
    } finally {
      setLoading(false);
    }
  };

  const handleToggleSocks = async () => {
    if (!status) return;
    const action = status.socksRunning ? 'stop' : 'start';
    await api.controlNetMergeSocks(action, socksAddr);
    showToast(
      action === 'start'
        ? `SOCKS5 multi-link balancer started on ${socksAddr}.`
        : 'SOCKS5 proxy stopped.'
    );
    await loadStatus();
  };

  const handleSaveConfig = async () => {
    if (!config) return;
    await api.saveNetMergeConfig(config);
    showToast('Net-Merge engine configuration saved.');
    await loadStatus();
  };

  const isRunning = status?.isRunning || false;
  const routingMode = status?.routingMode || config?.routingMode || 'speed';
  const adapterNames = interfaces.length > 0
    ? interfaces.filter(i => i.isUp).map(i => i.name)
    : (status?.nics || []).map(n => n.Name);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
      {errorMsg && (
        <div
          style={{
            padding: '10px 14px',
            borderRadius: '8px',
            background: 'rgba(244, 63, 94, 0.14)',
            border: '1px solid rgba(244, 63, 94, 0.35)',
            color: '#fb7185',
            fontSize: '12px',
            fontWeight: 600,
          }}
        >
          {errorMsg}
        </div>
      )}
      {feedback && (
        <div
          style={{
            padding: '10px 14px',
            borderRadius: '8px',
            background: 'rgba(16, 185, 129, 0.14)',
            border: '1px solid rgba(16, 185, 129, 0.35)',
            color: '#34d399',
            fontSize: '12px',
            fontWeight: 600,
          }}
        >
          {feedback}
        </div>
      )}

      {/* Top Master Banner */}
      <div
        className="card"
        style={{
          marginBottom: 0,
          background: '#121215',
          border: `1px solid ${isRunning ? 'rgba(16, 185, 129, 0.35)' : '#232329'}`,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '16px 20px',
          flexWrap: 'wrap',
          gap: '14px',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
          <div
            style={{
              width: '44px',
              height: '44px',
              borderRadius: '9px',
              background: isRunning ? 'rgba(16, 185, 129, 0.15)' : '#18181b',
              border: `1px solid ${isRunning ? 'rgba(16, 185, 129, 0.4)' : '#27272a'}`,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            {isRunning ? (
              <CheckCircle2 size={22} color="#10b981" />
            ) : (
              <PauseCircle size={22} color="#a1a1aa" />
            )}
          </div>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ fontSize: '15px', fontWeight: 700, color: '#fafafa' }}>
                Net-Merge Virtual Adapter
              </span>
              <span className={`status-pill ${isRunning ? 'pass' : 'blocked'}`}>
                {isRunning ? 'ACTIVE · net-merge' : 'STANDBY · System Default'}
              </span>
              {status?.binary?.version && (
                <span className="font-mono" style={{ fontSize: '10.5px', color: '#71717a' }}>
                  {status.binary.version}
                </span>
              )}
            </div>
            <div style={{ fontSize: '11.5px', color: '#a1a1aa', marginTop: '3px' }}>
              {isRunning
                ? `Mode: ${status?.mode || 'system-wide'} · Default Adapter: ${status?.defaultAdapter || 'net-merge'} · IPv6: ${status?.v6State || 'auto'} · Uptime: ${status?.uptimeSec || 0}s`
                : 'Inactive: Default adapter is System Default. NetPilot does nothing to system routes while Net-Merge is off.'}
            </div>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <button
            className="btn btn-secondary"
            onClick={handleToggleAutostart}
            disabled={loading}
            title={status?.autostart ? 'Disable starting net-merge at sign-in' : 'Enable starting net-merge at sign-in'}
          >
            <span>Autostart: {status?.autostart ? 'ON' : 'OFF'}</span>
          </button>

          <button
            className={`btn ${isRunning ? 'btn-danger' : 'btn-emerald'}`}
            onClick={handleToggleSystemWide}
            disabled={loading}
          >
            <Power size={14} />
            <span>{loading ? (isRunning ? 'Stopping...' : 'Starting...') : (isRunning ? 'Stop Net-Merge' : 'Start Net-Merge')}</span>
          </button>
        </div>
      </div>

      {/* Mode Selection Card: Speed Mode vs Backup Mode */}
      <div className="grid-2" style={{ gap: '14px' }}>
        {/* Speed Mode Card */}
        <div
          className="card"
          onClick={() => handleSwitchRoutingMode('speed')}
          style={{
            marginBottom: 0,
            cursor: 'pointer',
            border: `1px solid ${routingMode !== 'backup' ? '#38bdf8' : '#232329'}`,
            background: routingMode !== 'backup' ? 'rgba(56, 189, 248, 0.05)' : '#121215',
            transition: 'all 0.14s ease',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Zap size={16} color="#38bdf8" />
              <span style={{ fontSize: '13.5px', fontWeight: 700, color: '#fafafa' }}>
                Speed Mode (Multi-Link Aggregation)
              </span>
            </div>
            <span className={`status-pill ${routingMode !== 'backup' ? 'pass' : 'blocked'}`}>
              {routingMode !== 'backup' ? 'SELECTED (DEFAULT)' : 'SELECT'}
            </span>
          </div>
          <p style={{ fontSize: '11.5px', color: '#a1a1aa', lineHeight: 1.5 }}>
            Default behavior of <strong>net-merge</strong>: utilizes all healthy network adapters simultaneously using weighted least-busy-flows scheduling for maximum combined download and upload speed.
          </p>
          <p style={{ fontSize: '10.5px', color: '#38bdf8', marginTop: '8px', lineHeight: 1.5 }}>
            Speed Mode has no Primary or Secondary adapter — the merged <strong>net-merge</strong> adapter is the
            default route and every healthy link is used at once.
          </p>
        </div>

        {/* Backup Mode Card */}
        <div
          className="card"
          onClick={() => handleSwitchRoutingMode('backup')}
          style={{
            marginBottom: 0,
            cursor: 'pointer',
            border: `1px solid ${routingMode === 'backup' ? '#f59e0b' : '#232329'}`,
            background: routingMode === 'backup' ? 'rgba(245, 158, 11, 0.05)' : '#121215',
            transition: 'all 0.14s ease',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <ShieldAlert size={16} color="#f59e0b" />
              <span style={{ fontSize: '13.5px', fontWeight: 700, color: '#fafafa' }}>
                Backup Mode (Primary → Failover Only)
              </span>
            </div>
            <span className={`status-pill ${routingMode === 'backup' ? 'warn' : 'blocked'}`}>
              {routingMode === 'backup' ? 'SELECTED' : 'SELECT'}
            </span>
          </div>
          <p style={{ fontSize: '11.5px', color: '#a1a1aa', lineHeight: 1.5, marginBottom: '10px' }}>
            Routes 100% of traffic through the <strong>Primary Adapter</strong> while it is online. Automatically switches to the <strong>Secondary Adapter</strong> only when the primary connection fails, and switches back upon recovery.
          </p>

          {routingMode === 'backup' ? (
            <div
              onClick={e => e.stopPropagation()}
              style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px', paddingTop: '6px', borderTop: '1px solid #232329' }}
            >
              <div>
                <label style={{ fontSize: '10.5px', color: '#a1a1aa', display: 'block', marginBottom: '3px' }}>
                  Primary Adapter {osDefaultName ? `(OS default: ${osDefaultName})` : ''}
                </label>
                <select
                  value={config?.primaryAdapter || osDefaultName || adapterNames[0] || ''}
                  onChange={e => handleSwitchRoutingMode('backup', e.target.value, config?.secondaryAdapter)}
                  style={{ width: '100%', padding: '4px 8px', fontSize: '11.5px' }}
                >
                  {adapterNames.length === 0 && <option value="">Auto-detect Primary</option>}
                  {adapterNames.map(name => {
                    const info = interfaces.find(i => i.name === name);
                    return (
                      <option key={name} value={name}>
                        {name}{info?.isDefault ? ' · OS default' : ''}{info?.isVirtual ? ' · VPN/virtual' : ''}
                      </option>
                    );
                  })}
                </select>
              </div>
              <div>
                <label style={{ fontSize: '10.5px', color: '#a1a1aa', display: 'block', marginBottom: '3px' }}>
                  Secondary / Backup Adapter
                </label>
                <select
                  value={config?.secondaryAdapter || physicalNames.find(n => n !== (config?.primaryAdapter || osDefaultName)) || adapterNames[1] || ''}
                  onChange={e => handleSwitchRoutingMode('backup', config?.primaryAdapter || osDefaultName, e.target.value)}
                  style={{ width: '100%', padding: '4px 8px', fontSize: '11.5px' }}
                >
                  {adapterNames.length === 0 && <option value="">Auto-detect Secondary</option>}
                  {adapterNames.map(name => {
                    const info = interfaces.find(i => i.name === name);
                    return (
                      <option key={name} value={name}>
                        {name}{info?.isDefault ? ' · OS default' : ''}{info?.isVirtual ? ' · VPN/virtual' : ''}
                      </option>
                    );
                  })}
                </select>
              </div>
              <div style={{ gridColumn: '1 / -1', fontSize: '10px', color: '#71717a' }}>
                Defaults follow the OS default route. Manual choices are only used in Backup mode; Speed mode ignores them.
              </div>
            </div>
          ) : (
            <div
              onClick={e => e.stopPropagation()}
              style={{ paddingTop: '8px', borderTop: '1px solid #232329', fontSize: '10.5px', color: '#71717a' }}
            >
              Primary/Secondary selectors appear when Backup mode is selected. In Speed mode the merged adapter is the primary link.
            </div>
          )}
        </div>
      </div>

      {/* Live Adapter Slots & Bandwidth Limiter Table */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Sliders size={14} color="#10b981" />
            <span>Net-Merge Scheduler Slots & Per-Adapter Speed Caps</span>
          </div>
          <button className="btn btn-secondary btn-sm" onClick={loadStatus}>
            <RefreshCw size={12} />
            <span>Refresh</span>
          </button>
        </div>

        {status?.nics && status.nics.length > 0 ? (
          <table className="data-table">
            <thead>
              <tr>
                <th>Adapter</th>
                <th>Probe State (v4 / v6)</th>
                <th>Weight (v4 / v6)</th>
                <th>Flows (Active / Total)</th>
                <th>Relayed / Link Speed</th>
                <th>Speed Cap (Mbit/s)</th>
              </tr>
            </thead>
            <tbody>
              {status.nics.map(slot => {
                const viaMbps = (((slot.RateIn || 0) + (slot.RateOut || 0)) * 8) / 1e6;
                const linkDownMbps = ((slot.LinkIn || 0) * 8) / 1e6;
                const linkUpMbps = ((slot.LinkOut || 0) * 8) / 1e6;
                const isUp = slot.State === 'up' || slot.State === 'checking';
                return (
                  <tr key={slot.Name}>
                    <td>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', fontWeight: 600 }}>
                        <span className={`beszel-dot ${isUp ? 'online' : 'down'}`} />
                        <span>{slot.Name}</span>
                      </div>
                    </td>
                    <td>
                      <span className={`status-pill ${slot.State === 'up' ? 'pass' : slot.State === 'checking' ? 'warn' : 'fail'}`}>
                        v4: {slot.State || 'checking'}
                      </span>{' '}
                      <span className="status-pill blocked">
                        v6: {slot.State6 || '-'}
                      </span>
                    </td>
                    <td className="font-mono" style={{ fontSize: '11.5px' }}>
                      {(slot.Weight || 1).toFixed(2)} / {(slot.Weight6 || 1).toFixed(2)}
                    </td>
                    <td className="font-mono" style={{ fontSize: '11.5px' }}>
                      {slot.Active || 0} / {slot.Total || 0}
                      {slot.Failed > 0 && (
                        <span style={{ color: '#fb7185', marginLeft: '6px' }}>
                          ({slot.Failed} err)
                        </span>
                      )}
                    </td>
                    <td className="font-mono" style={{ fontSize: '11.5px' }}>
                      <div style={{ color: '#10b981', fontWeight: 600 }}>
                        via net-merge: {viaMbps.toFixed(1)} Mbps
                      </div>
                      <div style={{ fontSize: '10.5px', color: '#a1a1aa', display: 'flex', gap: '8px' }}>
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '2px' }}>
                          <ArrowDown size={10} color="#38bdf8" /> {linkDownMbps.toFixed(1)}M
                        </span>
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '2px' }}>
                          <ArrowUp size={10} color="#f59e0b" /> {linkUpMbps.toFixed(1)}M
                        </span>
                      </div>
                    </td>
                    <td>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <input
                          type="number"
                          placeholder="Unlimited"
                          value={limitInputs[slot.Name] ?? (slot.LimitMbps > 0 ? String(slot.LimitMbps) : '')}
                          onChange={e =>
                            setLimitInputs(prev => ({ ...prev, [slot.Name]: e.target.value }))
                          }
                          style={{ width: '90px', padding: '4px 7px', fontSize: '11.5px' }}
                        />
                        <button
                          className="btn btn-secondary btn-sm"
                          onClick={() => handleApplyLimit(slot.Name)}
                        >
                          Apply
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        ) : (
          <div style={{ padding: '18px', color: '#a1a1aa', fontSize: '12px', textAlign: 'center' }}>
            Discovering hardware network interfaces...
          </div>
        )}
      </div>

      {/* Bottom 2-Column Grid: SOCKS5 Proxy & Engine Parameters */}
      <div className="grid-2">
        {/* Per-App SOCKS5 Proxy Balancer */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Server size={14} color="#38bdf8" />
              <span>SOCKS5 Multi-Link Proxy (No Admin Required)</span>
            </div>
            <span className={`status-pill ${status?.socksRunning ? 'pass' : 'blocked'}`}>
              {status?.socksRunning ? 'LISTENING' : 'STOPPED'}
            </span>
          </div>

          <p style={{ fontSize: '11.5px', color: '#a1a1aa', marginBottom: '12px' }}>
            Run a local SOCKS5 proxy that balances or fails over connections across adapters without modifying the system-wide routing table.
          </p>

          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <input
              type="text"
              value={socksAddr}
              onChange={e => setSocksAddr(e.target.value)}
              placeholder="127.0.0.1:1080"
              style={{ flex: 1 }}
            />
            <button
              className={`btn ${status?.socksRunning ? 'btn-danger' : 'btn-primary'}`}
              onClick={handleToggleSocks}
            >
              {status?.socksRunning ? 'Stop SOCKS5' : 'Start SOCKS5'}
            </button>
          </div>
        </div>

        {/* Engine Parameters & Crash Recovery */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Sliders size={14} color="#f59e0b" />
              <span>TUN & Scheduler Parameters</span>
            </div>
            <button className="btn btn-danger btn-sm" onClick={handleCleanup} disabled={loading}>
              <ShieldAlert size={12} />
              <span>Emergency Route Cleanup</span>
            </button>
          </div>

          {config && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              <div className="grid-2" style={{ gap: '8px' }}>
                <div>
                  <label style={{ fontSize: '11px', color: '#a1a1aa', display: 'block', marginBottom: '3px' }}>
                    IPv6 Balancing Policy
                  </label>
                  <select
                    value={config.ipv6Mode}
                    onChange={e => setConfig({ ...config, ipv6Mode: e.target.value })}
                    style={{ width: '100%' }}
                  >
                    <option value="auto">Auto (Only if all NICs support IPv6)</option>
                    <option value="on">Always On</option>
                    <option value="off">Disabled (IPv4 Only)</option>
                  </select>
                </div>

                <div>
                  <label style={{ fontSize: '11px', color: '#a1a1aa', display: 'block', marginBottom: '3px' }}>
                    Virtual Adapter MTU
                  </label>
                  <input
                    type="number"
                    value={config.mtu}
                    min={1280}
                    max={65535}
                    onChange={e => setConfig({ ...config, mtu: parseInt(e.target.value, 10) || 1500 })}
                    style={{ width: '100%' }}
                  />
                </div>
              </div>

              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: '4px' }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '11.5px', cursor: 'pointer', color: '#d4d4d8' }}>
                  <input
                    type="checkbox"
                    checked={config.autoWeight}
                    onChange={e => setConfig({ ...config, autoWeight: e.target.checked })}
                  />
                  <span>Auto-Weight by Observed Throughput</span>
                </label>

                <button className="btn btn-primary btn-sm" onClick={handleSaveConfig}>
                  Save Parameters
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
