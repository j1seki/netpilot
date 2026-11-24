import { useEffect, useState } from 'react';
import { api, InterfaceInfo, NetMergeStatus, NetMergeConfig } from '../api/wails';
import { AdapterQuota } from '../components/AdapterQuota';
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

export const NetMergeView = ({ active }: { active: boolean }) => {
  const [status, setStatus] = useState<NetMergeStatus | null>(null);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [configDraft, setConfigDraft] = useState<Partial<Pick<NetMergeConfig, 'ipv6Mode' | 'mtu' | 'autoWeight'>>>({});
  const [loading, setLoading] = useState<boolean>(false);
  const [changingMode, setChangingMode] = useState(false);
  const [limitInputs, setLimitInputs] = useState<Record<string, string>>({});
  const [socksDraft, setSocksDraft] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<string>('');
  const [errorMsg, setErrorMsg] = useState<string>('');
  const config = status ? { ...status.config, ...configDraft } : null;
  const socksAddr = status?.socksRunning
    ? status.socksAddress
    : socksDraft ?? status?.config.socksAddress ?? '127.0.0.1:1080';

  const loadStatus = async () => {
    try {
      const next = await api.getNetMergeStatus();
      setStatus(next);
      return next;
    } catch (err: any) {
      setErrorMsg(`Could not read Net-Merge status: ${err?.message || String(err)}`);
    }
  };

  useEffect(() => {
    if (!active) return;
    loadStatus();
    const interval = setInterval(loadStatus, 2000);
    return () => clearInterval(interval);
  }, [active]);

  // The OS default route is the honest default for Backup mode. It also lets the
  // UI explain why a VPN adapter should not be treated as a physical primary.
  useEffect(() => {
    if (!active) return;
    const loadIfaces = () => {
      api.getInterfaces().then(setInterfaces).catch(() => {});
    };
    loadIfaces();
    const interval = setInterval(loadIfaces, 10000);
    return () => clearInterval(interval);
  }, [active]);

  const showToast = (msg: string) => {
    setFeedback(msg);
    setTimeout(() => setFeedback(''), 3500);
  };

  const handleToggleSystemWide = async () => {
    const next = !(status?.isRunning ?? false);
    setLoading(true);
    setErrorMsg('');
    try {
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
        `Could not ${next ? 'start' : 'stop'} Net-Merge: ${detail}` +
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

  const handleSwitchRoutingMode = async (
    mode: 'speed' | 'backup',
    primary = config?.primaryAdapter || '',
    secondary = config?.secondaryAdapter || '',
  ) => {
    setChangingMode(true);
    setErrorMsg('');
    try {
      await api.setNetMergeRoutingMode(mode, primary, secondary);
      const next = await loadStatus();
      showToast(next?.modeChangePending
        ? 'Routing preference saved. Restart the engine from NetPilot to apply it.'
        : next?.isRunning || next?.socksRunning
          ? 'Routing updated for new connections.'
          : 'Routing preference saved for the next connection.');
    } catch (err) {
      setErrorMsg(`Could not switch routing mode. ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setChangingMode(false);
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
      setErrorMsg(`Could not ${next ? 'enable' : 'disable'} sign-in autostart: ${err?.message || String(err)}`);
    } finally {
      setLoading(false);
    }
  };

  const handleApplyLimit = async (adapter: string) => {
    const mbps = Number(limitInputs[adapter] ?? status?.limits?.[adapter] ?? 0);
    setErrorMsg('');
    try {
      if (!Number.isFinite(mbps) || mbps < 0 || mbps > 1_000_000) {
        throw new Error('Enter a speed cap between 0 and 1,000,000 Mbit/s.');
      }
      await api.setNetMergeLimit(adapter, mbps);
      await loadStatus();
      setLimitInputs(prev => {
        const next = { ...prev };
        delete next[adapter];
        return next;
      });
      showToast(mbps > 0 ? `Speed cap for ${adapter} set to ${mbps} Mbit/s.` : `Speed cap removed for ${adapter}.`);
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    }
  };

  const handleCleanup = async () => {
    setLoading(true);
    setErrorMsg('');
    try {
      await api.cleanupNetMerge();
      showToast('Stale TUN interfaces and leftover routes cleaned up.');
      await loadStatus();
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setLoading(false);
    }
  };

  const handleToggleSocks = async () => {
    if (!status) return;
    const action = status.socksRunning ? 'stop' : 'start';
    setLoading(true);
    setErrorMsg('');
    try {
      await api.controlNetMergeSocks(action, socksAddr);
      await loadStatus();
      setSocksDraft(null);
      showToast(action === 'start' ? 'SOCKS5 proxy started.' : 'SOCKS5 proxy stopped.');
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setLoading(false);
    }
  };

  const handleSaveConfig = async () => {
    if (!config) return;
    setErrorMsg('');
    try {
      if (!Number.isInteger(config.mtu) || config.mtu < 1280 || config.mtu > 65535) {
        throw new Error('Enter a virtual adapter MTU between 1280 and 65535.');
      }
      await api.saveNetMergeConfig(config);
      await loadStatus();
      setConfigDraft({});
      showToast('Net-Merge engine configuration saved.');
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    }
  };

  const isRunning = status?.isRunning || false;
  const routingMode = config?.routingMode || status?.routingMode || 'speed';
  const canUseBackup = !config?.engineSource || config.engineSource === 'builtin';
  const adapterNames = Array.from(new Set([
    ...physicalNames,
    ...(status?.nics || []).map(n => n.Name),
    config?.primaryAdapter || '',
    config?.secondaryAdapter || '',
  ].filter(Boolean)));

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
      {errorMsg && (
        <div
          style={{
            padding: '10px 14px',
            borderRadius: '8px',
            background: 'rgba(244, 63, 94, 0.14)',
            border: '1px solid rgba(244, 63, 94, 0.35)',
            color: 'var(--text-danger)',
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
            color: 'var(--text-success)',
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
          background: 'var(--bg-card)',
          border: `1px solid ${isRunning ? 'rgba(16, 185, 129, 0.35)' : 'var(--border-color)'}`,
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
              background: isRunning ? 'rgba(16, 185, 129, 0.15)' : 'var(--bg-tertiary)',
              border: `1px solid ${isRunning ? 'rgba(16, 185, 129, 0.4)' : 'var(--border-control)'}`,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            {isRunning ? (
              <CheckCircle2 size={22} color="var(--accent-primary)" />
            ) : (
              <PauseCircle size={22} color="var(--text-secondary)" />
            )}
          </div>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ fontSize: '15px', fontWeight: 700, color: 'var(--text-primary)' }}>
                Net-Merge Virtual Adapter
              </span>
              <span className={`status-pill ${isRunning ? 'pass' : 'blocked'}`}>
                {isRunning ? 'ACTIVE · net-merge' : 'STANDBY · System Default'}
              </span>
            </div>
            <div style={{ fontSize: '11.5px', color: 'var(--text-secondary)', marginTop: '3px' }}>
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
            title={status?.autostart ? 'Disable start at sign-in' : 'Enable start at sign-in'}
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

      {status && <div className="card" style={{ marginBottom: 0 }}>
        <strong>{isRunning ? `Running mode · ${status.routingMode === 'backup' ? 'Backup' : 'Speed'}` : 'System tunnel is in standby'}</strong>
        {status.modeChangePending ? <p className="mobile-warning" role="status">Restart the engine from NetPilot to use the saved mode or Backup adapters. If another tray owns the tunnel, quit that tray first.</p>
          : <p className="rule-editor-note">{status.restartForMode
            ? 'This separate engine requires a restart for mode and Backup adapter changes.'
            : 'Mode and Backup adapter changes apply to new NetPilot connections. Existing flows stay on their original link.'}</p>}
      </div>}

      {/* Mode Selection Card: Speed Mode vs Backup Mode */}
      <div className="grid-2" style={{ gap: '14px' }}>
        {/* Speed Mode Card */}
        <div
          className="card"
          style={{
            marginBottom: 0,
            border: `1px solid ${routingMode !== 'backup' ? 'var(--text-info)' : 'var(--border-color)'}`,
            background: routingMode !== 'backup' ? 'rgba(56, 189, 248, 0.05)' : 'var(--bg-card)',
            transition: 'all 0.14s ease',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Zap size={16} color="var(--text-info)" />
              <span style={{ fontSize: '13.5px', fontWeight: 700, color: 'var(--text-primary)' }}>
                Speed Mode (Multi-Link Aggregation)
              </span>
            </div>
            <button className="btn btn-secondary btn-sm" type="button" aria-pressed={routingMode !== 'backup'} disabled={!status || changingMode || routingMode !== 'backup'} onClick={() => handleSwitchRoutingMode('speed')}>
              {routingMode !== 'backup' ? 'Selected' : 'Use Speed'}
            </button>
          </div>
          <p style={{ fontSize: '11.5px', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
            Default behavior of <strong>net-merge</strong>: utilizes all healthy network adapters simultaneously using weighted least-busy-flows scheduling for maximum combined download and upload speed.
          </p>
          <p style={{ fontSize: '10.5px', color: 'var(--text-info)', marginTop: '8px', lineHeight: 1.5 }}>
            Speed Mode has no Primary or Secondary adapter — the merged <strong>net-merge</strong> adapter is the
            default route and every healthy link is used at once.
          </p>
        </div>

        {/* Backup Mode Card */}
        <div
          className="card"
          style={{
            marginBottom: 0,
            border: `1px solid ${routingMode === 'backup' ? 'var(--accent-warning)' : 'var(--border-color)'}`,
            background: routingMode === 'backup' ? 'rgba(245, 158, 11, 0.05)' : 'var(--bg-card)',
            transition: 'all 0.14s ease',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <ShieldAlert size={16} color="var(--accent-warning)" />
              <span style={{ fontSize: '13.5px', fontWeight: 700, color: 'var(--text-primary)' }}>
                Backup Mode (Primary → Failover Only)
              </span>
            </div>
            <button className="btn btn-secondary btn-sm" type="button" aria-pressed={routingMode === 'backup'} disabled={!status || changingMode || !canUseBackup || routingMode === 'backup'} onClick={() => handleSwitchRoutingMode('backup')}>
              {routingMode === 'backup' ? 'Selected' : canUseBackup ? 'Use Backup' : 'Requires NetPilot engine'}
            </button>
          </div>
          <p style={{ fontSize: '11.5px', color: 'var(--text-secondary)', lineHeight: 1.5, marginBottom: '10px' }}>
            Uses the <strong>Primary Adapter</strong> for new flows while it is healthy. New flows fail over to the <strong>Secondary Adapter</strong> when needed. App rules can select their own adapters.
          </p>

          {routingMode === 'backup' ? (
            <div
              onClick={e => e.stopPropagation()}
              style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px', paddingTop: '6px', borderTop: '1px solid var(--border-color)' }}
            >
              <div>
                <label style={{ fontSize: '10.5px', color: 'var(--text-secondary)', display: 'block', marginBottom: '3px' }}>
                  Primary Adapter {osDefaultName ? `(OS default: ${osDefaultName})` : ''}
                </label>
                <select
                  value={config?.primaryAdapter || ''}
                  disabled={changingMode}
                  onChange={e => handleSwitchRoutingMode('backup', e.target.value, config?.secondaryAdapter)}
                  style={{ width: '100%', padding: '4px 8px', fontSize: '11.5px' }}
                >
                  <option value="">Automatic</option>
                  {adapterNames.map(name => {
                    const info = interfaces.find(i => i.name === name);
                    return (
                      <option key={name} value={name} disabled={name === config?.secondaryAdapter}>
                        {name}{info?.isDefault ? ' · OS default' : ''}{info?.isVirtual ? ' · VPN/virtual' : ''}
                      </option>
                    );
                  })}
                </select>
              </div>
              <div>
                <label style={{ fontSize: '10.5px', color: 'var(--text-secondary)', display: 'block', marginBottom: '3px' }}>
                  Secondary / Backup Adapter
                </label>
                <select
                  value={config?.secondaryAdapter || ''}
                  disabled={changingMode}
                  onChange={e => handleSwitchRoutingMode('backup', config?.primaryAdapter || '', e.target.value)}
                  style={{ width: '100%', padding: '4px 8px', fontSize: '11.5px' }}
                >
                  <option value="">Automatic</option>
                  {adapterNames.map(name => {
                    const info = interfaces.find(i => i.name === name);
                    return (
                      <option key={name} value={name} disabled={name === config?.primaryAdapter}>
                        {name}{info?.isDefault ? ' · OS default' : ''}{info?.isVirtual ? ' · VPN/virtual' : ''}
                      </option>
                    );
                  })}
                </select>
              </div>
              <div style={{ gridColumn: '1 / -1', fontSize: '10px', color: 'var(--text-muted)' }}>
                Automatic uses an eligible healthy link. Manual choices apply in Backup mode.
              </div>
            </div>
          ) : (
            <div
              onClick={e => e.stopPropagation()}
              style={{ paddingTop: '8px', borderTop: '1px solid var(--border-color)', fontSize: '10.5px', color: 'var(--text-muted)' }}
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
            <Sliders size={14} color="var(--accent-primary)" />
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
                        <span style={{ color: 'var(--text-danger)', marginLeft: '6px' }}>
                          ({slot.Failed} err)
                        </span>
                      )}
                    </td>
                    <td className="font-mono" style={{ fontSize: '11.5px' }}>
                      <div style={{ color: 'var(--accent-primary)', fontWeight: 600 }}>
                        via net-merge: {viaMbps.toFixed(1)} Mbps
                      </div>
                      <div style={{ fontSize: '10.5px', color: 'var(--text-secondary)', display: 'flex', gap: '8px' }}>
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '2px' }}>
                          <ArrowDown size={10} color="var(--text-info)" /> {linkDownMbps.toFixed(1)}M
                        </span>
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '2px' }}>
                          <ArrowUp size={10} color="var(--accent-warning)" /> {linkUpMbps.toFixed(1)}M
                        </span>
                      </div>
                    </td>
                    <td>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <input
                          type="number"
                          placeholder="Unlimited"
                          value={limitInputs[slot.Name] ?? status?.limits?.[slot.Name] ?? ''}
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
          <div style={{ padding: '18px', color: 'var(--text-secondary)', fontSize: '12px', textAlign: 'center' }}>
            Discovering hardware network interfaces...
          </div>
        )}
      </div>

      <AdapterQuota status={status} />

      {/* Bottom 2-Column Grid: SOCKS5 Proxy & Engine Parameters */}
      <div className="grid-2">
        {/* Per-App SOCKS5 Proxy Balancer */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Server size={14} color="var(--text-info)" />
              <span>SOCKS5 Multi-Link Proxy (No Admin Required)</span>
            </div>
            <span className={`status-pill ${status?.socksRunning ? 'pass' : 'blocked'}`}>
              {status?.socksRunning ? 'LISTENING' : 'STOPPED'}
            </span>
          </div>

          <p style={{ fontSize: '11.5px', color: 'var(--text-secondary)', marginBottom: '12px' }}>
            Run a local SOCKS5 proxy that balances or fails over connections across adapters without modifying the system-wide routing table.
          </p>

          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <input
              type="text"
              value={socksAddr}
              onChange={e => setSocksDraft(e.target.value)}
              aria-label="SOCKS5 listen address"
              disabled={status?.socksRunning}
              placeholder="127.0.0.1:1080"
              style={{ flex: 1 }}
            />
            <button
              className={`btn ${status?.socksRunning ? 'btn-danger' : 'btn-primary'}`}
              onClick={handleToggleSocks}
              disabled={!status || loading}
            >
              {status?.socksRunning ? 'Stop SOCKS5' : 'Start SOCKS5'}
            </button>
          </div>
        </div>

        {/* Engine Parameters & Crash Recovery */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Sliders size={14} color="var(--accent-warning)" />
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
                  <label style={{ fontSize: '11px', color: 'var(--text-secondary)', display: 'block', marginBottom: '3px' }}>
                    IPv6 Balancing Policy
                  </label>
                  <select
                    value={config.ipv6Mode}
                    onChange={e => setConfigDraft({ ...configDraft, ipv6Mode: e.target.value })}
                    style={{ width: '100%' }}
                  >
                    <option value="auto">Auto (Only if all NICs support IPv6)</option>
                    <option value="on">Always On</option>
                    <option value="off">Disabled (IPv4 Only)</option>
                  </select>
                </div>

                <div>
                  <label style={{ fontSize: '11px', color: 'var(--text-secondary)', display: 'block', marginBottom: '3px' }}>
                    Virtual Adapter MTU
                  </label>
                  <input
                    type="number"
                    aria-label="Virtual Adapter MTU"
                    value={config.mtu || ''}
                    min={1280}
                    max={65535}
                    onChange={e => setConfigDraft({ ...configDraft, mtu: Number(e.target.value) })}
                    style={{ width: '100%' }}
                  />
                </div>
              </div>

              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: '4px' }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '11.5px', cursor: 'pointer', color: 'var(--text-strong)' }}>
                  <input
                    type="checkbox"
                    checked={config.autoWeight}
                    onChange={e => setConfigDraft({ ...configDraft, autoWeight: e.target.checked })}
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
