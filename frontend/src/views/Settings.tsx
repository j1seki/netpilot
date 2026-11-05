import React, { useEffect, useState } from 'react';
import { api, BrandingInfo, CapabilityReport, NetMergeBinaryInfo } from '../api/wails';
import {
  Settings as SettingsIcon,
  ShieldAlert,
  Terminal,
  CheckCircle2,
  AlertTriangle,
  RefreshCw,
  Cpu,
} from 'lucide-react';

export const Settings: React.FC = () => {
  const [branding, setBranding] = useState<BrandingInfo | null>(null);
  const [targetMode, setTargetMode] = useState<'LIVE' | 'DRY_RUN'>('LIVE');
  const [modeMsg, setModeMsg] = useState('');
  const [showConsole, setShowConsole] = useState<boolean>(false);
  const [consoleMsg, setConsoleMsg] = useState('');
  const [caps, setCaps] = useState<CapabilityReport | null>(null);

  // net-merge Engine Binary state
  const [binaryInfo, setBinaryInfo] = useState<NetMergeBinaryInfo | null>(null);
  const [inputBinaryPath, setInputBinaryPath] = useState<string>('');
  const [binaryMsg, setBinaryMsg] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null);
  const [isValidating, setIsValidating] = useState<boolean>(false);

  const loadData = async () => {
    const [b, m, c, bin] = await Promise.all([
      api.getBranding(),
      api.getExecutionMode(),
      api.getCapabilities(),
      api.getNetMergeBinaryInfo(),
    ]);
    setBranding(b);
    setTargetMode(m.currentMode);
    setCaps(c);
    setBinaryInfo(bin);
    setInputBinaryPath(bin.customPath || bin.path || '');
  };

  useEffect(() => {
    loadData();
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

  const handleSaveBinaryPath = async () => {
    setIsValidating(true);
    setBinaryMsg(null);
    try {
      const updated = await api.setNetMergeBinaryPath(inputBinaryPath);
      setBinaryInfo(updated);
      if (updated.isValid) {
        setBinaryMsg({
          type: 'success',
          text: `Verified & Connected: ${updated.version} (Source: ${updated.source})`,
        });
      } else {
        setBinaryMsg({
          type: 'error',
          text: updated.error || 'Binary failed validation at specified path.',
        });
      }
    } catch (err: any) {
      setBinaryMsg({
        type: 'error',
        text: err?.message || String(err),
      });
    } finally {
      setIsValidating(false);
    }
  };

  const handleResetBinaryPath = async () => {
    setIsValidating(true);
    setBinaryMsg(null);
    try {
      const updated = await api.setNetMergeBinaryPath('');
      setInputBinaryPath(updated.path || '');
      setBinaryInfo(updated);
      if (updated.isValid) {
        setBinaryMsg({
          type: 'success',
          text: `Auto-detected from default location: ${updated.path} (${updated.version})`,
        });
      } else {
        setBinaryMsg({
          type: 'info',
          text: 'Reset to auto-detect. Place net-merge in the same folder as NetPilot or install it in PATH.',
        });
      }
    } catch (err: any) {
      setBinaryMsg({
        type: 'error',
        text: err?.message || String(err),
      });
    } finally {
      setIsValidating(false);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
      {/* net-merge Engine Location & Integration Card */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Cpu size={16} color="#38bdf8" /> net-merge Engine Executable &amp; Path
          </span>
          {binaryInfo?.isValid ? (
            <span className="status-pill pass" style={{ fontSize: '10.5px' }}>
              CONNECTED ({binaryInfo.version || 'ACTIVE'})
            </span>
          ) : (
            <span className="status-pill fail" style={{ fontSize: '10.5px' }}>
              ENGINE NOT FOUND
            </span>
          )}
        </div>
        <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '12px' }}>
          By default, NetPilot checks for <code>net-merge</code> in the <strong>same folder</strong> as the app (or in your system PATH). If you built or placed <code>net-merge</code> in a custom location, paste its full path below.
        </p>

        {/* Binary Status Details */}
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(4, 1fr)',
          gap: '12px',
          background: 'var(--bg-tertiary)',
          padding: '12px 14px',
          borderRadius: 'var(--radius-sm)',
          marginBottom: '14px',
          fontSize: '12px',
        }}>
          <div>
            <div style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>Detection Status</div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontWeight: 600 }}>
              {binaryInfo?.isValid ? (
                <>
                  <CheckCircle2 size={14} color="#10b981" />
                  <span style={{ color: '#10b981' }}>Verified</span>
                </>
              ) : (
                <>
                  <AlertTriangle size={14} color="#f87171" />
                  <span style={{ color: '#f87171' }}>Not Detected</span>
                </>
              )}
            </div>
          </div>

          <div>
            <div style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>Resolution Source</div>
            <div style={{ fontWeight: 600, textTransform: 'capitalize' }}>
              {binaryInfo?.source === 'same-folder'
                ? 'Same Folder (Default)'
                : binaryInfo?.source === 'custom'
                ? 'Custom Path'
                : binaryInfo?.source === 'path'
                ? 'System PATH'
                : binaryInfo?.source === 'system'
                ? 'System Directory'
                : 'None'}
            </div>
          </div>

          <div style={{ gridColumn: 'span 2' }}>
            <div style={{ color: 'var(--text-muted)', fontSize: '10.5px' }}>Active Executable Path</div>
            <div className="font-mono" style={{ fontSize: '11.5px', wordBreak: 'break-all', color: binaryInfo?.isValid ? '#38bdf8' : 'var(--text-muted)' }}>
              {binaryInfo?.path || 'No executable resolved'}
            </div>
          </div>
        </div>

        {/* Input & Action Buttons */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <label style={{ fontSize: '11px', fontWeight: 600, color: 'var(--text-secondary)' }}>
            Custom net-merge Path (override auto-detection):
          </label>
          <div style={{ display: 'flex', gap: '8px' }}>
            <input
              type="text"
              placeholder="/usr/local/bin/net-merge or C:\path\to\net-merge.exe"
              value={inputBinaryPath}
              onChange={(e) => setInputBinaryPath(e.target.value)}
              className="font-mono"
              style={{
                flex: 1,
                padding: '8px 12px',
                background: 'var(--bg-secondary)',
                border: '1px solid var(--border-color)',
                borderRadius: 'var(--radius-sm)',
                color: '#f8fafc',
                fontSize: '12px',
              }}
            />
            <button
              className="btn btn-primary btn-sm"
              onClick={handleSaveBinaryPath}
              disabled={isValidating}
              style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
            >
              {isValidating && <RefreshCw size={12} className="spin" />}
              Save &amp; Verify Path
            </button>
            <button
              className="btn btn-secondary btn-sm"
              onClick={handleResetBinaryPath}
              disabled={isValidating}
              title="Reset custom override and use auto-detection (same folder)"
            >
              Reset to Same Folder
            </button>
          </div>

          {binaryMsg && (
            <div style={{
              marginTop: '4px',
              fontSize: '11.5px',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              color: binaryMsg.type === 'success' ? '#4ade80' : binaryMsg.type === 'error' ? '#f87171' : '#38bdf8',
            }}>
              {binaryMsg.type === 'success' ? <CheckCircle2 size={13} /> : <AlertTriangle size={13} />}
              <span>{binaryMsg.text}</span>
            </div>
          )}
        </div>
      </div>

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
