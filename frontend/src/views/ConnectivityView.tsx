import React, { useEffect, useRef, useState } from 'react';
import {
  api,
  ConnectionProfile,
  HotspotProfile,
  HotspotStatus,
  NetMergeConfig,
  NetMergeStatus,
  SpeedtestResult,
  UpdateStatus,
} from '../api/wails';
import { TTLView } from './TTLView';
import {
  Bell,
  CheckCircle2,
  DownloadCloud,
  Gauge,
  Layers,
  Radio,
  RefreshCw,
  Save,
  ShieldCheck,
  Sliders,
  Smartphone,
  Trash2,
  Zap,
} from 'lucide-react';

const gradeColor = (grade: string) => {
  switch (grade) {
    case 'A': return '#10b981';
    case 'B': return '#38bdf8';
    case 'C': return '#facc15';
    case 'D': return '#fb923c';
    case 'F': return '#fb7185';
    default: return '#a1a1aa';
  }
};

const fmtMbps = (v: number) => (v >= 1 ? `${v.toFixed(1)} Mbps` : `${Math.round(v * 1000)} Kbps`);

export const ConnectivityView: React.FC<{ initialTTL?: boolean }> = ({ initialTTL = false }) => {
  const [showTTL, setShowTTL] = useState(initialTTL);
  const [hotspot, setHotspot] = useState<HotspotStatus | null>(null);
  const [status, setStatus] = useState<NetMergeStatus | null>(null);
  const [profiles, setProfiles] = useState<ConnectionProfile[]>([]);
  const [selectedProfile, setSelectedProfile] = useState<string>('hotspot-bypass');
  const [manualTargets, setManualTargets] = useState<string[]>([]);
  const [busy, setBusy] = useState<string>('');
  const [errorMsg, setErrorMsg] = useState<string>('');
  const [notice, setNotice] = useState<string>('');

  const [speedResults, setSpeedResults] = useState<SpeedtestResult[]>([]);
  const [speedRunning, setSpeedRunning] = useState<boolean>(false);
  const [duration, setDuration] = useState<number>(5);
  const [includeMerged, setIncludeMerged] = useState<boolean>(true);

  const [scheduling, setScheduling] = useState<'balanced' | 'low-latency'>('balanced');
  const [probe4, setProbe4] = useState<string>('');
  const [probe6, setProbe6] = useState<string>('');
  const schedInit = useRef(false);
  const profileInit = useRef(false);

  const [profileName, setProfileName] = useState<string>('');
  const [profileDesc, setProfileDesc] = useState<string>('');

  const [update, setUpdate] = useState<UpdateStatus | null>(null);
  const [checkingUpdate, setCheckingUpdate] = useState<boolean>(false);

  const load = async () => {
    try {
      const [hs, st, ps] = await Promise.all([
        api.getHotspotStatus(),
        api.getNetMergeStatus(),
        api.getProfiles(),
      ]);
      setHotspot(hs);
      setStatus(st);
      setProfiles(ps);
      if (!profileInit.current && hs.status?.profile) {
        setSelectedProfile(hs.status.profile);
        profileInit.current = true;
      }
      if (!schedInit.current && st.config) {
        setScheduling(st.config.scheduling === 'low-latency' ? 'low-latency' : 'balanced');
        setProbe4((st.config.probeTargetsV4 || []).join(', '));
        setProbe6((st.config.probeTargetsV6 || []).join(', '));
        schedInit.current = true;
      }
    } catch (err: any) {
      setErrorMsg(`Could not read connectivity state: ${err?.message || String(err)}`);
    }
  };

  useEffect(() => {
    load();
    const interval = setInterval(load, 5000);
    return () => clearInterval(interval);
  }, []);

  const applyHotspot = async () => {
    setBusy('hotspot');
    setErrorMsg('');
    setNotice('');
    try {
      const st = await api.applyHotspot(selectedProfile, false, manualTargets);
      setNotice(st.note || 'Hotspot profile applied.');
      await load();
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setBusy('');
    }
  };

  const revertHotspot = async () => {
    setBusy('hotspot');
    setErrorMsg('');
    setNotice('');
    try {
      const st = await api.revertHotspot();
      setNotice(st.note || 'Hotspot mode reverted.');
      await load();
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setBusy('');
    }
  };

  const toggleAuto = async () => {
    if (!hotspot) return;
    setBusy('auto');
    try {
      await api.setHotspotAuto(!hotspot.status.autoApply);
      await load();
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setBusy('');
    }
  };

  const saveScheduler = async () => {
    if (!status) return;
    setBusy('scheduler');
    setErrorMsg('');
    setNotice('');
    try {
      const split = (v: string) => v.split(/[\s,]+/).map(x => x.trim()).filter(Boolean);
      const cfg: NetMergeConfig = {
        ...status.config,
        scheduling,
        probeTargetsV4: split(probe4),
        probeTargetsV6: split(probe6),
      };
      await api.saveNetMergeConfig(cfg);
      setNotice('Scheduler settings saved.');
      await load();
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setBusy('');
    }
  };

  const runSpeedtest = async () => {
    setSpeedRunning(true);
    setErrorMsg('');
    setNotice('');
    try {
      const res = await api.runSpeedtest({
        durationSec: duration,
        streams: 4,
        includeMerged,
        maxMB: 10,
      });
      setSpeedResults(res);
    } catch (err: any) {
      setErrorMsg(`Speedtest failed: ${err?.message || String(err)}`);
    } finally {
      setSpeedRunning(false);
    }
  };

  const applySavedProfile = async (id: string) => {
    setBusy(`apply-${id}`);
    setErrorMsg('');
    setNotice('');
    try {
      await api.applyProfile(id);
      setNotice('Profile applied.');
      await load();
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setBusy('');
    }
  };

  const saveCurrentProfile = async () => {
    if (!status) return;
    setBusy('save-profile');
    setErrorMsg('');
    try {
      await api.saveProfile({
        id: '',
        name: profileName,
        description: profileDesc,
        netMerge: status.config,
      });
      setProfileName('');
      setProfileDesc('');
      setNotice('Profile saved.');
      await load();
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setBusy('');
    }
  };

  const deleteProfile = async (id: string) => {
    setBusy(`delete-${id}`);
    try {
      await api.deleteProfile(id);
      await load();
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setBusy('');
    }
  };

  const checkUpdate = async () => {
    setCheckingUpdate(true);
    try {
      setUpdate(await api.checkForUpdate());
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setCheckingUpdate(false);
    }
  };

  const testNotification = async () => {
    setBusy('notify');
    try {
      await api.sendTestNotification();
      setNotice('Test notification sent.');
    } catch (err: any) {
      setErrorMsg(`Notifications are not available: ${err?.message || String(err)}`);
    } finally {
      setBusy('');
    }
  };

  const profilesById = (id: string): HotspotProfile | undefined =>
    hotspot?.profiles.find(p => p.id === id);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div>
          <h1 style={{ fontSize: '16px', fontWeight: 700, margin: 0, display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Smartphone size={16} color="#10b981" />
            Mobile &amp; Connectivity
          </h1>
          <p style={{ margin: '4px 0 0', fontSize: '11.5px', color: '#8b8b93' }}>
            Tethering, packet TTL and hop limit, link benchmarks, and saved setups.
          </p>
        </div>
        <button className="btn btn-secondary btn-sm" onClick={load}>
          <RefreshCw size={12} />
          <span>Refresh</span>
        </button>
      </div>

      <nav aria-label="Mobile sections" style={{ display: 'flex', gap: '8px' }}>
        <button className={`btn btn-sm ${showTTL ? 'btn-secondary' : 'btn-emerald'}`} aria-pressed={!showTTL} onClick={() => setShowTTL(false)}>
          <Radio size={13} aria-hidden="true" /> Tethering &amp; Links
        </button>
        <button className={`btn btn-sm ${showTTL ? 'btn-emerald' : 'btn-secondary'}`} aria-pressed={showTTL} onClick={() => setShowTTL(true)}>
          <ShieldCheck size={13} aria-hidden="true" /> TTL &amp; Hop Limit
        </button>
      </nav>

      {showTTL ? <TTLView /> : <>

      {errorMsg && (
        <div className="card" style={{ marginBottom: 0, borderColor: 'rgba(251, 113, 133, 0.35)', color: '#fb7185', fontSize: '11.5px' }}>
          {errorMsg}
        </div>
      )}
      {notice && (
        <div className="card" style={{ marginBottom: 0, borderColor: 'rgba(16, 185, 129, 0.35)', color: '#10b981', fontSize: '11.5px' }}>
          {notice}
        </div>
      )}

      {/* Hotspot */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Radio size={14} color="#38bdf8" />
            <span>Hotspot / Cellular Tethering</span>
          </div>
          <span className={`status-pill ${hotspot?.status.enabled ? 'pass' : hotspot?.available ? 'warn' : 'skipped'}`}>
            {!hotspot ? 'Checking…' : hotspot.status.enabled ? 'Profile saved' : hotspot.available ? 'Candidate found' : 'No match'}
          </span>
        </div>

        {!hotspot?.available && !hotspot?.status.enabled ? (
          <div style={{ fontSize: '11.5px', color: '#a1a1aa', lineHeight: 1.6 }}>
            {!hotspot ? 'Waiting for adapter status.' : 'No tether or cellular adapter was identified by name. Connect your phone by Wi-Fi or USB tethering.'}
            <div style={{ marginTop: '10px', color: '#d4d4d8' }}>
              Select the active Wi-Fi or USB adapter below. iPhone USB tethering on macOS may appear as <span className="font-mono">enN</span>.
            </div>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '10px', marginTop: '8px' }}>
              {(status?.nics || []).map(n => (
                <label key={n.Name} style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                  <input
                    type="checkbox"
                    checked={manualTargets.includes(n.Name)}
                    onChange={e => setManualTargets(prev =>
                      e.target.checked ? [...prev, n.Name] : prev.filter(x => x !== n.Name))}
                  />
                  <span className="font-mono">{n.Name}</span>
                </label>
              ))}
            </div>
            <div style={{ display: 'flex', gap: '8px', marginTop: '10px', alignItems: 'center' }}>
              <select
                className="form-input"
                value={selectedProfile}
                onChange={e => setSelectedProfile(e.target.value)}
                style={{ padding: '5px 8px', fontSize: '11.5px' }}
              >
                {(hotspot?.profiles || []).map(p => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
              <button
                className="btn btn-emerald btn-sm"
                onClick={applyHotspot}
                disabled={!hotspot || manualTargets.length === 0 || busy === 'hotspot'}
              >
                <Zap size={12} />
                 <span>Save tunnel profile</span>
              </button>
            </div>
          </div>
        ) : (
          <>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginBottom: '12px' }}>
              {hotspot.adapters.map(a => (
                <span key={a} className="status-pill pass">{a}</span>
              ))}
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '8px', marginBottom: '12px' }}>
              {hotspot.profiles.map(p => (
                <label
                  key={p.id}
                  style={{
                    display: 'block',
                    padding: '10px',
                    borderRadius: '7px',
                    border: `1px solid ${selectedProfile === p.id ? 'rgba(16, 185, 129, 0.55)' : '#27272a'}`,
                    backgroundColor: selectedProfile === p.id ? 'rgba(16, 185, 129, 0.08)' : '#0f0f12',
                    cursor: 'pointer',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: '7px', fontSize: '12px', fontWeight: 600 }}>
                    <input
                      type="radio"
                      name="hotspot-profile"
                      checked={selectedProfile === p.id}
                      onChange={() => setSelectedProfile(p.id)}
                    />
                    <span>{p.name}</span>
                  </div>
                  <p style={{ margin: '6px 0 0 22px', fontSize: '10.5px', color: '#8b8b93', lineHeight: 1.45 }}>
                    {p.description}
                  </p>
                  <div className="font-mono" style={{ marginLeft: '22px', marginTop: '5px', fontSize: '10px', color: '#71717a' }}>
                     Tunnel MTU {p.mtu} · IPv6 {p.ipv6Mode}
                  </div>
                </label>
              ))}
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
              <button className="btn btn-emerald btn-sm" onClick={applyHotspot} disabled={busy === 'hotspot'}>
                <Zap size={12} />
                 <span>{hotspot.status.enabled ? 'Update tunnel profile' : 'Save tunnel profile'}</span>
              </button>
              {hotspot.status.enabled && (
                <button className="btn btn-secondary btn-sm" onClick={revertHotspot} disabled={busy === 'hotspot'}>
                  <RefreshCw size={12} />
                  <span>Revert</span>
                </button>
              )}
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '11.5px', color: '#a1a1aa', marginLeft: 'auto' }}>
                <input type="checkbox" checked={hotspot.status.autoApply} onChange={toggleAuto} disabled={busy === 'auto'} />
                Auto-apply when a tether appears
              </label>
            </div>

            {hotspot.status.enabled && (
              <div style={{ marginTop: '10px', fontSize: '10.5px', color: '#71717a' }}>
                Profile <span className="font-mono">{profilesById(hotspot.status.profile)?.name || hotspot.status.profile}</span>
                 {' '}saved with {hotspot.status.adapters?.join(', ')} selected at{' '}
                {hotspot.status.appliedAt ? new Date(hotspot.status.appliedAt).toLocaleString() : '—'}.
              </div>
            )}
            {hotspot.status.note && (
              <div style={{ marginTop: '8px', fontSize: '10.5px', color: '#facc15', lineHeight: 1.5 }}>{hotspot.status.note}</div>
            )}
          </>
        )}
      </div>

      {/* Scheduler */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Sliders size={14} color="#a78bfa" />
            <span>Scheduling &amp; Reachability Probes</span>
          </div>
          <button className="btn btn-emerald btn-sm" onClick={saveScheduler} disabled={busy === 'scheduler' || !status}>
            <Save size={12} />
            <span>Save</span>
          </button>
        </div>
        <div className="grid-2" style={{ gap: '12px' }}>
          <div>
            <div className="form-label" style={{ marginBottom: '5px' }}>Flow scheduling</div>
            <select
              className="form-input"
              value={scheduling}
              onChange={e => setScheduling(e.target.value as 'balanced' | 'low-latency')}
              style={{ width: '100%', padding: '6px 8px', fontSize: '11.5px' }}
            >
              <option value="balanced">Balanced — spread flows by adapter weight</option>
              <option value="low-latency">Low latency — prefer the lowest RTT/jitter link</option>
            </select>
            <div style={{ marginTop: '6px', fontSize: '10px', color: '#71717a', lineHeight: 1.5 }}>
              Low-latency mode still uses every healthy adapter, but new flows strongly prefer the
              best-probed link.
            </div>
          </div>
          <div>
            <div className="form-label" style={{ marginBottom: '5px' }}>IPv4 probe targets</div>
            <input
              className="form-input"
              placeholder="1.1.1.1:443, 8.8.8.8:53"
              value={probe4}
              onChange={e => setProbe4(e.target.value)}
              style={{ width: '100%', padding: '6px 8px', fontSize: '11.5px' }}
            />
            <div className="form-label" style={{ margin: '8px 0 5px' }}>IPv6 probe targets</div>
            <input
              className="form-input"
              placeholder="[2606:4700:4700::1111]:443"
              value={probe6}
              onChange={e => setProbe6(e.target.value)}
              style={{ width: '100%', padding: '6px 8px', fontSize: '11.5px' }}
            />
            <div style={{ marginTop: '6px', fontSize: '10px', color: '#71717a', lineHeight: 1.5 }}>
              Accepts host:port or bare hosts, comma or space separated. Empty restores the defaults.
            </div>
          </div>
        </div>
      </div>

      {/* Speedtest */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Gauge size={14} color="#facc15" />
            <span>Link Speed &amp; Bufferbloat</span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <select
              className="form-input"
              value={duration}
              onChange={e => setDuration(Number(e.target.value))}
              style={{ width: '84px', padding: '4px 7px', fontSize: '11px' }}
            >
              <option value={3}>3 s</option>
              <option value={5}>5 s</option>
              <option value={10}>10 s</option>
            </select>
            <label style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '11px', color: '#a1a1aa' }}>
              <input type="checkbox" checked={includeMerged} onChange={e => setIncludeMerged(e.target.checked)} />
              Merged pool
            </label>
            <button className="btn btn-primary btn-sm" onClick={runSpeedtest} disabled={speedRunning}>
              <Zap size={12} />
              <span>{speedRunning ? 'Measuring…' : 'Run speedtest'}</span>
            </button>
          </div>
        </div>

        {speedResults.length === 0 ? (
          <div style={{ fontSize: '11.5px', color: '#a1a1aa' }}>
            Each adapter is measured through its own interface, so the numbers reflect that link
            rather than whichever route the OS would pick. The merged run goes through the local
            SOCKS5 front-end.
          </div>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Adapter</th>
                <th>Download</th>
                <th>Upload</th>
                <th>Idle</th>
                <th>Loaded</th>
                <th>Bufferbloat</th>
              </tr>
            </thead>
            <tbody>
              {speedResults.map(r => (
                <tr key={r.adapter}>
                  <td style={{ fontWeight: 600 }}>{r.adapter}</td>
                  <td className="font-mono">{fmtMbps(r.downMbps)}</td>
                  <td className="font-mono">{fmtMbps(r.upMbps)}</td>
                  <td className="font-mono">{r.idleMs > 0 ? `${r.idleMs.toFixed(0)} ms` : '—'}</td>
                  <td className="font-mono">{r.loadedMs > 0 ? `${r.loadedMs.toFixed(0)} ms` : '—'}</td>
                  <td>
                    {r.error ? (
                      <span style={{ color: '#fb7185', fontSize: '10.5px' }}>{r.error}</span>
                    ) : (
                      <span className="status-pill" style={{ color: gradeColor(r.grade), borderColor: gradeColor(r.grade) }}>
                        {r.grade} · +{Math.round(r.bufferbloatMs)} ms
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <p style={{ margin: '8px 0 0', color: '#a1a1aa', fontSize: '11px' }}>
          These measurements describe link performance, not how the phone or carrier classifies traffic.
        </p>
      </div>

      <div className="grid-2" style={{ gap: '14px' }}>
        {/* Saved profiles */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Layers size={14} color="#10b981" />
              <span>Connection Profiles</span>
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '7px', marginBottom: '12px' }}>
            {profiles.map(p => (
              <div
                key={p.id}
                style={{
                  display: 'flex', alignItems: 'center', gap: '8px', padding: '8px 9px',
                  border: '1px solid #27272a', borderRadius: '7px', backgroundColor: '#0f0f12',
                }}
              >
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: '12px', fontWeight: 600, display: 'flex', alignItems: 'center', gap: '6px' }}>
                    {p.name}
                    {p.builtin && <span className="status-pill pass" style={{ fontSize: '9px' }}>built-in</span>}
                  </div>
                  <div style={{ fontSize: '10.5px', color: '#8b8b93', marginTop: '2px' }}>{p.description}</div>
                  <div className="font-mono" style={{ fontSize: '9.5px', color: '#71717a', marginTop: '3px' }}>
                    {(p.netMerge.routingMode || 'speed')} · {p.netMerge.scheduling || 'balanced'} · MTU {p.netMerge.mtu} · IPv6 {p.netMerge.ipv6Mode}
                  </div>
                </div>
                <button className="btn btn-secondary btn-sm" onClick={() => applySavedProfile(p.id)} disabled={busy === `apply-${p.id}`}>
                  Apply
                </button>
                {!p.builtin && (
                  <button className="btn btn-danger btn-sm" onClick={() => deleteProfile(p.id)} disabled={busy === `delete-${p.id}`}>
                    <Trash2 size={11} />
                  </button>
                )}
              </div>
            ))}
          </div>

          <div style={{ borderTop: '1px solid #232329', paddingTop: '10px' }}>
            <div className="form-label" style={{ marginBottom: '5px' }}>Save current configuration</div>
            <div style={{ display: 'flex', gap: '6px', marginBottom: '6px' }}>
              <input
                className="form-input"
                placeholder="Profile name"
                value={profileName}
                onChange={e => setProfileName(e.target.value)}
                style={{ flex: 1, padding: '5px 8px', fontSize: '11.5px' }}
              />
            </div>
            <div style={{ display: 'flex', gap: '6px' }}>
              <input
                className="form-input"
                placeholder="Description (optional)"
                value={profileDesc}
                onChange={e => setProfileDesc(e.target.value)}
                style={{ flex: 1, padding: '5px 8px', fontSize: '11.5px' }}
              />
              <button className="btn btn-emerald btn-sm" onClick={saveCurrentProfile} disabled={!profileName.trim() || busy === 'save-profile'}>
                <Save size={12} />
                <span>Save</span>
              </button>
            </div>
          </div>
        </div>

        {/* Updates and notifications */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Bell size={14} color="#fb923c" />
              <span>Updates &amp; Notifications</span>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '10px' }}>
            <button className="btn btn-secondary btn-sm" onClick={checkUpdate} disabled={checkingUpdate}>
              <DownloadCloud size={12} />
              <span>{checkingUpdate ? 'Checking…' : 'Check for updates'}</span>
            </button>
            <button className="btn btn-secondary btn-sm" onClick={testNotification} disabled={busy === 'notify'}>
              <Bell size={12} />
              <span>Test notification</span>
            </button>
          </div>

          {update && (
            <div style={{ fontSize: '11.5px', lineHeight: 1.6 }}>
              {update.error ? (
                <span style={{ color: '#fb7185' }}>Update check failed: {update.error}</span>
              ) : update.updateAvailable ? (
                <div style={{ display: 'flex', alignItems: 'flex-start', gap: '7px', color: '#facc15' }}>
                  <CheckCircle2 size={14} style={{ marginTop: '2px' }} />
                  <span>
                    NetPilot <span className="font-mono">{update.latest}</span> is available (you have{' '}
                    <span className="font-mono">{update.current}</span>).{' '}
                    <a href={update.releasesUrl} target="_blank" rel="noreferrer" style={{ color: '#38bdf8' }}>Open releases</a>
                  </span>
                </div>
              ) : (
                <span style={{ color: '#10b981' }}>
                  You are on the latest release (<span className="font-mono">{update.current}</span>).
                </span>
              )}
              <div style={{ marginTop: '4px', fontSize: '10px', color: '#71717a' }}>
                Checked {new Date(update.checkedAt).toLocaleString()}
              </div>
            </div>
          )}

          <div style={{ marginTop: '12px', fontSize: '10.5px', color: '#8b8b93', lineHeight: 1.55 }}>
            NetPilot notifies you when the merged connection starts or stops, when the active adapter
            changes, when a link flaps, and when an adapter reaches its data quota. Notifications use
            the native desktop facility and are best-effort.
          </div>
        </div>
      </div>
      </>}
    </div>
  );
};
