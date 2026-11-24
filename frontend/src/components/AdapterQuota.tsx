import { useCallback, useEffect, useState } from 'react';
import { api, type AdapterCapRecord, type NetMergeStatus } from '../api/wails';

export function AdapterQuota({ status }: { status: NetMergeStatus | null }) {
  const [caps, setCaps] = useState<AdapterCapRecord[]>([]);
  const [adapterChoice, setAdapterChoice] = useState('');
  const [draft, setDraft] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const refresh = useCallback(async () => setCaps(await api.getAdapterCaps()), []);

  useEffect(() => {
    const load = () => refresh().catch(err => setError(String(err)));
    void load();
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
  }, [refresh]);

  const liveNames = (status?.nics ?? []).filter(slot => slot.Present).map(slot => slot.Name);
  const adapters = [...new Set([...liveNames, ...caps.map(cap => cap.interfaceName)])];
  const adapter = adapters.includes(adapterChoice) ? adapterChoice : (adapters[0] ?? '');
  const cap = caps.find(item => item.interfaceName === adapter);
  const input = draft ?? (cap?.enabled && cap.dataQuotaMb > 0 ? String(cap.dataQuotaMb) : '');
  const quotaMb = Number(input);
  const source = status?.isRunning ? status.runningSource : status?.config.engineSource;

  const save = async (reset = false) => {
    setBusy(true);
    setError('');
    setFeedback('');
    try {
      await api.updateAdapterQuota(adapter, reset ? { reset: true } : { quotaMb });
      await refresh();
      if (!reset) setDraft(null);
      setFeedback(reset ? 'Usage counter reset.' : 'Quota saved. Allow up to five seconds for NetPilot to update the cap.');
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="card" style={{ marginBottom: 0 }}>
      <div className="card-title">Adapter data quota</div>
      <p className="view-subtitle">Counts traffic through this adapter while NetPilot monitors it. At the quota, the NetPilot engine limits relayed traffic to 0.01 Mbps. Set the quota to 0 to disable it, or reset usage at the start of a new billing period.</p>
      {source && source !== 'builtin' && <p className="view-subtitle">Quota enforcement requires the NetPilot engine.</p>}
      {adapters.length > 0 ? (
        <div className="metrics-units" style={{ flexWrap: 'wrap', alignItems: 'end' }}>
          <label>Adapter<select value={adapter} disabled={busy} onChange={event => {
            setAdapterChoice(event.target.value);
            setDraft(null);
            setFeedback('');
            setError('');
          }}>
            {adapters.map(name => <option key={name} value={name}>{name}{liveNames.includes(name) ? '' : ' (offline)'}</option>)}
          </select></label>
          <label>Quota (MiB)<input type="number" min="0" step="any" placeholder="0" value={input} disabled={busy} onChange={event => setDraft(event.target.value)} /></label>
          <button className="btn btn-secondary btn-sm" disabled={busy || !Number.isFinite(quotaMb) || quotaMb < 0 || draft === null} onClick={() => void save()}>Save quota</button>
          <span>{(cap?.dataUsedMb ?? 0).toLocaleString(undefined, { maximumFractionDigits: 1 })} MiB recorded</span>
          <button className="btn btn-secondary btn-sm" disabled={busy || !cap?.dataUsedMb} onClick={() => {
            if (window.confirm(`Reset the recorded usage for ${adapter}?`)) void save(true);
          }}>Reset usage</button>
        </div>
      ) : <p className="view-subtitle">Connect an eligible adapter to set a quota.</p>}
      {feedback && <p role="status">{feedback}</p>}
      {error && <p role="alert" style={{ color: 'var(--text-danger)' }}>{error}</p>}
    </div>
  );
}
