import { useEffect, useRef, useState, type ReactNode } from 'react';
import {
  api, type ConnectionProfile, type HotspotStatus, type InterfaceInfo,
  type MobileTunnelSettings, type NetMergeStatus, type SpeedtestResult,
} from '../api/wails';
import { TTLView } from './TTLView';
import { RefreshCw, RotateCcw, Save, Smartphone, Trash2, Zap } from 'lucide-react';

const gradeColors: Record<string, string> = {
  A: '#10b981', B: '#38bdf8', C: '#facc15', D: '#fb923c', F: '#fb7185',
};
const fmtMbps = (value: number) => value >= 1 ? `${value.toFixed(1)} Mbps` : `${Math.round(value * 1000)} Kbps`;
const ipv6Label = (value: string) => value === 'off' ? 'IPv4 only' : value === 'on' ? 'IPv6 enabled' : 'Automatic';
const schedulingLabel = (value: string) => value === 'low-latency' ? 'Prefer lower latency' : 'Balanced';

type MobileDraft = {
  mtu: string;
  ipv6Mode: string;
  scheduling: MobileTunnelSettings['scheduling'];
  probe4: string;
  probe6: string;
  adapter: string;
  autoApply: boolean;
  profile: string;
};

function makeDraft(value: Pick<MobileTunnelSettings, 'mtu' | 'ipv6Mode'> & Partial<MobileTunnelSettings>, adapter = '', autoApply = false, profile = ''): MobileDraft {
  return {
    mtu: String(value.mtu),
    ipv6Mode: value.ipv6Mode,
    scheduling: value.scheduling || 'balanced',
    probe4: (value.probeTargetsV4 ?? []).join(', '),
    probe6: (value.probeTargetsV6 ?? []).join(', '),
    adapter,
    autoApply,
    profile,
  };
}

function tunnelSettings(draft: MobileDraft): MobileTunnelSettings {
  const targets = (value: string) => value.split(/[\s,]+/).filter(Boolean);
  return {
    mtu: Number(draft.mtu),
    ipv6Mode: draft.ipv6Mode,
    scheduling: draft.scheduling,
    probeTargetsV4: targets(draft.probe4),
    probeTargetsV6: targets(draft.probe6),
  };
}

function SettingRow({ label, current, description, children }: { label: string; current: string; description: string; children: ReactNode }) {
  return <div className="mobile-setting-row">
    <div><strong>{label}</strong><span className="mobile-setting-current">{current}</span><p>{description}</p></div>
    <div className="mobile-setting-input">{children}</div>
  </div>;
}

