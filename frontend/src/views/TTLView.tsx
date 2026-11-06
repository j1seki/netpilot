import React, { useEffect, useState } from 'react';
import { api, TTLReport, TTLOutcome } from '../api/wails';
import {
  Sliders,
  ShieldCheck,
  RefreshCw,
  RotateCcw,
  Play,
  Eye,
  Zap,
} from 'lucide-react';

export const TTLView: React.FC = () => {
  const [report, setReport] = useState<TTLReport | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [ipv4Val, setIpv4Val] = useState<number>(64);
  const [ipv6Val, setIpv6Val] = useState<number>(64);
  const [policy, setPolicy] = useState<string>('none');
  const [applying, setApplying] = useState<boolean>(false);
  const [restoring, setRestoring] = useState<boolean>(false);
  const [outcome, setOutcome] = useState<TTLOutcome | null>(null);
  const [errorMsg, setErrorMsg] = useState<string>('');

  const loadData = async () => {
    try {
      const rep = await api.inspectTTL();
      setReport(rep);

      // If current values exist, populate
      const cur4 = rep.settings.find(s => s.family === 'ipv4' && s.scope === 'system-global');
      const cur6 = rep.settings.find(s => s.family === 'ipv6' && s.scope === 'system-global');
      if (cur4 && cur4.value) setIpv4Val(cur4.value);
      if (cur6 && cur6.value) setIpv6Val(cur6.value);
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleApply = async (dryRun: boolean) => {
    setApplying(true);
    setErrorMsg('');
    try {
      const out = await api.applyTTL({
        bumpIPv4: ipv4Val,
        bumpIPv6: ipv6Val,
        policy,
        ifaces: [],
        dryRun,
      });
      setOutcome(out);
      // A preview must not disturb the values the user is editing; only a real apply
      // changed the system, so only then re-read the live settings.
      if (!dryRun) {
        await loadData();
      }
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setApplying(false);
    }
  };

  const handleRestore = async () => {
    setRestoring(true);
    setErrorMsg('');
    try {
      const out = await api.restoreTTL(false);
      setOutcome(out);
      await loadData();
    } catch (err: any) {
      setErrorMsg(err?.message || String(err));
    } finally {
      setRestoring(false);
    }
  };

  if (loading && !report) {
    return (
      <div style={{ padding: '24px', textAlign: 'center', color: '#94a3b8' }}>
        Probing platform TTL and Hop Limit status...
      </div>
    );
  }

  const cur4 = report?.settings.find(s => s.family === 'ipv4' && s.scope === 'system-global');
  const cur6 = report?.settings.find(s => s.family === 'ipv6' && s.scope === 'system-global');

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
      {/* Top Banner: Capabilities & Status */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '10px' }}>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <h2 style={{ fontSize: '15px', fontWeight: 700, color: 'white' }}>
                Packet TTL &amp; Hop Limit Manager
              </h2>
              <span className="status-pill pass" style={{ fontSize: '10.5px' }}>
                {report?.capabilities.platform.toUpperCase()} • FULLY SUPPORTED
              </span>
            </div>
            <p style={{ fontSize: '11.5px', color: '#94a3b8', marginTop: '2px' }}>
              Controls outbound IPv4 Time-To-Live and IPv6 Hop Limit packet headers. Features automatic original capture, read-back verification, and single-click safe rollback.
            </p>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <button
              className="btn btn-secondary btn-sm"
              onClick={handleRestore}
              disabled={restoring}
              style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '11px' }}
            >
              <RotateCcw size={12} />
              <span>{restoring ? 'Restoring...' : 'Restore Originals'}</span>
            </button>
            <button
              className="btn btn-secondary btn-sm"
              onClick={loadData}
              style={{ padding: '6px 8px' }}
            >
              <RefreshCw size={12} />
            </button>
          </div>
        </div>
      </div>

      {/* Elevation state is reported by the backend, so the user learns whether a live
          apply can work before clicking it instead of discovering it from a failed write. */}
      {report && (
        <div style={{
          padding: '9px 12px',
          borderRadius: '7px',
          fontSize: '11.5px',
          display: 'flex',
          alignItems: 'flex-start',
          gap: '8px',
          background: report.elevated ? 'rgba(16, 185, 129, 0.1)' : 'rgba(245, 158, 11, 0.1)',
          border: `1px solid ${report.elevated ? 'rgba(16, 185, 129, 0.28)' : 'rgba(245, 158, 11, 0.3)'}`,
          color: report.elevated ? '#34d399' : '#fbbf24',
        }}>
          <ShieldCheck size={14} style={{ marginTop: '1px', flexShrink: 0 }} />
          <span>
            <strong>{report.elevated ? 'Administrator rights available.' : 'Not elevated.'}</strong>{' '}
            {report.elevationNote || (report.elevated
              ? 'Preview and live changes are both available.'
              : 'Preview (dry run) works normally; applying changes needs administrator/root privileges.')}
          </span>
        </div>
      )}

      {/* Grid: Configuration & Hotspot Presets */}
      <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: '12px' }}>
        {/* Controls Card */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Sliders size={14} color="#38bdf8" /> Configure Packet Headers
            </span>
            <span style={{ fontSize: '10.5px', color: '#64748b' }}>Range: 1 – 255</span>
          </div>

          {/* Quick Presets */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '14px', flexWrap: 'wrap' }}>
            <span style={{ fontSize: '11px', color: '#94a3b8', marginRight: '4px' }}>Presets:</span>
            <button
              onClick={() => { setIpv4Val(64); setIpv6Val(64); }}
              style={{
                fontSize: '10.5px',
                padding: '3px 8px',
                borderRadius: '4px',
                background: ipv4Val === 64 ? 'rgba(56, 189, 248, 0.2)' : 'rgba(255, 255, 255, 0.06)',
                border: `1px solid ${ipv4Val === 64 ? '#38bdf8' : 'rgba(255, 255, 255, 0.1)'}`,
                color: ipv4Val === 64 ? '#38bdf8' : '#cbd5e1',
                cursor: 'pointer',
              }}
            >
              Standard OS Default (64)
            </button>

            <button
              onClick={() => { setIpv4Val(65); setIpv6Val(65); }}
              style={{
                fontSize: '10.5px',
                padding: '3px 8px',
                borderRadius: '4px',
                background: ipv4Val === 65 ? 'rgba(16, 185, 129, 0.2)' : 'rgba(255, 255, 255, 0.06)',
                border: `1px solid ${ipv4Val === 65 ? '#10b981' : 'rgba(255, 255, 255, 0.1)'}`,
                color: ipv4Val === 65 ? '#34d399' : '#cbd5e1',
                cursor: 'pointer',
              }}
              title="Decrements by 1 hop through tethered phone so carrier sees standard on-device handset TTL 64"
            >
              ⚡ Tethering / Hotspot Optimization (65)
            </button>

            <button
              onClick={() => { setIpv4Val(128); setIpv6Val(128); }}
              style={{
                fontSize: '10.5px',
                padding: '3px 8px',
                borderRadius: '4px',
                background: ipv4Val === 128 ? 'rgba(250, 204, 21, 0.2)' : 'rgba(255, 255, 255, 0.06)',
                border: `1px solid ${ipv4Val === 128 ? '#facc15' : 'rgba(255, 255, 255, 0.1)'}`,
                color: ipv4Val === 128 ? '#facc15' : '#cbd5e1',
                cursor: 'pointer',
              }}
            >
              Windows Server / Enterprise (128)
            </button>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
            <div>
              <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '3px' }}>
                IPv4 Packet TTL
              </label>
              <input
                type="number"
                min="1"
                max="255"
                value={ipv4Val}
                onChange={e => setIpv4Val(parseInt(e.target.value) || 64)}
                className="input"
                style={{ width: '100%', fontSize: '13px', fontWeight: 700, color: '#38bdf8' }}
              />
              <span style={{ fontSize: '10px', color: '#64748b' }}>
                Current live: {cur4?.value ?? 64}
              </span>
            </div>

            <div>
              <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '3px' }}>
                IPv6 Packet Hop Limit
              </label>
              <input
                type="number"
                min="1"
                max="255"
                value={ipv6Val}
                onChange={e => setIpv6Val(parseInt(e.target.value) || 64)}
                className="input"
                style={{ width: '100%', fontSize: '13px', fontWeight: 700, color: '#facc15' }}
              />
              <span style={{ fontSize: '10px', color: '#64748b' }}>
                Current live: {cur6?.value ?? 64}
              </span>
            </div>
          </div>

          {/* Scope Selector */}
          <div style={{ marginTop: '12px' }}>
            <label style={{ fontSize: '11px', color: '#94a3b8', display: 'block', marginBottom: '3px' }}>
              Target Scope &amp; Interface Policy
            </label>
            <select
              value={policy}
              onChange={e => setPolicy(e.target.value)}
              className="input"
              style={{ width: '100%', fontSize: '11.5px' }}
            >
              <option value="none">System-Global (applies to all machine interfaces)</option>
              <option value="list">Selected Adapters Only (per-interface)</option>
              <option value="all-except">All Adapters Except Excluded</option>
            </select>
          </div>

          {errorMsg && (
            <div style={{ marginTop: '10px', padding: '6px 10px', borderRadius: '4px', background: 'rgba(239, 68, 68, 0.15)', color: '#f87171', fontSize: '11px' }}>
              {errorMsg}
            </div>
          )}

          {outcome && (
            <div style={{
              marginTop: '10px',
              padding: '7px 10px',
              borderRadius: '4px',
              background: outcome.dryRun ? 'rgba(56, 189, 248, 0.13)' : (outcome.failed ? 'rgba(239, 68, 68, 0.15)' : 'rgba(16, 185, 129, 0.15)'),
              color: outcome.dryRun ? '#7dd3fc' : (outcome.failed ? '#f87171' : '#34d399'),
              fontSize: '11px',
            }}>
              <div style={{ fontWeight: 600 }}>
                {outcome.dryRun
                  ? `Dry-run plan calculated: ${outcome.changes.length} setting(s) would change (nothing was written).`
                  : outcome.failed
                    ? `Applied ${outcome.applied ?? 0}, failed ${outcome.failed}.`
                    : `Successfully applied ${outcome.applied ?? outcome.changes.length} setting(s).`}
              </div>
              {outcome.changes.length > 0 && (
                <div style={{ marginTop: '5px', display: 'flex', flexDirection: 'column', gap: '2px' }}>
                  {outcome.changes.slice(0, 8).map((c, i) => (
                    <div key={i} className="font-mono" style={{ fontSize: '10.5px' }}>
                      {c.setting?.name || c.setting?.id}: {c.from ?? 'unreadable'} → {c.to}
                      {c.error ? ` (${c.error})` : ''}
                    </div>
                  ))}
                </div>
              )}
              {outcome.notes && outcome.notes.length > 0 && (
                <div style={{ marginTop: '5px', fontSize: '10.5px' }}>{outcome.notes.join(' ')}</div>
              )}
            </div>
          )}

          {/* Action Buttons */}
          <div style={{ display: 'flex', gap: '8px', marginTop: '14px' }}>
            <button
              className="btn btn-secondary btn-sm"
              onClick={() => handleApply(true)}
              disabled={applying}
              style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '11.5px' }}
            >
              <Eye size={12} />
              <span>Preview (Dry Run)</span>
            </button>

            <button
              className="btn btn-primary btn-sm"
              onClick={() => handleApply(false)}
              disabled={applying}
              style={{ display: 'flex', alignItems: 'center', gap: '5px', fontWeight: 600, fontSize: '11.5px' }}
            >
              <Play size={12} />
              <span>{applying ? 'Applying...' : (report?.elevated ? 'Apply TTL Changes' : 'Apply TTL Changes (needs admin)')}</span>
            </button>
          </div>
        </div>

        {/* Info & Safety Architecture Card */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <ShieldCheck size={14} color="#10b981" /> Safety Architecture
            </span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '11.5px', color: '#cbd5e1' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '6px' }}>
              <span style={{ color: '#10b981' }}>✓</span>
              <span><strong>Captured Originals:</strong> Before first write, initial values are backed up to disk so restore never relies on hardcoded guesses.</span>
            </div>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '6px' }}>
              <span style={{ color: '#10b981' }}>✓</span>
              <span><strong>Read-Back Verification:</strong> Re-reads the setting immediately after applying to confirm it took effect.</span>
            </div>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '6px' }}>
              <span style={{ color: '#10b981' }}>✓</span>
              <span><strong>Atomic Rollback:</strong> Partial transaction failures automatically roll back any modified settings.</span>
            </div>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '6px' }}>
              <span style={{ color: '#10b981' }}>✓</span>
              <span><strong>Hotspot Optimization:</strong> Sets TTL to 65 to normalize downstream tethered packets across wireless carriers.</span>
            </div>
          </div>
        </div>
      </div>

      {/* Observed Settings Table */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div className="card-title">
          <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Zap size={14} color="#38bdf8" /> Inspected Kernel Settings ({report?.settings.length || 0})
          </span>
          <span style={{ fontSize: '10.5px', color: '#64748b' }}>
            Live Platform Source: {report?.capabilities.platform}
          </span>
        </div>

        <table className="data-table">
          <thead>
            <tr>
              <th>Setting</th>
              <th>Family</th>
              <th>Scope</th>
              <th>Current Value</th>
              <th>Kernel Origin</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {report?.settings.map(s => (
              <tr key={s.id}>
                <td style={{ fontWeight: 600 }}>{s.name}</td>
                <td>
                  <span className="font-mono" style={{ fontSize: '10.5px', color: '#38bdf8' }}>
                    {s.family.toUpperCase()}
                  </span>
                </td>
                <td style={{ color: '#94a3b8', fontSize: '11px' }}>
                  {s.scope}
                </td>
                <td className="font-mono" style={{ fontSize: '12px', fontWeight: 700, color: s.value ? '#22d3ee' : '#64748b' }}>
                  {s.value ?? 'Unset'}
                </td>
                <td className="font-mono" style={{ fontSize: '10.5px', color: '#64748b' }}>
                  {s.valueOrigin || 'default'}
                </td>
                <td>
                  <span className={`status-pill ${s.writable ? 'pass' : 'blocked'}`} style={{ fontSize: '10px' }}>
                    {s.writable ? 'WRITABLE' : 'INSPECT-ONLY'}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};
