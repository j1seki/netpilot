import React, { useEffect, useState } from 'react';
import { api, NetMergeStatus, NetMergeConfig } from '../api/wails';
import {
  GitMerge,
  Power,
  RefreshCw,
  Zap,
  Sliders,
  Radio,
  Trash2,
} from 'lucide-react';

export const NetMergeView: React.FC = () => {
  const [status, setStatus] = useState<NetMergeStatus | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [toggling, setToggling] = useState<boolean>(false);
  const [cleaning, setCleaning] = useState<boolean>(false);
  const [socksLoading, setSocksLoading] = useState<boolean>(false);
  const [config, setConfig] = useState<NetMergeConfig>({
    autoPause: true,
    autoWeight: true,
    ipv6Mode: 'auto',
    mtu: 1500,
    includeNics: '',
    rescanSec: 3,
    socksAddress: '127.0.0.1:1080',
    weights: {},
    showConsole: false,
    autostart: false,
  });

  const loadStatus = async () => {
    try {
      const st = await api.getNetMergeStatus();
      setStatus(st);
      if (st.config) {
        setConfig(st.config);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadStatus();
    const interval = setInterval(loadStatus, 2000);
    return () => clearInterval(interval);
  }, []);

  const handleToggleSystemWide = async () => {
    if (!status) return;
    setToggling(true);
    try {
      await api.toggleNetMerge(!status.isRunning);
      await loadStatus();
    } finally {
      setToggling(false);
    }
  };

  const handleToggleAutostart = async (val: boolean) => {
    await api.setNetMergeAutostart(val);
    await loadStatus();
  };

  const handleSaveConfig = async () => {
    await api.saveNetMergeConfig(config);
    await loadStatus();
  };

  const handleCleanup = async () => {
    setCleaning(true);
    try {
      await api.cleanupNetMerge();
      await loadStatus();
    } finally {
      setCleaning(false);
    }
  };

  const handleToggleSocks = async () => {
    if (!status) return;
    setSocksLoading(true);
    try {
      if (status.socksRunning) {
        await api.controlNetMergeSocks('stop');
      } else {
        await api.controlNetMergeSocks('start', config.socksAddress);
      }
      await loadStatus();
    } finally {
      setSocksLoading(false);
    }
  };

  const handleSetLimit = async (adapter: string, mbpsStr: string) => {
    const mbps = parseFloat(mbpsStr) || 0;
    await api.setNetMergeLimit(adapter, mbps);
    await loadStatus();
  };

  if (loading && !status) {
    return (
      <div style={{ padding: '20px', textAlign: 'center', color: 'var(--text-secondary)' }}>
        Initializing Net-Merge engine controller...
      </div>
    );
  }

  const isRunning = status?.isRunning ?? false;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      {/* Engine Status & Master Switch Header */}
      <div className="card" style={{
        marginBottom: 0,
        background: isRunning ? 'linear-gradient(135deg, rgba(16, 185, 129, 0.08), rgba(6, 182, 212, 0.08))' : 'var(--bg-card)',
        borderColor: isRunning ? 'rgba(16, 185, 129, 0.35)' : 'var(--border)',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '10px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div style={{
              width: '36px',
              height: '36px',
              borderRadius: '8px',
              background: isRunning ? 'rgba(16, 185, 129, 0.2)' : 'rgba(255, 255, 255, 0.05)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: `1px solid ${isRunning ? '#10b981' : 'var(--border)'}`,
            }}>
              <GitMerge size={20} color={isRunning ? '#10b981' : '#94a3b8'} />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h2 style={{ fontSize: '15px', fontWeight: 700, color: '#f8fafc' }}>
                  Net-Merge Multi-WAN Aggregation Engine
                </h2>
                <span className={`status-pill ${isRunning ? 'pass' : 'blocked'}`} style={{ fontSize: '10.5px' }}>
                  {isRunning ? '● ACTIVE (MERGING)' : '○ PAUSED / OFF'}
                </span>
                {status?.pid ? (
                  <span className="font-mono" style={{ fontSize: '10.5px', color: 'var(--text-muted)' }}>
                    PID: {status.pid} • Uptime: {status.uptimeSec}s
                  </span>
                ) : null}
              </div>
              <p style={{ fontSize: '11.5px', color: 'var(--text-secondary)', marginTop: '2px' }}>
                Binds to all physical and virtual interfaces simultaneously, aggregating bandwidth and performing packet-level round-robin balancing.
              </p>
              {status?.binary && (
                <div style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '6px',
                  fontSize: '11px',
                  marginTop: '4px',
                  color: status.binary.isValid ? '#4ade80' : '#f87171',
                }}>
                  <span style={{ color: 'var(--text-muted)' }}>Engine binary:</span>
                  <span className="font-mono" style={{ color: status.binary.isValid ? '#38bdf8' : '#f87171' }}>
                    {status.binary.isValid ? `${status.binary.version} [${status.binary.source}]` : 'Not found (Place next to app or set path in Settings)'}
                  </span>
                </div>
              )}
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <button
              className={`btn btn-sm ${isRunning ? 'btn-danger' : 'btn-primary'}`}
              onClick={handleToggleSystemWide}
              disabled={toggling}
              style={{ display: 'flex', alignItems: 'center', gap: '6px', fontWeight: 600, padding: '7px 14px' }}
            >
              <Power size={13} />
              {toggling ? 'Applying State...' : isRunning ? 'Turn System-Wide OFF' : 'Turn System-Wide ON'}
            </button>
            <button
              className="btn btn-secondary btn-sm"
              onClick={loadStatus}
              title="Refresh Engine Status"
              style={{ padding: '7px 10px' }}
            >
              <RefreshCw size={13} />
            </button>
          </div>
        </div>
      </div>

      {/* Grid: Live Slot Statistics & SOCKS5 Proxy Mode */}
      <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: '10px' }}>
        {/* Active Balancing Slots */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Zap size={14} color="#38bdf8" /> Balanced Interfaces ({status?.nics.length || 0})
            </span>
            <span className="font-mono" style={{ fontSize: '10.5px', color: 'var(--text-muted)' }}>
              IPv6: {status?.v6State || 'auto'}
            </span>
          </div>

          {(!status?.nics || status.nics.length === 0) ? (
            <div style={{ padding: '16px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '11.5px' }}>
              No active Net-Merge balancing slots registered yet. Turn System-Wide ON or configure adapter inclusion list.
            </div>
          ) : (
            <table className="data-table">
              <thead>
                <tr>
                  <th>Adapter</th>
                  <th>State</th>
                  <th>Weight</th>
                  <th>Flows (Act/Tot)</th>
                  <th>Link Speed (Rx/Tx)</th>
                  <th>Speed Cap</th>
                </tr>
              </thead>
              <tbody>
                {status.nics.map(nic => {
                  const limit = status.limits[nic.Name] || 0;
                  return (
                    <tr key={nic.Name}>
                      <td style={{ fontWeight: 600 }}>{nic.Name}</td>
                      <td>
                        <span className={`status-pill ${nic.State === 'up' ? 'pass' : 'fail'}`} style={{ fontSize: '10px' }}>
                          v4: {nic.State}
                        </span>
                      </td>
                      <td className="font-mono" style={{ fontSize: '11px' }}>
                        {nic.Weight.toFixed(2)}
                      </td>
                      <td className="font-mono" style={{ fontSize: '11px' }}>
                        <span style={{ color: '#38bdf8' }}>{nic.Active}</span> / {nic.Total}
                      </td>
                      <td className="font-mono" style={{ fontSize: '11px' }}>
                        <span style={{ color: '#22d3ee' }}>↓ {(nic.LinkIn * 8 / 1e6).toFixed(1)}</span>{' '}
                        <span style={{ color: '#facc15' }}>↑ {(nic.LinkOut * 8 / 1e6).toFixed(1)}</span>
                      </td>
                      <td>
                        <input
                          type="number"
                          placeholder="No cap"
                          defaultValue={limit > 0 ? limit : ''}
                          onBlur={e => handleSetLimit(nic.Name, e.target.value)}
                          className="input"
                          style={{ width: '75px', padding: '2px 6px', fontSize: '11px' }}
                        />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>

        {/* SOCKS5 Proxy Mode */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Radio size={14} color="#facc15" /> SOCKS5 Per-App Proxy Balancer
            </span>
            <span className={`status-pill ${status?.socksRunning ? 'pass' : 'blocked'}`} style={{ fontSize: '10px' }}>
              {status?.socksRunning ? 'LISTENING' : 'OFF'}
            </span>
          </div>
          <p style={{ fontSize: '11px', color: 'var(--text-secondary)', marginBottom: '10px' }}>
            Route individual browsers, torrent clients, or games across multi-WAN without modifying default system routes or requiring admin privileges.
          </p>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
            <div>
              <label style={{ fontSize: '10.5px', color: 'var(--text-muted)', display: 'block', marginBottom: '2px' }}>
                Proxy Listen Address
              </label>
              <input
                type="text"
                value={config.socksAddress}
                onChange={e => setConfig(prev => ({ ...prev, socksAddress: e.target.value }))}
                className="input"
                style={{ width: '100%', fontSize: '11.5px', padding: '5px 8px' }}
                disabled={status?.socksRunning}
              />
            </div>

            <button
              className={`btn btn-sm ${status?.socksRunning ? 'btn-danger' : 'btn-primary'}`}
              onClick={handleToggleSocks}
              disabled={socksLoading}
              style={{ marginTop: '4px', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px', fontWeight: 600 }}
            >
              <Zap size={12} />
              {socksLoading ? 'Processing...' : status?.socksRunning ? 'Stop SOCKS5 Proxy' : 'Start SOCKS5 Proxy'}
            </button>
          </div>
        </div>
      </div>

      {/* Grid: Engine Settings & Crash Cleanup */}
      <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: '10px' }}>
        {/* Core Net-Merge Settings */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Sliders size={14} color="#a78bfa" /> Aggregation Parameters
            </span>
            <button className="btn btn-primary btn-sm" onClick={handleSaveConfig} style={{ fontSize: '11px', padding: '3px 8px' }}>
              Save Settings
            </button>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px', fontSize: '11.5px' }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={config.autostart}
                onChange={e => handleToggleAutostart(e.target.checked)}
                style={{ accentColor: '#10b981' }}
              />
              <span>Autostart at Windows Sign-in</span>
            </label>

            <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={config.autoPause}
                onChange={e => setConfig(prev => ({ ...prev, autoPause: e.target.checked }))}
                style={{ accentColor: '#10b981' }}
              />
              <span>Auto-Pause (if &lt; 2 uplinks work)</span>
            </label>

            <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={config.autoWeight}
                onChange={e => setConfig(prev => ({ ...prev, autoWeight: e.target.checked }))}
                style={{ accentColor: '#10b981' }}
              />
              <span>Auto-Weight by Measured Speed</span>
            </label>

            <div>
              <span style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>IPv6 Mode:</span>
              <select
                value={config.ipv6Mode}
                onChange={e => setConfig(prev => ({ ...prev, ipv6Mode: e.target.value }))}
                className="input"
                style={{ width: '100%', marginTop: '2px', padding: '3px 6px', fontSize: '11px' }}
              >
                <option value="auto">Auto (v6 only if all links have it)</option>
                <option value="on">Always On</option>
                <option value="off">Off (Force IPv4)</option>
              </select>
            </div>

            <div>
              <span style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>TUN Device MTU (1280-65535):</span>
              <input
                type="number"
                value={config.mtu}
                onChange={e => setConfig(prev => ({ ...prev, mtu: parseInt(e.target.value) || 1500 }))}
                className="input"
                style={{ width: '100%', marginTop: '2px', padding: '3px 6px', fontSize: '11px' }}
              />
            </div>

            <div>
              <span style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>Include Specific Adapters:</span>
              <input
                type="text"
                placeholder="Leave blank for all"
                value={config.includeNics}
                onChange={e => setConfig(prev => ({ ...prev, includeNics: e.target.value }))}
                className="input"
                style={{ width: '100%', marginTop: '2px', padding: '3px 6px', fontSize: '11px' }}
              />
            </div>
          </div>
        </div>

        {/* Network Cleanup & Emergency Recovery */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Trash2 size={14} color="#f87171" /> Emergency Recovery & Cleanup
            </span>
          </div>
          <p style={{ fontSize: '11.5px', color: 'var(--text-secondary)', marginBottom: '12px' }}>
            If NetPilot or Windows crashed or closed abruptly, click below to safely tear down leftover TUN adapters, restore default gateway metrics, and flush orphaned routes.
          </p>

          <button
            className="btn btn-secondary btn-sm"
            onClick={handleCleanup}
            disabled={cleaning}
            style={{ display: 'flex', alignItems: 'center', gap: '6px', color: '#f87171', borderColor: 'rgba(239, 68, 68, 0.4)', fontWeight: 600 }}
          >
            <RefreshCw size={12} />
            {cleaning ? 'Cleaning Up Routes...' : 'Run Emergency Network Cleanup'}
          </button>
        </div>
      </div>
    </div>
  );
};
