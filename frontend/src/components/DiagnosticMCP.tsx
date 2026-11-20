import { useEffect, useState } from 'react';
import { Copy, Eye, EyeOff, Play, Radio, RefreshCw, Square } from 'lucide-react';
import { api, DiagnosticMCPStatus } from '../api/wails';

export function DiagnosticMCP() {
  const [status, setStatus] = useState<DiagnosticMCPStatus | null>(null);
  const [address, setAddress] = useState('127.0.0.1');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [showToken, setShowToken] = useState(false);

  const refresh = async () => {
    try {
      const result = await api.getDiagnosticMCP();
      setStatus(result);
      setAddress(current => result.addresses.some(item => item.ip === current) ? current : '127.0.0.1');
      setError('');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  useEffect(() => { void refresh(); }, []);

  const toggle = async () => {
    setBusy(true);
    setError('');
    setNotice('');
    setShowToken(false);
    try {
      setStatus(await api.setDiagnosticMCP(!status?.running, address));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const copy = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setNotice(label + ' copied.');
    } catch {
      if (label === 'Token') setShowToken(true);
      setNotice('Select and copy the value from its field.');
    }
  };

  return <section className="card diagnostic-mcp" aria-labelledby="diagnostic-mcp-title">
    <div className="card-title">
      <div className="panel-identity">
        <span className="panel-icon"><Radio size={16} aria-hidden="true" /></span>
        <h2 id="diagnostic-mcp-title">Diagnostics MCP</h2>
      </div>
      <span className={`status-pill ${status?.running ? 'pass' : 'blocked'}`}>
        <span className={`beszel-dot ${status?.running ? 'online' : 'standby'}`} aria-hidden="true" />
        {!status ? 'Checking…' : status.running ? 'Listening' : 'Stopped'}
      </span>
    </div>
    <p className="process-network-note">Share adapter and process snapshots and run the safe diagnostic suite from an MCP client.</p>
    <div className="diagnostic-controls mcp-controls">
      <div className="diagnostic-field">
        <label htmlFor="mcp-address">Listen on</label>
        <div className="diagnostic-field-row">
          <select id="mcp-address" value={address} onChange={e => setAddress(e.target.value)} disabled={busy || status?.running}>
            {(status?.addresses ?? []).map(item => <option key={item.ip} value={item.ip}>
              {item.label} · {item.ip}{item.mode === 'lan' ? ' · HTTPS' : ''}
            </option>)}
          </select>
          <button type="button" className="btn btn-secondary btn-sm" onClick={refresh} disabled={busy}>
            <RefreshCw size={13} aria-hidden="true" /> Refresh addresses
          </button>
        </div>
      </div>
      <button type="button" className={`btn ${status?.running ? 'btn-danger' : 'btn-primary'}`} onClick={toggle} disabled={busy || !status}>
        {status?.running ? <Square size={12} aria-hidden="true" /> : <Play size={13} aria-hidden="true" />}
        {busy ? (status?.running ? 'Stopping…' : 'Starting…') : (status?.running ? 'Stop MCP server' : 'Start MCP server')}
      </button>
    </div>
    {error && <p className="diagnostic-feedback diagnostic-error" role="alert">{error}</p>}
    {notice && <p className="diagnostic-feedback" role="status">{notice}</p>}
    {status?.running && status.endpoint && status.token && <div className="mcp-connection netpilot-enter">
      <div className="diagnostic-field">
        <label htmlFor="mcp-endpoint">Streamable HTTP endpoint</label>
        <div className="diagnostic-field-row">
          <input id="mcp-endpoint" className="font-mono" readOnly value={status.endpoint} />
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => copy(status.endpoint ?? '', 'Endpoint')}>
            <Copy size={13} aria-hidden="true" /> Copy endpoint
          </button>
        </div>
      </div>
      <div className="diagnostic-field">
        <label htmlFor="mcp-token">Bearer token</label>
        <div className="diagnostic-field-row">
          <input id="mcp-token" className="font-mono" type={showToken ? 'text' : 'password'} readOnly value={status.token} spellCheck={false} />
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setShowToken(!showToken)}
            aria-label={showToken ? 'Hide token' : 'Show token'} title={showToken ? 'Hide token' : 'Show token'}>
            {showToken ? <EyeOff size={15} aria-hidden="true" /> : <Eye size={15} aria-hidden="true" />}
          </button>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => copy(status.token ?? '', 'Token')}>
            <Copy size={13} aria-hidden="true" /> Copy token
          </button>
        </div>
      </div>
      {status.certificate && <details>
        <summary>LAN certificate</summary>
        <p className="process-network-note">Trust this certificate in the MCP client for this session.</p>
        <textarea className="font-mono" aria-label="LAN certificate" rows={4} readOnly value={status.certificate} />
        <button type="button" className="btn btn-secondary btn-sm" onClick={() => copy(status.certificate ?? '', 'Certificate')}>Copy certificate</button>
      </details>}
    </div>}
  </section>;
}
