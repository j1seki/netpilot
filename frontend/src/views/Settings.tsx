import React, { useEffect, useRef, useState } from 'react';
import {
  api,
  BrandingInfo,
  ExecutionModeState,
  CapabilityReport,
  NetMergeBinaryInfo,
  NetMergeEngineSource,
} from '../api/wails';
import { UpdatePanel } from '../components/UpdatePanel';
import {
  Shield,
  Terminal,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Cpu,
  Minimize2,
  PowerOff,
  Box,
  Download,
  Upload,
  HardDrive,
} from 'lucide-react';

export const Settings: React.FC = () => {
  const [branding, setBranding] = useState<BrandingInfo | null>(null);
  const [modeState, setModeState] = useState<ExecutionModeState | null>(null);
  const [capReport, setCapReport] = useState<CapabilityReport | null>(null);
  const [binInfo, setBinInfo] = useState<NetMergeBinaryInfo | null>(null);
  const [engineSource, setEngineSource] = useState<NetMergeEngineSource>('builtin');
  const [releaseTags, setReleaseTags] = useState<string[]>([]);
  const [releaseTag, setReleaseTag] = useState('');
  const [customPath, setCustomPath] = useState<string>('');
  const [closeAction, setCloseAction] = useState<'minimize' | 'close'>('minimize');
  const [showConsole, setShowConsole] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string>('');
  const [successMsg, setSuccessMsg] = useState<string>('');
  const [savingPath, setSavingPath] = useState<boolean>(false);
  const [configBusy, setConfigBusy] = useState<boolean>(false);
  const importInputRef = useRef<HTMLInputElement | null>(null);

  const loadAll = async () => {
    try {
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
      setEngineSource(bin.engineSource || 'builtin');
      setCustomPath(bin.customPath || '');
      setCloseAction(ca || 'minimize');
      setErrorMsg('');
    } catch (err: any) {
      setErrorMsg(`Could not load settings: ${err?.message || String(err)}`);
    }
  };

  useEffect(() => {
    loadAll().catch((e) => console.error(e));
  }, []);

  const flashSuccess = (msg: string) => {
    setErrorMsg('');
    setSuccessMsg(msg);
    setTimeout(() => setSuccessMsg(''), 4000);
  };

  const handleCloseActionChange = async (action: 'minimize' | 'close') => {
    const previous = closeAction;
    setCloseAction(action);
    try {
      await api.setCloseAction(action);
      flashSuccess(
        action === 'minimize'
          ? 'Close button set to: Minimize to System Tray (Open NetPilot from tray).'
          : 'Close button set to: Close Application completely.'
      );
    } catch (err: any) {
      setCloseAction(previous);
      setErrorMsg(`Could not save close-button behavior: ${err?.message || String(err)}`);
    }
  };

  const handleSaveEngine = async () => {
    setSavingPath(true);
    setErrorMsg('');
    setSuccessMsg('');
    try {
      const updated = engineSource === 'release'
        ? await api.installNetMergeRelease(releaseTag)
        : await api.setNetMergeEngineSource(engineSource, engineSource === 'external' ? customPath.trim() : '');
      setBinInfo(updated);
      flashSuccess(`${updated.version} selected for the next start.`);
    } catch (err: any) {
      setErrorMsg(err.message || 'Could not change the selected engine.');
    } finally {
      setSavingPath(false);
    }
  };

  const loadReleases = async () => {
    setSavingPath(true);
    setErrorMsg('');
    try {
      const tags = await api.getNetMergeReleases();
      setReleaseTags(tags);
      setReleaseTag(tags.includes(binInfo?.releaseTag || '') ? binInfo!.releaseTag! : tags[0] || '');
      if (!tags.length) setErrorMsg('No verified releases are available for this platform.');
    } catch (err: any) {
      setErrorMsg(err.message || 'Could not fetch release versions.');
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
    const previous = showConsole;
    setShowConsole(checked);
    try {
      await api.setConsoleVisible(checked);
    } catch (err: any) {
      setShowConsole(previous);
      setErrorMsg(`Could not change console visibility: ${err?.message || String(err)}`);
    }
  };

  const handleExportConfig = async () => {
    setConfigBusy(true);
    setErrorMsg('');
    try {
      const bundle = await api.exportConfigBundle();
      const blob = new Blob([JSON.stringify(bundle, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `netpilot-config-${new Date().toISOString().slice(0, 10)}.json`);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
      flashSuccess(`Exported ${bundle.rules.length} rules, ${bundle.adapterCaps.length} adapter caps and ${bundle.adapterLabels?.length || 0} labels.`);
    } catch (err: any) {
      setErrorMsg(`Configuration export failed: ${err?.message || String(err)}`);
    } finally {
      setConfigBusy(false);
    }
  };

  const handleImportFile = async (file: File | null) => {
    if (!file) return;
    setConfigBusy(true);
    setErrorMsg('');
    setSuccessMsg('');
    try {
      const text = await file.text();
      const bundle = JSON.parse(text);
      if (!bundle || typeof bundle !== 'object' || !Array.isArray(bundle.rules) || !Array.isArray(bundle.adapterCaps)) {
        throw new Error('This file is not a NetPilot configuration export.');
      }
      const result = await api.importConfigBundle(bundle);
      await loadAll();
      flashSuccess(
        `Restored ${result.rulesImported} rules, ${result.adapterCapsImported} adapter caps, ${result.labelsImported} labels` +
        (result.netMergeImported ? ' and net-merge settings.' : '.')
      );
    } catch (err: any) {
      setErrorMsg(`Configuration import failed: ${err?.message || String(err)}`);
    } finally {
      setConfigBusy(false);
      if (importInputRef.current) importInputRef.current.value = '';
    }
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

      {/* Automatic Updates: stable vs beta channel, checksum-verified install */}
      <UpdatePanel />

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

      {/* Engine version and source selection */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Box size={15} color="#38bdf8" />
            <span>Net-Merge Engine</span>
          </div>
          {binInfo && (
            <span className={`status-pill ${binInfo.isValid ? 'pass' : 'fail'}`}>
              {binInfo.isValid
                ? binInfo.engineSource === 'builtin'
                  ? 'NETPILOT ENGINE'
                  : 'ENGINE VERIFIED'
                : 'ENGINE UNAVAILABLE'}
            </span>
          )}
        </div>

        <p style={{ fontSize: '12px', color: '#a1a1aa', marginBottom: '12px' }}>
          Upgrade or downgrade by selecting an official release. Stop net-merge and its SOCKS proxy before switching. The NetPilot engine is always available for Backup Failover and the in-app SOCKS proxy; official releases use Speed mode.
        </p>

        <div className="engine-summary" aria-live="polite">
          <div>Selected engine: <strong>{binInfo?.version || binInfo?.error || 'Checking…'}</strong></div>
          {binInfo?.bundledVersion && <div>Official release included with this build: <strong>{binInfo.bundledVersion}</strong></div>}
        </div>
        <div className="engine-actions">
          <label htmlFor="engine-source">Engine source</label>
          <select id="engine-source" value={engineSource} disabled={savingPath} onChange={e => setEngineSource(e.target.value as NetMergeEngineSource)}>
            <option value="builtin">NetPilot engine (Backup + Speed)</option>
            {binInfo?.bundledVersion && <option value="bundled">Bundled official release ({binInfo.bundledVersion})</option>}
            <option value="release">Download an official version</option>
            <option value="external">Choose an external executable</option>
          </select>
          {engineSource === 'release' && <>
            <button className="btn btn-secondary btn-sm" disabled={savingPath} onClick={loadReleases}><RefreshCw size={12} /> List releases</button>
            {releaseTags.length > 0 && <select aria-label="Official release version" value={releaseTag} disabled={savingPath} onChange={e => setReleaseTag(e.target.value)}>
              {releaseTags.map(tag => <option key={tag} value={tag}>{tag}</option>)}
            </select>}
          </>}
          {engineSource === 'external' && <input aria-label="Full path to net-merge executable" placeholder="Full path to net-merge executable" value={customPath} disabled={savingPath} onChange={e => setCustomPath(e.target.value)} />}
          <button className="btn btn-primary btn-sm" onClick={handleSaveEngine} disabled={savingPath || (engineSource === 'release' && !releaseTag) || (engineSource === 'external' && !customPath.trim())}>
            {savingPath ? 'Working…' : engineSource === 'release' ? 'Download & use' : 'Use this engine'}
          </button>
        </div>
      </div>

      {/* Configuration Backup & Restore */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <HardDrive size={15} color="#c084fc" />
            <span>Configuration Backup & Restore</span>
          </div>
          <span className="status-pill pass">PORTABLE JSON</span>
        </div>

        <p style={{ fontSize: '12px', color: '#a1a1aa', marginBottom: '12px' }}>
          Export routing rules, adapter speed caps, friendly adapter names and net-merge settings as a single JSON file.
          Import it on another machine or after a reinstall to restore your setup.
        </p>

        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          <button
            className="btn btn-primary btn-sm"
            onClick={handleExportConfig}
            disabled={configBusy}
            style={{ display: 'flex', alignItems: 'center', gap: '5px' }}
          >
            <Download size={12} />
            {configBusy ? 'Working...' : 'Export Configuration'}
          </button>

          <button
            className="btn btn-secondary btn-sm"
            onClick={() => importInputRef.current?.click()}
            disabled={configBusy}
            style={{ display: 'flex', alignItems: 'center', gap: '5px' }}
          >
            <Upload size={12} />
            Import Configuration
          </button>

          <input
            ref={importInputRef}
            type="file"
            accept="application/json,.json"
            style={{ display: 'none' }}
            onChange={e => handleImportFile(e.target.files?.[0] || null)}
          />
        </div>
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
