import React, { useEffect, useState } from 'react';
import {
  api,
  BrandingInfo,
  ExecutionModeState,
  CapabilityReport,
  NetMergeBinaryInfo,
} from '../api/wails';
import {
  Shield,
  Terminal,
  FolderOpen,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Cpu,
  Minimize2,
  PowerOff,
  Box,
} from 'lucide-react';

export const Settings: React.FC = () => {
  const [branding, setBranding] = useState<BrandingInfo | null>(null);
  const [modeState, setModeState] = useState<ExecutionModeState | null>(null);
  const [capReport, setCapReport] = useState<CapabilityReport | null>(null);
  const [binInfo, setBinInfo] = useState<NetMergeBinaryInfo | null>(null);
  const [engineSource, setEngineSource] = useState<'builtin' | 'external'>('builtin');
  const [customPath, setCustomPath] = useState<string>('');
  const [closeAction, setCloseAction] = useState<'minimize' | 'close'>('minimize');
  const [showConsole, setShowConsole] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string>('');
  const [successMsg, setSuccessMsg] = useState<string>('');
  const [savingPath, setSavingPath] = useState<boolean>(false);

  const loadAll = async () => {
    const [b, m, c, bin, ca] = await Promise.all([
      api.getBranding(),
      api.getExecutionMode(),
      api.getCapabilities(),
      api.getNetMergeBinaryInfo(),
      api.getCloseAction(),
    ]);
    setBranding(b);
    setModeState(m);
    setCapReport(c);
    setBinInfo(bin);
    setEngineSource(bin.engineSource === 'external' ? 'external' : 'builtin');
    setCustomPath(bin.customPath || '');
    setCloseAction(ca || 'minimize');
  };

  useEffect(() => {
    loadAll();
  }, []);

  const flashSuccess = (msg: string) => {
    setErrorMsg('');
    setSuccessMsg(msg);
    setTimeout(() => setSuccessMsg(''), 4000);
  };

  const handleCloseActionChange = async (action: 'minimize' | 'close') => {
    setCloseAction(action);
    await api.setCloseAction(action);
    flashSuccess(
      action === 'minimize'
        ? 'Close button set to: Minimize to System Tray (Open NetPilot from tray).'
        : 'Close button set to: Close Application completely.'
    );
  };

  const handleEngineSourceChange = async (source: 'builtin' | 'external') => {
    setEngineSource(source);
    setErrorMsg('');
    if (source === 'builtin') {
      setSavingPath(true);
      try {
        const updated = await api.setNetMergeEngineSource('builtin', customPath);
        setBinInfo(updated);
        flashSuccess('Switched to Built-in embedded net-merge + net-merge-tray.');
      } catch (err: any) {
        setErrorMsg(err.message || 'Failed to switch to built-in net-merge.');
      } finally {
        setSavingPath(false);
      }
    }
  };

  const handleSaveExternalBinary = async () => {
    setSavingPath(true);
    setErrorMsg('');
    setSuccessMsg('');
    try {
      if (!customPath.trim()) {
        setErrorMsg('Please enter the full path to the external net-merge binary.');
        return;
      }
      const updated = await api.setNetMergeEngineSource('external', customPath.trim());
      setBinInfo(updated);
      flashSuccess(`Verified and activated external net-merge binary: ${updated.path}`);
    } catch (err: any) {
      const updated = await api.getNetMergeBinaryInfo();
      setBinInfo(updated);
      setErrorMsg(err.message || 'Invalid external net-merge executable path.');
    } finally {
      setSavingPath(false);
    }
  };

  const handleModeSwitch = async (target: 'LIVE' | 'DRY_RUN') => {
    setErrorMsg('');
    try {
      const st = await api.setExecutionMode(target, true);
      setModeState(st);
      flashSuccess(`Execution mode switched to ${target}.`);
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to switch mode');
    }
  };

  const handleToggleConsole = async (checked: boolean) => {
    setShowConsole(checked);
    await api.setConsoleVisible(checked);
  };

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
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <AlertCircle size={15} />
          <span>{errorMsg}</span>
        </div>
      )}

      {successMsg && (
        <div
          style={{
            padding: '10px 14px',
            borderRadius: '8px',
            background: 'rgba(16, 185, 129, 0.14)',
            border: '1px solid rgba(16, 185, 129, 0.35)',
            color: '#34d399',
            fontSize: '12px',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <CheckCircle2 size={15} />
          <span>{successMsg}</span>
        </div>
      )}

      {/* Window & System Tray Behavior Settings */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Minimize2 size={15} color="#10b981" />
            <span>Window & System Tray Behavior</span>
          </div>
          <span className="status-pill pass">
            {closeAction === 'minimize' ? 'DEFAULT: MINIMIZE TO TRAY' : 'CLOSE APP'}
          </span>
        </div>

        <p style={{ fontSize: '12px', color: '#a1a1aa', marginBottom: '12px' }}>
          Choose what happens when you click the window close (X) button. By default, NetPilot minimizes to the system tray so you can reopen NetPilot from the tray at any time.
        </p>

        <div className="grid-2" style={{ gap: '12px' }}>
          <div
            onClick={() => handleCloseActionChange('minimize')}
            style={{
              padding: '14px 16px',
              borderRadius: '8px',
              cursor: 'pointer',
              background: closeAction === 'minimize' ? 'rgba(16, 185, 129, 0.08)' : '#09090b',
              border: `1px solid ${closeAction === 'minimize' ? '#10b981' : '#27272a'}`,
              display: 'flex',
              flexDirection: 'column',
              gap: '6px',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '7px', fontWeight: 700, fontSize: '13px', color: '#fafafa' }}>
                <Minimize2 size={14} color="#10b981" />
                <span>Minimize to System Tray (Default)</span>
              </div>
              {closeAction === 'minimize' && (
                <span className="status-pill pass">ACTIVE</span>
              )}
            </div>
            <div style={{ fontSize: '11.5px', color: '#a1a1aa' }}>
              Clicking the close button hides the window to the system tray. Click the NetPilot tray icon or choose "Open NetPilot" from the tray menu to restore.
            </div>
          </div>

          <div
            onClick={() => handleCloseActionChange('close')}
            style={{
              padding: '14px 16px',
              borderRadius: '8px',
              cursor: 'pointer',
              background: closeAction === 'close' ? 'rgba(244, 63, 94, 0.08)' : '#09090b',
              border: `1px solid ${closeAction === 'close' ? '#f43f5e' : '#27272a'}`,
              display: 'flex',
              flexDirection: 'column',
              gap: '6px',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '7px', fontWeight: 700, fontSize: '13px', color: '#fafafa' }}>
                <PowerOff size={14} color="#f43f5e" />
                <span>Quit / Close Application</span>
              </div>
              {closeAction === 'close' && (
                <span className="status-pill fail">ACTIVE</span>
              )}
            </div>
            <div style={{ fontSize: '11.5px', color: '#a1a1aa' }}>
              Clicking the close button exits NetPilot completely instead of keeping it running in the system tray.
            </div>
          </div>
        </div>
      </div>

      {/* Net-Merge Engine Binary Selection: Built-in vs External Full Path */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Box size={15} color="#38bdf8" />
            <span>Net-Merge Engine Source (Built-in vs External Binary)</span>
          </div>
          {binInfo && (
            <span className={`status-pill ${binInfo.isValid ? 'pass' : 'fail'}`}>
              {binInfo.isValid
                ? engineSource === 'builtin'
                  ? 'BUILT-IN (EMBEDDED + TRAY)'
                  : 'EXTERNAL BINARY VERIFIED'
                : 'BINARY NOT FOUND'}
            </span>
          )}
        </div>

        <p style={{ fontSize: '12px', color: '#a1a1aa', marginBottom: '12px' }}>
          NetPilot embeds both <strong>net-merge</strong> and <strong>net-merge-tray</strong> directly out of the box. You can also switch to an external <code>net-merge</code> binary defined by its full path.
        </p>

        <div className="grid-2" style={{ gap: '12px', marginBottom: '12px' }}>
          {/* Option 1: Built-in Net-Merge */}
          <div
            onClick={() => handleEngineSourceChange('builtin')}
            style={{
              padding: '13px 15px',
              borderRadius: '8px',
              cursor: 'pointer',
              background: engineSource === 'builtin' ? 'rgba(56, 189, 248, 0.08)' : '#09090b',
              border: `1px solid ${engineSource === 'builtin' ? '#38bdf8' : '#27272a'}`,
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '4px' }}>
              <span style={{ fontWeight: 700, fontSize: '13px', color: '#fafafa' }}>
                Use Built-in Net-Merge + Tray (Default)
              </span>
              {engineSource === 'builtin' && <span className="status-pill pass">SELECTED</span>}
            </div>
            <div style={{ fontSize: '11.5px', color: '#a1a1aa' }}>
              Uses the embedded <code>net-merge</code> and <code>net-merge-tray</code> bundled inside NetPilot. Automatically kept up-to-date with upstream releases.
            </div>
          </div>

          {/* Option 2: External Full Path */}
          <div
            onClick={() => setEngineSource('external')}
            style={{
              padding: '13px 15px',
              borderRadius: '8px',
              cursor: 'pointer',
              background: engineSource === 'external' ? 'rgba(245, 158, 11, 0.08)' : '#09090b',
              border: `1px solid ${engineSource === 'external' ? '#f59e0b' : '#27272a'}`,
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '4px' }}>
              <span style={{ fontWeight: 700, fontSize: '13px', color: '#fafafa' }}>
                Use External Net-Merge (Full Path)
              </span>
              {engineSource === 'external' && <span className="status-pill warn">SELECTED</span>}
            </div>
            <div style={{ fontSize: '11.5px', color: '#a1a1aa' }}>
              Specify the full filesystem path to a custom or externally installed <code>net-merge</code> / <code>net-merge.exe</code> binary.
            </div>
          </div>
        </div>

        {/* External Path Input when External is selected */}
        {engineSource === 'external' && (
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center', marginBottom: '12px' }}>
            <FolderOpen size={15} color="#a1a1aa" />
            <input
              type="text"
              placeholder="Full path to net-merge (e.g. C:\Tools\net-merge.exe or /usr/local/bin/net-merge)"
              value={customPath}
              onChange={e => setCustomPath(e.target.value)}
              style={{ flex: 1 }}
            />
            <button
              className="btn btn-primary btn-sm"
              onClick={handleSaveExternalBinary}
              disabled={savingPath}
            >
              {savingPath ? 'Verifying...' : 'Verify & Use External'}
            </button>
          </div>
        )}

        {/* Resolved Binary Status Box */}
        {binInfo && (
          <div
            style={{
              padding: '10px 14px',
              borderRadius: '7px',
              background: '#09090b',
              border: `1px solid ${binInfo.isValid ? 'rgba(16, 185, 129, 0.28)' : 'rgba(244, 63, 94, 0.35)'}`,
              fontSize: '11.5px',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
            }}
          >
            <div style={{ display: 'flex', flexDirection: 'column', gap: '3px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                {binInfo.isValid ? (
                  <CheckCircle2 size={14} color="#10b981" />
                ) : (
                  <AlertCircle size={14} color="#f43f5e" />
                )}
                <span style={{ fontWeight: 600, color: '#fafafa' }}>
                  Active Engine:{' '}
                  <span className="font-mono" style={{ color: binInfo.isValid ? '#34d399' : '#fb7185' }}>
                    {binInfo.path || 'Not found'}
                  </span>
                </span>
              </div>
              <div style={{ color: '#a1a1aa', paddingLeft: '20px' }}>
                {binInfo.isValid
                  ? `Version: ${binInfo.version} · Source: ${binInfo.engineSource || binInfo.source}`
                  : `Error: ${binInfo.error}`}
              </div>
            </div>

            <button
              className="btn btn-secondary btn-sm"
              onClick={loadAll}
              title="Re-verify binary"
            >
              <RefreshCw size={12} />
            </button>
          </div>
        )}
      </div>

      {/* Windows Console & Execution Mode */}
      <div className="grid-2" style={{ gap: '14px' }}>
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Terminal size={15} color="#f59e0b" />
              <span>Console & Silent Subprocess Control</span>
            </div>
            <span className="status-pill pass">CREATE_NO_WINDOW ACTIVE</span>
          </div>

          <p style={{ fontSize: '11.5px', color: '#a1a1aa', marginBottom: '12px' }}>
            All background probes, PowerShell/netsh commands, and <code>net-merge</code> child processes run silently with <code>CREATE_NO_WINDOW</code> so CMD windows never pop up on Windows.
          </p>

          <label
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              fontSize: '12px',
              cursor: 'pointer',
              fontWeight: 500,
              color: '#fafafa',
            }}
          >
            <input
              type="checkbox"
              checked={showConsole}
              onChange={e => handleToggleConsole(e.target.checked)}
            />
            <span>Show Debug Console Window (Windows Only)</span>
          </label>
        </div>

        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Shield size={15} color="#10b981" />
              <span>Execution Mode: {modeState?.currentMode}</span>
            </div>
          </div>

          <p style={{ fontSize: '11.5px', color: '#a1a1aa', marginBottom: '12px' }}>
            {modeState?.description}
          </p>

          <div style={{ display: 'flex', gap: '8px' }}>
            {(['LIVE', 'DRY_RUN'] as const).map(m => (
              <button
                key={m}
                className={`btn btn-sm ${modeState?.currentMode === m ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => handleModeSwitch(m)}
              >
                {m === 'LIVE' ? 'Live Mode (Default)' : 'Dry-Run Inspection'}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Host & Capability Matrix */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Cpu size={15} color="#38bdf8" />
            <span>Runtime Environment & Branding</span>
          </div>
        </div>

        <div className="grid-2" style={{ fontSize: '12px', color: '#a1a1aa' }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
            <div><strong style={{ color: '#fafafa' }}>Application:</strong> {branding?.name} v{branding?.version}</div>
            <div><strong style={{ color: '#fafafa' }}>Bundle ID:</strong> <span className="font-mono">{branding?.id}</span></div>
            <div><strong style={{ color: '#fafafa' }}>Embedded Engine:</strong> <span className="font-mono">net-merge + net-merge-tray</span></div>
          </div>
          {capReport && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
              <div><strong style={{ color: '#fafafa' }}>OS / Arch:</strong> {capReport.environment.runtimeOs}/{capReport.environment.runtimeArch}</div>
              <div><strong style={{ color: '#fafafa' }}>Kernel:</strong> <span className="font-mono">{capReport.environment.kernelRelease}</span></div>
              <div><strong style={{ color: '#fafafa' }}>Privileges:</strong> {capReport.environment.isElevated ? 'Elevated (Admin/Root)' : 'Standard User'}</div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
