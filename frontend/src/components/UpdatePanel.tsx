import React, { useCallback, useEffect, useState } from 'react';
import {
  AlertCircle,
  ArrowUpCircle,
  CheckCircle2,
  DownloadCloud,
  ExternalLink,
  RefreshCw,
  Rocket,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';
import { api, UpdateChannel, UpdateStatus } from '../api/wails';

const BUSY_STATES = new Set(['checking', 'downloading', 'installing', 'restarting']);
const POLL_STATES = new Set(['downloading', 'installing', 'restarting']);

const channelLabel = (ch?: UpdateChannel) => (ch === 'beta' ? 'Beta' : 'Stable');

const fmtBytes = (bytes?: number) => {
  if (!bytes || bytes <= 0) return '0 B';
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${bytes} B`;
};

const fmtDate = (value?: string | null) => {
  if (!value) return '';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
};

export const UpdatePanel: React.FC = () => {
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [error, setError] = useState('');
  const [checking, setChecking] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [changing, setChanging] = useState(false);

  const load = useCallback(async () => {
    try {
      setStatus(await api.getUpdateStatus());
    } catch (err: any) {
      setError(err?.message || String(err));
    }
  }, []);

  useEffect(() => {
    load().catch(e => console.error(e));
  }, [load]);

  useEffect(() => {
    // `installing` covers the long install request: the backend reports
    // download/install progress while that fetch is still open, so poll until
    // it returns instead of only waiting for a state change. Outside an active
    // transfer a slow poll picks up the background check result without
    // triggering extra GitHub requests.
    const busy = installing || (!!status && POLL_STATES.has(status.state));
    const id = setInterval(() => {
      load().catch(() => {});
    }, busy ? 1000 : 30000);
    return () => clearInterval(id);
  }, [installing, status?.state, load]);

  const check = async () => {
    setChecking(true);
    setError('');
    try {
      setStatus(await api.checkForUpdate());
    } catch (err: any) {
      setError(err?.message || String(err));
    } finally {
      setChecking(false);
    }
  };

  const chooseChannel = async (channel: UpdateChannel) => {
    if (changing || status?.channel === channel) return;
    setChanging(true);
    setError('');
    try {
      await api.setUpdateChannel(channel);
      setStatus(await api.checkForUpdate());
    } catch (err: any) {
      setError(err?.message || String(err));
    } finally {
      setChanging(false);
    }
  };

  const toggleAutoCheck = async () => {
    if (!status || changing) return;
    setChanging(true);
    setError('');
    try {
      const next = await api.setAutoUpdateCheck(!status.autoCheck);
      setStatus(next);
      if (next.autoCheck && !next.checkedAt) {
        setStatus(await api.checkForUpdate());
      }
    } catch (err: any) {
      setError(err?.message || String(err));
    } finally {
      setChanging(false);
    }
  };

  const install = async () => {
    setInstalling(true);
    setError('');
    try {
      setStatus(await api.installUpdate());
    } catch (err: any) {
      setError(err?.message || String(err));
      await load();
    } finally {
      setInstalling(false);
    }
  };

  const busy = checking || installing || changing || (!!status && BUSY_STATES.has(status.state));
  const downloading = status?.state === 'downloading';
  const progress = Math.max(0, Math.min(1, status?.progress || 0));
  const upToDate = !!status && !!status.checkedAt && !status.updateAvailable && status.state !== 'error';
  const activeChannel: UpdateChannel = status?.channel === 'beta' ? 'beta' : 'stable';

  return (
    <div className="card" style={{ marginBottom: 0 }}>
      <div className="card-title">
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <RefreshCw size={15} color="var(--text-info)" />
          <span>Automatic Updates</span>
        </div>
        <span className={`status-pill ${activeChannel === 'beta' ? 'warn' : 'pass'}`}>
          {channelLabel(activeChannel).toUpperCase()} CHANNEL
        </span>
      </div>

      <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '12px' }}>
        NetPilot can check GitHub for a newer build, verify its published SHA-256 checksum, then install and
        restart itself. Choose <strong>Stable</strong> for production releases or <strong>Beta</strong> to
        receive prerelease builds as soon as they are published.
      </p>

      <div className="grid-2" style={{ gap: '12px', marginBottom: '12px' }}>
        <div
          onClick={() => chooseChannel('stable')}
          style={{
            padding: '13px 15px',
            borderRadius: '8px',
            cursor: busy ? 'default' : 'pointer',
            background: activeChannel === 'stable' ? 'rgba(16, 185, 129, 0.08)' : 'var(--bg-primary)',
            border: `1px solid ${activeChannel === 'stable' ? 'var(--accent-primary)' : 'var(--border-control)'}`,
            opacity: busy ? 0.7 : 1,
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '4px' }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: '7px', fontWeight: 700, fontSize: '13px', color: 'var(--text-primary)' }}>
              <ShieldCheck size={14} color="var(--accent-primary)" />
              Stable
            </span>
            {activeChannel === 'stable' ? (
              <span className="status-pill pass">SELECTED</span>
            ) : (
              <span className="status-pill pass" style={{ opacity: 0.65 }}>RECOMMENDED</span>
            )}
          </div>
          <div style={{ fontSize: '11.5px', color: 'var(--text-secondary)' }}>
            Only production releases. New features arrive after they have been tested.
          </div>
        </div>

        <div
          onClick={() => chooseChannel('beta')}
          style={{
            padding: '13px 15px',
            borderRadius: '8px',
            cursor: busy ? 'default' : 'pointer',
            background: activeChannel === 'beta' ? 'rgba(245, 158, 11, 0.08)' : 'var(--bg-primary)',
            border: `1px solid ${activeChannel === 'beta' ? 'var(--accent-warning)' : 'var(--border-control)'}`,
            opacity: busy ? 0.7 : 1,
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '4px' }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: '7px', fontWeight: 700, fontSize: '13px', color: 'var(--text-primary)' }}>
              <Sparkles size={14} color="var(--accent-warning)" />
              Beta
            </span>
            {activeChannel === 'beta' ? (
              <span className="status-pill warn">SELECTED</span>
            ) : (
              <span className="status-pill warn" style={{ opacity: 0.65 }}>PRERELEASE</span>
            )}
          </div>
          <div style={{ fontSize: '11.5px', color: 'var(--text-secondary)' }}>
            Beta, release-candidate and other prerelease builds, plus every stable release.
          </div>
        </div>
      </div>

      <label
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          fontSize: '12px',
          cursor: busy ? 'default' : 'pointer',
          fontWeight: 500,
          color: 'var(--text-primary)',
          marginBottom: '12px',
        }}
      >
        <input
          type="checkbox"
          checked={!!status?.autoCheck}
          disabled={!status || busy}
          onChange={toggleAutoCheck}
        />
        <span>Check for updates automatically every 6 hours and notify me</span>
      </label>

      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
        <button className="btn btn-secondary btn-sm" onClick={check} disabled={busy}>
          <RefreshCw size={12} />
          <span>{checking || status?.state === 'checking' ? 'Checking…' : 'Check now'}</span>
        </button>

        {status?.updateAvailable && status.installSupported && (
          <button className="btn btn-primary btn-sm" onClick={install} disabled={busy}>
            <DownloadCloud size={12} />
            <span>{downloading ? 'Downloading…' : installing ? 'Starting…' : `Install NetPilot ${status.latest}`}</span>
          </button>
        )}

        {status?.updateAvailable && status.installSupported && (
          <span style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '10.5px', color: 'var(--text-muted)' }}>
            <Rocket size={11} />
            {status.installNote || 'NetPilot will restart to finish the update.'}
          </span>
        )}

        {status && (!status.updateAvailable || !status.installSupported) && (
          <a
            className="btn btn-secondary btn-sm"
            href={status.releasesUrl}
            target="_blank"
            rel="noreferrer"
            style={{ textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '5px' }}
          >
            <ExternalLink size={12} />
            <span>Open releases</span>
          </a>
        )}
      </div>

      {error && (
        <div
          style={{
            marginTop: '12px',
            padding: '9px 12px',
            borderRadius: '7px',
            background: 'rgba(244, 63, 94, 0.12)',
            border: '1px solid rgba(244, 63, 94, 0.35)',
            color: 'var(--text-danger)',
            fontSize: '11.5px',
            display: 'flex',
            alignItems: 'flex-start',
            gap: '7px',
          }}
        >
          <AlertCircle size={14} style={{ marginTop: '1px', flexShrink: 0 }} />
          <span>{error}</span>
        </div>
      )}

      {!error && status?.state === 'error' && status.error && (
        <div
          style={{
            marginTop: '12px',
            padding: '9px 12px',
            borderRadius: '7px',
            background: 'rgba(244, 63, 94, 0.12)',
            border: '1px solid rgba(244, 63, 94, 0.35)',
            color: 'var(--text-danger)',
            fontSize: '11.5px',
            display: 'flex',
            alignItems: 'flex-start',
            gap: '7px',
          }}
        >
          <AlertCircle size={14} style={{ marginTop: '1px', flexShrink: 0 }} />
          <span>{status.error}</span>
        </div>
      )}

      {status?.updateAvailable && (
        <div
          style={{
            marginTop: '12px',
            padding: '11px 13px',
            borderRadius: '7px',
            background: 'var(--bg-primary)',
            border: '1px solid rgba(56, 189, 248, 0.28)',
            fontSize: '11.5px',
            lineHeight: 1.65,
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '7px', flexWrap: 'wrap' }}>
            <ArrowUpCircle size={15} color="var(--text-info)" />
            <span style={{ fontWeight: 700, color: 'var(--text-primary)' }}>
              NetPilot {status.latest} is available
            </span>
            {status.prerelease && <span className="status-pill warn">PRERELEASE</span>}
            <span style={{ color: 'var(--text-muted)' }}>
              you have {status.current}
              {fmtDate(status.publishedAt) ? ` · published ${fmtDate(status.publishedAt)}` : ''}
            </span>
          </div>

          {status.releaseName && status.releaseName !== status.latest && (
            <div style={{ color: 'var(--text-secondary)', marginTop: '3px' }}>{status.releaseName}</div>
          )}

          {status.assetName && (
            <div style={{ color: 'var(--text-muted)', marginTop: '4px' }}>
              Package: <span className="font-mono">{status.assetName}</span>
              {status.assetSize ? ` · ${fmtBytes(status.assetSize)}` : ''}
              {status.checksumVerified ? (
                <span style={{ color: 'var(--text-success)' }}> · SHA-256 verified</span>
              ) : null}
            </div>
          )}

          {(downloading || status.state === 'installing' || status.state === 'restarting') && (
            <div style={{ marginTop: '9px' }}>
              <div style={{ height: '6px', borderRadius: '999px', background: 'var(--border-control)', overflow: 'hidden' }}>
                <div
                  style={{
                    height: '100%',
                    width: `${Math.round(progress * 100)}%`,
                    background: 'linear-gradient(90deg, var(--text-info), var(--accent-primary))',
                    transition: 'width 250ms ease',
                  }}
                />
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '4px', fontSize: '10.5px', color: 'var(--text-muted)' }}>
                <span>{status.message || 'Downloading…'}</span>
                <span className="font-mono">
                  {downloading
                    ? `${fmtBytes(status.downloaded)} / ${fmtBytes(status.assetSize)}`
                    : `${Math.round(progress * 100)}%`}
                </span>
              </div>
            </div>
          )}

          {!downloading && status.state !== 'installing' && status.state !== 'restarting' && status.installNote && (
            <div style={{ marginTop: '5px', color: 'var(--text-warning)' }}>{status.installNote}</div>
          )}

          <a
            href={status.releasesUrl}
            target="_blank"
            rel="noreferrer"
            style={{ display: 'inline-block', marginTop: '6px', color: 'var(--text-info)' }}
          >
            Release notes
          </a>
        </div>
      )}

      {upToDate && (
        <div style={{ marginTop: '12px', fontSize: '11.5px', color: 'var(--text-success)', display: 'flex', alignItems: 'center', gap: '6px' }}>
          <CheckCircle2 size={14} />
          <span>
            {status.message || `NetPilot ${status.current} is the newest ${channelLabel(activeChannel)} release.`}
          </span>
        </div>
      )}

      {status?.state === 'ready' && (
        <div style={{ marginTop: '12px', fontSize: '11.5px', color: 'var(--text-warning)', display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Rocket size={14} />
          <span>{status.message || 'Update staged. Restart NetPilot to finish installing.'}</span>
        </div>
      )}

      <div style={{ marginTop: '10px', fontSize: '10px', color: 'var(--text-muted)', lineHeight: 1.55 }}>
        {status?.checkedAt
          ? `Last checked ${new Date(status.checkedAt).toLocaleString()}.`
          : 'Not checked yet.'}{' '}
        Downloads come from GitHub Releases. Automatic install is supported for the Windows installer/portable
        build, the macOS app bundle in Applications, and the Linux AppImage.
      </div>
    </div>
  );
};
