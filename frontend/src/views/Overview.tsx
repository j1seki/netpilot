import React, { useEffect, useState, useRef } from 'react';
import { api, InterfaceInfo, ProcessInfo, ExecutionModeState, CapabilityReport } from '../api/wails';
import { Sparkline, ThroughputGraph, DonutChart } from '../components/Charts';
import { ArrowDownRight, ArrowUpRight, Cpu, Network, ShieldCheck, AlertTriangle, Zap, Server } from 'lucide-react';

export const Overview: React.FC = () => {
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [apps, setApps] = useState<ProcessInfo[]>([]);
  const [modeState, setModeState] = useState<ExecutionModeState | null>(null);
  const [caps, setCaps] = useState<CapabilityReport | null>(null);

  // Rolling 30-point history for live graph
  const [rxHistory, setRxHistory] = useState<number[]>(new Array(24).fill(0.02));
  const [txHistory, setTxHistory] = useState<number[]>(new Array(24).fill(0.05));
  const historyRef = useRef({ rx: rxHistory, tx: txHistory });
  historyRef.current = { rx: rxHistory, tx: txHistory };

  useEffect(() => {
    const load = async () => {
      const [ifaces, appList, mode, c] = await Promise.all([
        api.getInterfaces(),
        api.getApplications(),
        api.getExecutionMode(),
        api.getCapabilities(),
      ]);
      setInterfaces(ifaces);
      setApps(appList);
      setModeState(mode);
      setCaps(c);

      const curRx = ifaces.reduce((acc, i) => acc + (i.rxMbps || 0), 0);
      const curTx = ifaces.reduce((acc, i) => acc + (i.txMbps || 0), 0);

      setRxHistory(prev => [...prev.slice(1), curRx]);
      setTxHistory(prev => [...prev.slice(1), curTx]);
    };

    load();
    const interval = setInterval(load, 2000);
    return () => clearInterval(interval);
  }, []);

  const totalRx = interfaces.reduce((acc, i) => acc + (i.rxMbps || 0), 0);
  const totalTx = interfaces.reduce((acc, i) => acc + (i.txMbps || 0), 0);

  // Segments for donut chart
  const ifaceSegments = interfaces.map((iface, idx) => ({
    label: iface.name,
    value: Math.max(0.1, (iface.rxMbps || 0) + (iface.txMbps || 0)),
    color: idx === 0 ? '#38bdf8' : idx === 1 ? '#4ade80' : idx === 2 ? '#facc15' : '#c084fc',
  }));

  const activeIfaces = interfaces.filter(i => i.isUp);
  const sortedApps = [...apps].sort((a, b) => (b.socketCount || 0) - (a.socketCount || 0)).slice(0, 5);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      {/* Top Banner: Mode & Host */}
      {modeState && (
        <div style={{
          padding: '8px 12px',
          borderRadius: 'var(--radius-sm)',
          backgroundColor: modeState.currentMode === 'LIVE' ? 'rgba(16, 185, 129, 0.08)' : 'rgba(245, 158, 11, 0.08)',
          border: `1px solid ${modeState.currentMode === 'LIVE' ? 'rgba(16, 185, 129, 0.25)' : 'rgba(245, 158, 11, 0.25)'}`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontSize: '11.5px',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            {modeState.currentMode === 'LIVE' ? <ShieldCheck size={16} color="#10b981" /> : <AlertTriangle size={16} color="#f59e0b" />}
            <span style={{ fontWeight: 600, color: 'white' }}>
              OPERATIONAL STATUS: <span style={{ color: modeState.currentMode === 'LIVE' ? '#34d399' : '#fbbf24' }}>{modeState.currentMode}</span>
            </span>
            <span style={{ color: 'var(--text-muted)' }}>• Real OS processes & kernel /proc telemetry</span>
          </div>
          {caps?.environment.isVm && (
            <span className="font-mono" style={{ fontSize: '10.5px', background: 'rgba(255, 255, 255, 0.06)', padding: '2px 6px', borderRadius: '4px', color: '#cbd5e1' }}>
              Host: {caps.environment.vmType} ({caps.environment.inferredHostOs})
            </span>
          )}
        </div>
      )}

      {/* Top 4 KPI Cards */}
      <div className="metric-grid" style={{ marginBottom: 0 }}>
        <div className="metric-box">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <div>
              <span className="metric-label">Aggregated Download</span>
              <div className="metric-value" style={{ color: '#38bdf8' }}>
                {totalRx >= 1 ? totalRx.toFixed(2) : totalRx.toFixed(3)} <span style={{ fontSize: '11px', fontWeight: 500 }}>Mbps</span>
              </div>
            </div>
            <Sparkline data={rxHistory} color="#38bdf8" width={60} height={22} />
          </div>
          <span className="metric-sub" style={{ display: 'flex', alignItems: 'center', gap: '3px' }}>
            <ArrowDownRight size={11} color="#38bdf8" /> Across {interfaces.length} links
          </span>
        </div>

        <div className="metric-box">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <div>
              <span className="metric-label">Aggregated Upload</span>
              <div className="metric-value" style={{ color: '#4ade80' }}>
                {totalTx >= 1 ? totalTx.toFixed(2) : totalTx.toFixed(3)} <span style={{ fontSize: '11px', fontWeight: 500 }}>Mbps</span>
              </div>
            </div>
            <Sparkline data={txHistory} color="#4ade80" width={60} height={22} />
          </div>
          <span className="metric-sub" style={{ display: 'flex', alignItems: 'center', gap: '3px' }}>
            <ArrowUpRight size={11} color="#4ade80" /> Across {interfaces.length} links
          </span>
        </div>

        <div className="metric-box">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <div>
              <span className="metric-label">Active Links</span>
              <div className="metric-value" style={{ color: '#f8fafc' }}>
                {activeIfaces.length} <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>/ {interfaces.length} UP</span>
              </div>
            </div>
            <span className="status-pill pass" style={{ fontSize: '10px', padding: '2px 5px' }}>
              <Zap size={10} /> Online
            </span>
          </div>
          <span className="metric-sub" style={{ display: 'flex', alignItems: 'center', gap: '3px' }}>
            <Network size={11} color="#94a3b8" /> Dynamic steering active
          </span>
        </div>

        <div className="metric-box">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <div>
              <span className="metric-label">Managed Processes</span>
              <div className="metric-value" style={{ color: '#c084fc' }}>
                {apps.length}
              </div>
            </div>
            <span style={{ fontSize: '10px', background: 'rgba(192, 132, 252, 0.15)', color: '#c084fc', padding: '2px 5px', borderRadius: '4px', fontWeight: 600 }}>
              Live Proc
            </span>
          </div>
          <span className="metric-sub" style={{ display: 'flex', alignItems: 'center', gap: '3px' }}>
            <Cpu size={11} color="#c084fc" /> Process inspection active
          </span>
        </div>
      </div>

      {/* Middle Telemetry Dashboard: Live Graph + Donut */}
      <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: '10px' }}>
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Server size={14} color="#38bdf8" /> Real-time Multi-WAN Throughput Timeline
            </span>
            <span className="font-mono" style={{ fontSize: '10.5px', color: 'var(--text-muted)' }}>
              Sampling: 2.0s
            </span>
          </div>
          <ThroughputGraph rxHistory={rxHistory} txHistory={txHistory} height={100} />
        </div>

        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span>Adapter Traffic Share</span>
            <span className="font-mono" style={{ fontSize: '10.5px', color: 'var(--text-muted)' }}>Load Distribution</span>
          </div>
          <div style={{ paddingTop: '6px' }}>
            <DonutChart
              segments={ifaceSegments}
              size={95}
              thickness={14}
              centerTitle={`${activeIfaces.length}`}
              centerSub="NICs"
            />
          </div>
        </div>
      </div>

      {/* Bottom Grid: Compact Adapters Table + Top Sockets Leaderboard */}
      <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: '10px' }}>
        {/* Active Network Links */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Network size={14} color="#60a5fa" /> Network Adapters
            </span>
            <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Live Kernel Devs</span>
          </div>
          <table className="data-table">
            <thead>
              <tr>
                <th>Adapter</th>
                <th>IP Address</th>
                <th>Download (Rx)</th>
                <th>Upload (Tx)</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {interfaces.map(iface => (
                <tr key={iface.name}>
                  <td>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                      <span style={{ fontWeight: 600 }}>{iface.name}</span>
                      <span style={{ fontSize: '9.5px', color: iface.isHardware ? '#60a5fa' : '#94a3b8', background: 'rgba(255,255,255,0.05)', padding: '1px 4px', borderRadius: '3px' }}>
                        {iface.isHardware ? 'HW' : 'VIRT'}
                      </span>
                    </div>
                  </td>
                  <td className="font-mono" style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
                    {iface.ip || '—'}
                  </td>
                  <td className="font-mono" style={{ color: '#38bdf8', fontWeight: 600 }}>
                    {(iface.rxMbps || 0).toFixed(2)} <span style={{ fontSize: '10px' }}>Mbps</span>
                  </td>
                  <td className="font-mono" style={{ color: '#4ade80', fontWeight: 600 }}>
                    {(iface.txMbps || 0).toFixed(2)} <span style={{ fontSize: '10px' }}>Mbps</span>
                  </td>
                  <td>
                    <span className={`status-pill ${iface.isUp ? 'pass' : 'fail'}`} style={{ fontSize: '10px', padding: '1px 6px' }}>
                      {iface.isUp ? 'UP' : 'DOWN'}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Top Process Sockets Leaderboard */}
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Cpu size={14} color="#c084fc" /> Top Network Processes
            </span>
            <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Open Sockets</span>
          </div>
          <table className="data-table">
            <thead>
              <tr>
                <th>Process</th>
                <th>PID / Sockets</th>
                <th>Route Link</th>
              </tr>
            </thead>
            <tbody>
              {sortedApps.map(app => (
                <tr key={app.pid}>
                  <td>
                    <div style={{ fontWeight: 600, fontSize: '11.5px' }}>{app.name}</div>
                    <div className="font-mono" style={{ fontSize: '10px', color: 'var(--text-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '140px' }}>
                      {app.executable}
                    </div>
                  </td>
                  <td className="font-mono" style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
                    PID {app.pid} • <span style={{ color: '#facc15', fontWeight: 600 }}>{app.socketCount || 0} fds</span>
                  </td>
                  <td>
                    <span style={{
                      background: app.assignedNic === 'FASTEST' ? 'rgba(234, 179, 8, 0.2)' : 'rgba(59, 130, 246, 0.15)',
                      color: app.assignedNic === 'FASTEST' ? '#facc15' : '#93c5fd',
                      padding: '2px 5px',
                      borderRadius: '3px',
                      fontSize: '10.5px',
                      fontWeight: 600,
                    }}>
                      {app.assignedNic || 'eth0'}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
