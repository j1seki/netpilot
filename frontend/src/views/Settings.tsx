import React, { useEffect, useState } from 'react';
import { api, BrandingInfo, CapabilityReport } from '../api/wails';
import { Settings as SettingsIcon, ShieldAlert, Terminal } from 'lucide-react';

export const Settings: React.FC = () => {
  const [branding, setBranding] = useState<BrandingInfo | null>(null);
  const [targetMode, setTargetMode] = useState<'LIVE' | 'DRY_RUN'>('LIVE');
  const [modeMsg, setModeMsg] = useState('');
  const [showConsole, setShowConsole] = useState<boolean>(false);
  const [consoleMsg, setConsoleMsg] = useState('');
  const [caps, setCaps] = useState<CapabilityReport | null>(null);

  useEffect(() => {
    const load = async () => {
      const [b, m, c] = await Promise.all([
        api.getBranding(),
        api.getExecutionMode(),
        api.getCapabilities(),
      ]);
      setBranding(b);
      setTargetMode(m.currentMode);
      setCaps(c);
    };
    load();
  }, []);

  const handleApplyMode = async () => {
    try {
      const updated = await api.setExecutionMode(targetMode, true);
      setModeMsg(`Switched to ${updated.currentMode} mode.`);
    } catch (err: any) {
      setModeMsg(`Error: ${err?.message || err}`);
    }
  };

  const handleToggleConsole = async (enabled: boolean) => {
    setShowConsole(enabled);
    try {
      await api.setConsoleVisible(enabled);
      setConsoleMsg(enabled ? 'Console window attached (visible)' : 'Console window detached (silent)');
    } catch (err: any) {
      setConsoleMsg(`Error: ${err?.message || err}`);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
      {/* Windows Console / Terminal Silent Setting */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Terminal size={16} color="#38bdf8" /> Windows Terminal &amp; Console Visibility
          </span>
          <span className="status-pill pass" style={{ fontSize: '10.5px' }}>
            {showConsole ? 'CONSOLE VISIBLE' : 'SILENT (DEFAULT)'}
          </span>
        </div>
        <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '12px' }}>
          By default, NetPilot launches silently without opening a Windows Command Prompt or Terminal window. Enable the toggle below only if you need to inspect raw standard output/error logs.
        </p>

        <label style={{
          display: 'flex',
          alignItems: 'center',
          gap: '10px',
          padding: '10px 14px',
          background: 'var(--bg-tertiary)',
          borderRadius: 'var(--radius-sm)',
          cursor: 'pointer',
          width: 'fit-content',
        }}>
          <input
            type="checkbox"
            checked={showConsole}
            onChange={e => handleToggleConsole(e.target.checked)}
            style={{ accentColor: '#10b981', cursor: 'pointer', width: '16px', height: '16px' }}
          />
          <div>
            <div style={{ fontSize: '13px', fontWeight: 600, color: '#f8fafc' }}>
              Show Windows Console Window
            </div>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
              Attaches an active kernel32.AllocConsole window for real-time Go runtime logs
            </div>
          </div>
        </label>
        {consoleMsg && (
          <div style={{ marginTop: '8px', fontSize: '11.5px', color: '#4ade80' }}>
            {consoleMsg}
          </div>
        )}
      </div>

      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <SettingsIcon size={16} color="#60a5fa" /> Application Identity &amp; System Info
          </span>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '14px', fontSize: '12px' }}>
          <div>
            <div style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>Product Name</div>
            <div style={{ fontWeight: 600, fontSize: '13.5px' }}>{branding?.name}</div>
          </div>
          <div>
            <div style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>Application Identifier</div>
            <div className="font-mono">{branding?.id}</div>
          </div>
          <div>
            <div style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>Version</div>
            <div className="font-mono">v{branding?.version}</div>
          </div>
          <div>
            <div style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>Environment</div>
            <div>{caps?.environment.runtimeOs} / {caps?.environment.runtimeArch}</div>
          </div>
        </div>
      </div>

      {/* Execution Posture & Operational Modes */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <ShieldAlert size={16} color="#10b981" /> Operational Modes
          </span>
        </div>
        <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '12px' }}>
          Select runtime posture. Live mode inspects and routes real system traffic across network adapters. Dry-run mode evaluates policies without applying modifications.
        </p>

        <div style={{ display: 'flex', gap: '12px', marginBottom: '12px' }}>
          <label style={{ flex: 1, display: 'flex', alignItems: 'flex-start', gap: '10px', padding: '10px 14px', background: 'var(--bg-tertiary)', borderRadius: 'var(--radius-sm)', cursor: 'pointer' }}>
            <input
              type="radio"
              name="mode"
              value="LIVE"
              checked={targetMode === 'LIVE'}
              onChange={() => setTargetMode('LIVE')}
              style={{ marginTop: '3px', accentColor: '#10b981' }}
            />
            <div>
              <div style={{ fontSize: '12.5px', fontWeight: 600, color: '#10b981' }}>Live Mode (Active Host Mode)</div>
              <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                Real operating system traffic is inspected and managed across physical adapters and active processes.
              </div>
            </div>
          </label>

          <label style={{ flex: 1, display: 'flex', alignItems: 'flex-start', gap: '10px', padding: '10px 14px', background: 'var(--bg-tertiary)', borderRadius: 'var(--radius-sm)', cursor: 'pointer' }}>
            <input
              type="radio"
              name="mode"
              value="DRY_RUN"
              checked={targetMode === 'DRY_RUN'}
              onChange={() => setTargetMode('DRY_RUN')}
              style={{ marginTop: '3px', accentColor: '#f59e0b' }}
            />
            <div>
              <div style={{ fontSize: '12.5px', fontWeight: 600, color: '#f59e0b' }}>Dry-Run Mode</div>
              <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                Inspects real system network state and calculates policy decisions without altering routes or firewall rules.
              </div>
            </div>
          </label>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <button className="btn btn-primary btn-sm" onClick={handleApplyMode}>
            Apply Execution Mode
          </button>
          {modeMsg && (
            <span style={{ fontSize: '12px', color: modeMsg.startsWith('Error') ? '#f87171' : '#4ade80' }}>
              {modeMsg}
            </span>
          )}
        </div>
      </div>
    </div>
  );
};
