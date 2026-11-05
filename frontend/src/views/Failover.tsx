import React, { useEffect, useState } from 'react';
import { api, InterfaceInfo } from '../api/wails';
import { CheckCircle2, AlertTriangle, XCircle, ArrowRightLeft, Zap } from 'lucide-react';

export const Failover: React.FC = () => {
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);

  const loadData = async () => {
    const list = await api.getInterfaces();
    setInterfaces(list);
  };

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 2000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div>
      <div className="card" style={{ marginBottom: '20px' }}>
        <div className="card-title">
          <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <ArrowRightLeft size={16} color="#38bdf8" /> Failover State Machine Configuration
          </span>
          <span className="status-pill pass">Active & Monitoring</span>
        </div>
        <p style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '16px' }}>
          NetPilot continuously pings probe gateways on each interface. Anti-flapping hysteresis prevents rapid oscillations by requiring consecutive successful probes before restoring a failed link.
        </p>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '12px' }}>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: 'var(--radius-sm)' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Failure Threshold</div>
            <div style={{ fontSize: '15px', fontWeight: 600, marginTop: '2px' }}>3 consecutive drops</div>
          </div>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: 'var(--radius-sm)' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Recovery Hysteresis</div>
            <div style={{ fontSize: '15px', fontWeight: 600, marginTop: '2px' }}>2 consecutive passes</div>
          </div>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: 'var(--radius-sm)' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Degraded RTT Cutoff</div>
            <div style={{ fontSize: '15px', fontWeight: 600, marginTop: '2px' }}>&gt; 400 ms latency</div>
          </div>
          <div style={{ padding: '12px', background: 'rgba(0,0,0,0.2)', borderRadius: 'var(--radius-sm)' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Anti-Flap Freeze</div>
            <div style={{ fontSize: '15px', fontWeight: 600, marginTop: '2px' }}>5 transitions / 60s</div>
          </div>
        </div>
      </div>

      {/* Interface Health Table */}
      <div className="card">
        <div className="card-title">
          <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Zap size={16} color="#fbbf24" /> Adapter Health Status & RTT Probing
          </span>
        </div>
        <table className="data-table">
          <thead>
            <tr>
              <th>Interface</th>
              <th>Health Condition</th>
              <th>Last Probe RTT</th>
              <th>Failures / Successes</th>
              <th>Simulation Actions</th>
            </tr>
          </thead>
          <tbody>
            {interfaces.map(i => (
              <tr key={i.name}>
                <td style={{ fontWeight: 600 }}>{i.name}</td>
                <td>
                  <span className={`status-pill ${i.linkState === 'UP' ? 'pass' : i.linkState === 'DEGRADED' ? 'skipped' : 'fail'}`}>
                    {i.linkState === 'UP' ? <CheckCircle2 size={11} /> : i.linkState === 'DEGRADED' ? <AlertTriangle size={11} /> : <XCircle size={11} />}
                    {i.linkState}
                  </span>
                </td>
                <td className="font-mono">
                  {i.linkState === 'UP' ? '18.4 ms' : i.linkState === 'DEGRADED' ? '415.0 ms' : 'Timeout'}
                </td>
                <td className="font-mono" style={{ fontSize: '12px', color: 'var(--text-secondary)' }}>
                  0 fails / 14 passes
                </td>
                <td>
                  <div style={{ display: 'flex', gap: '8px' }}>
                    <button
                      className="btn btn-danger btn-sm"
                      onClick={() => alert(`Simulating drop on ${i.name}`)}
                    >
                      Simulate Drop
                    </button>
                    <button
                      className="btn btn-secondary btn-sm"
                      onClick={() => alert(`Restoring link on ${i.name}`)}
                    >
                      Restore Link
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};