export function ConnectivityView({ initialTTL = false }: { initialTTL?: boolean }) {
  const [ttlVisited, setTTLVisited] = useState(initialTTL);
  const [hotspot, setHotspot] = useState<HotspotStatus | null>(null);
  const [status, setStatus] = useState<NetMergeStatus | null>(null);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [profiles, setProfiles] = useState<ConnectionProfile[]>([]);
  const [draft, setDraft] = useState<MobileDraft | null>(null);
  const [draftName, setDraftName] = useState('');
  const [profileName, setProfileName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [speedResults, setSpeedResults] = useState<SpeedtestResult[]>([]);
  const [speedRunning, setSpeedRunning] = useState(false);
  const [duration, setDuration] = useState(5);
  const [includeMerged, setIncludeMerged] = useState(true);
  const request = useRef(0);

  const load = async () => {
    const version = ++request.current;
    const [hs, st, ps, links] = await Promise.allSettled([
      api.getHotspotStatus(), api.getNetMergeStatus(), api.getProfiles(), api.getInterfaces(),
    ] as const);
    if (version !== request.current) return;
    if (hs.status === 'fulfilled') setHotspot(hs.value);
    if (st.status === 'fulfilled') setStatus(st.value);
    if (ps.status === 'fulfilled') setProfiles(ps.value);
    if (links.status === 'fulfilled') setInterfaces(links.value);
    if (hs.status === 'rejected' || st.status === 'rejected') setError('Could not refresh Mobile settings.');
  };

  useEffect(() => {
    void load();
    const timer = setInterval(load, 5000);
    return () => { request.current++; clearInterval(timer); };
  }, []);

  const config = status?.config;
  const saved = config && hotspot ? makeDraft(config, hotspot.status.adapters?.[0], hotspot.status.autoApply, hotspot.status.profile) : null;
  const desired = draft ?? saved;
  const connected = interfaces.filter(link => link.isUp && !link.isVirtual && link.ip);
  const candidates = connected.filter(link => hotspot?.adapters.includes(link.name));
  const selectedConnected = !desired?.adapter || connected.some(link => link.name === desired.adapter);
  const defaultLink = connected.find(link => link.isDefault);
  const linkLabel = (name: string) => connected.find(link => link.name === name)?.displayName || name;
  const mtu = Number(desired?.mtu);
  const valid = Number.isInteger(mtu) && mtu >= 1280 && mtu <= 65535 && selectedConnected;

  const edit = <K extends keyof MobileDraft,>(key: K, value: MobileDraft[K]) => {
    if (desired) setDraft({ ...desired, [key]: value });
  };
  const clearDraft = () => { setDraft(null); setDraftName(''); };
  const change = async (operation: () => Promise<string>) => {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      setNotice(await operation());
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const runSpeedtest = async () => {
    setSpeedRunning(true);
    setError('');
    try {
      setSpeedResults(await api.runSpeedtest({ durationSec: duration, streams: 4, includeMerged, maxMB: 10 }));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSpeedRunning(false);
    }
  };

  return <div className="mobile-page">
    <header className="mobile-page-header">
      <div>
        <h1><Smartphone size={16} color="#10b981" /> Mobile &amp; Connectivity</h1>
        <p>Identify the PC link connected to your phone and tune how the NetPilot tunnel handles that connection.</p>
      </div>
      <button className="btn btn-secondary btn-sm" type="button" disabled={busy} onClick={() => { setError(''); void load(); }}>
        <RefreshCw size={12} /> Refresh
      </button>
    </header>

    {error && <p className="mobile-message mobile-error" role="alert">{error}</p>}
    {notice && <p className="mobile-message" role="status">{notice}</p>}

    <section className="card mobile-settings" aria-labelledby="mobile-settings-heading">
      <div className="card-title">
        <span id="mobile-settings-heading">Hotspot &amp; tunnel settings</span>
        <span className="rule-editor-note">{status?.isRunning ? `Connected · ${status.routingMode === 'backup' ? 'Backup' : 'Speed'}` : 'Tunnel in standby'}
          {defaultLink && ` · OS link ${linkLabel(defaultLink.name)}`}</span>
      </div>
      {config && hotspot && saved && desired ? <>
        <div className="mobile-profile-picker">
          <strong>Suggested profiles</strong>
          <p>Load a starting point into Desired values, then customize and apply when ready.</p>
          <div className="mobile-actions">
            {hotspot.profiles.map(profile => <button key={profile.id} type="button" className="btn btn-secondary btn-sm"
              disabled={busy} title={profile.description} onClick={() => {
                setDraft({ ...desired, profile: profile.id, mtu: String(profile.mtu), ipv6Mode: profile.ipv6Mode });
                setDraftName(profile.name);
              }}>{profile.name}</button>)}
          </div>
          {draftName && <p role="status">Loaded into draft · {draftName}</p>}
        </div>

        <form onSubmit={event => { event.preventDefault(); if (!draft || !valid || busy) return; void change(async () => {
          const result = await api.saveMobileSettings({
            ...tunnelSettings(desired), adapter: desired.adapter, autoApply: desired.autoApply, profile: desired.profile,
          });
          clearDraft();
          return result.note || 'Mobile settings saved.';
        }); }}>
          <fieldset disabled={busy} className="mobile-settings-fields">
            <div className="mobile-settings-labels"><h3>Current settings</h3><h3>Desired values</h3></div>
            <SettingRow label="Phone hotspot adapter" current={saved.adapter ? linkLabel(saved.adapter) : 'No adapter saved'}
              description="Pick the PC’s Wi-Fi adapter for a wireless hotspot, or the USB adapter for tethering. The selected adapter is recorded with the Mobile settings.">
              <select aria-label="Phone hotspot adapter" value={desired.adapter} onChange={event => edit('adapter', event.target.value)}>
                <option value="">No hotspot adapter selected</option>
                {!selectedConnected && <option value={desired.adapter} disabled>{desired.adapter} · disconnected</option>}
                {connected.map(link => <option key={link.name} value={link.name}>
                  {link.displayName || link.name}{candidates.includes(link) ? ' · likely phone or cellular' : link.isDefault ? ' · current internet link' : ''}
                </option>)}
              </select>
              {candidates.length === 1 && <button type="button" className="btn btn-secondary btn-sm" onClick={() => edit('adapter', candidates[0].name)}>
                Use detected adapter · {linkLabel(candidates[0].name)}
              </button>}
              {candidates.length !== 1 && defaultLink && <button type="button" className="btn btn-secondary btn-sm" onClick={() => edit('adapter', defaultLink.name)}>
                Use current connection · {linkLabel(defaultLink.name)}
              </button>}
            </SettingRow>
            <SettingRow label="Tunnel MTU" current={`${config.mtu} bytes saved${status?.runningMtu ? ` · ${status.runningMtu} running` : ''}`}
              description="Maximum packet size inside the virtual tunnel. Start with defaults on a stable link. Try 1420 if sites stall over a hotspot.">
              <input aria-label="Tunnel MTU" type="number" min={1280} max={65535} step={1} value={desired.mtu}
                onChange={event => edit('mtu', event.target.value)} />
            </SettingRow>
            <SettingRow label="Tunnel IPv6" current={ipv6Label(saved.ipv6Mode)}
              description="Automatic lets the engine use IPv6 when all eligible links support it. Enable it for capable links, or choose IPv4 only to troubleshoot IPv6 connectivity.">
              <select aria-label="Tunnel IPv6" value={desired.ipv6Mode} onChange={event => edit('ipv6Mode', event.target.value)}>
                <option value="auto">Automatic</option><option value="on">Enable IPv6</option><option value="off">IPv4 only</option>
              </select>
            </SettingRow>
            <SettingRow label="Speed-mode scheduling" current={schedulingLabel(saved.scheduling)}
              description="Balanced spreads new flows across links. Lower latency gives faster-responding links more preference. Backup mode follows its adapter order.">
              <select aria-label="Speed-mode scheduling" value={desired.scheduling} onChange={event => edit('scheduling', event.target.value === 'low-latency' ? 'low-latency' : 'balanced')}>
                <option value="balanced">Balanced</option><option value="low-latency">Prefer lower latency</option>
              </select>
            </SettingRow>
            <SettingRow label="IPv4 reachability probes" current={saved.probe4 || 'Engine defaults'}
              description="Hosts the engine uses to decide whether each link can reach IPv4. Enter hosts or host:port values, separated by commas or spaces. Blank restores defaults.">
              <input aria-label="IPv4 reachability probes" value={desired.probe4} placeholder="Engine defaults"
                onChange={event => edit('probe4', event.target.value)} />
            </SettingRow>
            <SettingRow label="IPv6 reachability probes" current={saved.probe6 || 'Engine defaults'}
              description="Checks IPv6 link health. Bracket an IPv6 address when adding a port, for example [2606:4700:4700::1111]:443.">
              <input aria-label="IPv6 reachability probes" value={desired.probe6} placeholder="Engine defaults"
                onChange={event => edit('probe6', event.target.value)} />
            </SettingRow>
            <SettingRow label="Auto-apply saved hotspot preset" current={saved.autoApply ? 'On' : 'Off'}
              description="With no active Mobile configuration, detection of USB tethering or cellular can apply the last built-in preset in Live Mode.">
              <label className="mobile-check"><input type="checkbox" checked={desired.autoApply}
                onChange={event => edit('autoApply', event.target.checked)} /> Enable auto-apply</label>
            </SettingRow>
            <div className="mobile-actions mobile-settings-actions">
              <button className="btn btn-emerald btn-sm" type="submit" disabled={!draft || !valid}><Save size={13} /> Apply changes</button>
              <button className="btn btn-secondary btn-sm" type="button" disabled={!draft} onClick={clearDraft}>Discard edits</button>
              <button className="btn btn-secondary btn-sm" type="button" onClick={() => {
                setDraft(makeDraft(hotspot.defaults, desired.adapter, false, 'hotspot-bypass'));
                setDraftName('Tunnel defaults');
              }}>Load defaults</button>
              <button className="btn btn-secondary btn-sm" type="button" disabled={!hotspot.status.enabled}
                onClick={() => void change(async () => { const result = await api.revertHotspot(); clearDraft(); return result.note || 'Previous settings restored.'; })}>
                <RotateCcw size={12} /> Restore previous
              </button>
            </div>
          </fieldset>
        </form>
        {status?.runningMtu && status.runningMtu !== config.mtu && <p className="mobile-warning">Restart Net-Merge to use the saved MTU.</p>}
        {status?.restartForMode && <p className="mobile-warning">A separate engine is running. Restart it from NetPilot to pick up saved tunnel settings. Probe and scheduling controls require the NetPilot engine.</p>}
        <details className="mobile-help">
          <summary>Limitations and suggestions</summary>
          <ul>
            <li>USB sharing drivers and cellular interfaces can be suggested automatically. A PC Wi-Fi adapter can join any access point, so select it manually when your phone shares over Wi-Fi.</li>
            <li>MTU and IPv6 apply to the whole NetPilot tunnel. Choosing a hotspot adapter records the link and does not pin all traffic to it.</li>
            <li>Try one setting at a time. If a smaller MTU or IPv4-only mode helps, compare connectivity before changing the other values. Link benchmarks use data on the selected connection.</li>
          </ul>
        </details>
      </> : <p className="rule-editor-note" role="status">Reading saved Mobile settings…</p>}
    </section>

    {config && desired && <details className="card mobile-details">
      <summary>Saved connection profiles</summary>
      <p className="rule-editor-note">Load the Mobile values from a saved configuration into the draft.</p>
      <div className="mobile-saved-profiles">
        {profiles.filter(profile => !profile.builtin).map(profile => <div key={profile.id}>
          <div><strong>{profile.name}</strong>{profile.description && <p>{profile.description}</p>}</div>
          <button type="button" className="btn btn-secondary btn-sm" disabled={busy} onClick={() => {
            setDraft(makeDraft(profile.netMerge, desired.adapter, desired.autoApply, desired.profile)); setDraftName(profile.name);
          }}>Load values</button>
          <button type="button" className="btn btn-danger btn-sm" disabled={busy} aria-label={`Delete ${profile.name}`}
            onClick={() => void change(async () => { await api.deleteProfile(profile.id); return 'Profile deleted.'; })}><Trash2 size={12} /></button>
        </div>)}
      </div>
      <form className="mobile-save-profile" onSubmit={event => {
        event.preventDefault();
        if (!valid || !profileName.trim() || busy) return;
        void change(async () => {
          await api.saveProfile({ id: '', name: profileName.trim(), netMerge: { ...config, ...tunnelSettings(desired) } });
          setProfileName('');
          return 'Profile saved. The current tunnel configuration did not change.';
        });
      }}>
        <label>Save desired values as a profile<input value={profileName} onChange={event => setProfileName(event.target.value)} placeholder="Profile name" /></label>
        <button type="submit" className="btn btn-secondary btn-sm" disabled={busy || !valid || !profileName.trim()}><Save size={12} /> Save profile</button>
      </form>
    </details>}

    <details className="card mobile-details mobile-ttl" open={initialTTL} onToggle={event => {
      if (event.currentTarget.open) setTTLVisited(true);
    }}>
      <summary>Advanced · Packet TTL &amp; Hop Limit</summary>
      <p className="rule-editor-note">These OS packet settings have separate preview, apply, and restore controls.</p>
      {ttlVisited && <TTLView />}
    </details>

      <details className="card mobile-details">
        <summary>Link speed &amp; bufferbloat</summary>
        <div className="card-title">
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
                      <span className="status-pill" style={{ color: (gradeColors[r.grade] || '#a1a1aa'), borderColor: (gradeColors[r.grade] || '#a1a1aa') }}>
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
          Benchmarks measure link performance and may use mobile data.
        </p>
      </details>


  </div>;
}
