import React, { useEffect, useState } from 'react';
import { api, InterfaceInfo, ProcessInfo, MonthlyUsageRecord, MonthlySummary } from '../api/wails';
import { DonutChart, MonthlyBarChart } from '../components/Charts';
import { ArrowDown, ArrowUp, Calendar, Download, FileSpreadsheet } from 'lucide-react';

export const Analytics: React.FC = () => {
  const currentYearMonth = new Date().toISOString().slice(0, 7);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [apps, setApps] = useState<ProcessInfo[]>([]);
  const [monthlyRecords, setMonthlyRecords] = useState<MonthlyUsageRecord[]>([]);
  const [monthlySummaries, setMonthlySummaries] = useState<MonthlySummary[]>([]);
  const [selectedMonth, setSelectedMonth] = useState<string>(currentYearMonth);
  const [isExporting, setIsExporting] = useState(false);

  const loadData = async () => {
    const [ifaces, appList, monthly] = await Promise.all([
      api.getInterfaces(),
      api.getApplications(),
      api.getMonthlyHistory(selectedMonth),
    ]);
    setInterfaces(ifaces);
    setApps(appList);
    setMonthlyRecords(monthly.records);
    setMonthlySummaries(monthly.summaries);
  };

  useEffect(() => {
    loadData().catch((e) => console.error(e));
    const interval = setInterval(() => loadData().catch((e) => console.error(e)), 2500);
    return () => clearInterval(interval);
  }, [selectedMonth]);

  const totalMonthBytes = monthlyRecords.reduce((acc, r) => acc + r.bytesIn + r.bytesOut, 0);
  const availableMonths = Array.from(
    new Set([currentYearMonth, ...monthlySummaries.map(s => s.yearMonth).filter(Boolean)])
  ).sort().reverse();

  const handleExportCsv = async () => {
    setIsExporting(true);
    try {
      const csvText = await api.exportMonthlyCsvText(selectedMonth);
      const blob = new Blob([csvText], { type: 'text/csv;charset=utf-8;' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `netpilot-monthly-${selectedMonth || 'all'}.csv`);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } finally {
      setIsExporting(false);
    }
  };

  const currentSummary = monthlySummaries.find(s => s.yearMonth === selectedMonth) || {
    yearMonth: selectedMonth,
    totalRxMb: monthlyRecords.reduce((acc, r) => acc + r.rxMb, 0),
    totalTxMb: monthlyRecords.reduce((acc, r) => acc + r.txMb, 0),
    totalMb: monthlyRecords.reduce((acc, r) => acc + r.totalMb, 0),
  };

  // Monthly bar chart data
  const barChartData = monthlySummaries.map(s => ({
    month: s.yearMonth,
    rxMb: s.totalRxMb,
    txMb: s.totalTxMb,
  }));

  // App bandwidth segments for donut chart
  const appSegments = apps.slice(0, 5).map((app, idx) => ({
    label: app.name,
    value: Math.max(1, app.socketCount || 1),
    color: idx === 0 ? '#38bdf8' : idx === 1 ? '#4ade80' : idx === 2 ? '#facc15' : idx === 3 ? '#c084fc' : '#f87171',
  }));

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      {/* Top 3 Compact KPIs */}
      <div className="metric-grid" style={{ marginBottom: 0 }}>
        <div className="metric-box">
          <span className="metric-label">Recorded Volume ({selectedMonth === 'all' ? 'All Time' : selectedMonth})</span>
          <div className="metric-value" style={{ color: '#60a5fa' }}>
            {(totalMonthBytes / (1024 * 1024)).toFixed(2)} <span style={{ fontSize: '11px' }}>MB</span>
          </div>
          <span className="metric-sub">Real interface byte counters</span>
        </div>

        <div className="metric-box">
          <span className="metric-label">Primary Link Load</span>
          <div className="metric-value" style={{ color: '#38bdf8' }}>
            {interfaces[0] ? (interfaces[0].rxMbps + interfaces[0].txMbps).toFixed(2) : '0.00'} <span style={{ fontSize: '11px' }}>Mbps</span>
          </div>
          <span className="metric-sub">{interfaces[0]?.name || 'No adapter'} {interfaces[0]?.ip ? `(${interfaces[0].ip})` : ''}</span>
        </div>

        <div className="metric-box">
          <span className="metric-label">Secondary Link Load</span>
          <div className="metric-value" style={{ color: '#4ade80' }}>
            {interfaces[1] ? (interfaces[1].rxMbps + interfaces[1].txMbps).toFixed(2) : '0.00'} <span style={{ fontSize: '11px' }}>Mbps</span>
          </div>
          <span className="metric-sub">{interfaces[1]?.name || 'Single adapter'} {interfaces[1]?.ip ? `(${interfaces[1].ip})` : ''}</span>
        </div>
      </div>

      {/* Visual Graphs Row: Monthly Bar Chart + Process Share Donut */}
      <div style={{ display: 'grid', gridTemplateColumns: '1.8fr 1.2fr', gap: '10px' }}>
        <MonthlyBarChart data={barChartData} height={85} />

        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span>Process Socket Share</span>
            <span className="font-mono" style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Top Active</span>
          </div>
          <DonutChart
            segments={appSegments}
            size={85}
            thickness={12}
            centerTitle={`${apps.length}`}
            centerSub="Apps"
          />
        </div>
      </div>

      {/* Monthly History & CSV Export Section */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px' }}>
          <div>
            <div style={{ fontSize: '13.5px', fontWeight: 600, display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Calendar size={15} color="#60a5fa" />
              Monthly Accounting History
            </div>
            <p style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
              Historical usage per month, network adapter, and application.
            </p>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <select
              value={selectedMonth}
              onChange={e => setSelectedMonth(e.target.value)}
              style={{
                padding: '5px 8px',
                backgroundColor: 'var(--bg-tertiary)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                color: 'white',
                fontSize: '11.5px',
              }}
            >
              {availableMonths.map(m => (
                <option key={m} value={m}>
                  {m === currentYearMonth ? `${m} (Current)` : m}
                </option>
              ))}
              <option value="all">All Months</option>
            </select>

            <button
              className="btn btn-primary btn-sm"
              onClick={handleExportCsv}
              disabled={isExporting}
              style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '11.5px', padding: '5px 10px' }}
            >
              <Download size={12} />
              {isExporting ? 'Exporting...' : 'Export CSV'}
            </button>
          </div>
        </div>

        {/* Monthly Summary Strip */}
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(4, 1fr)',
          gap: '8px',
          marginBottom: '10px',
        }}>
          <div style={{ backgroundColor: 'rgba(0,0,0,0.2)', padding: '6px 10px', borderRadius: '4px' }}>
            <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Month Download (Rx)</div>
            <div className="font-mono" style={{ fontSize: '15px', fontWeight: 700, color: '#38bdf8' }}>
              {currentSummary.totalRxMb.toFixed(1)} <span style={{ fontSize: '10px' }}>MB</span>
            </div>
          </div>
          <div style={{ backgroundColor: 'rgba(0,0,0,0.2)', padding: '6px 10px', borderRadius: '4px' }}>
            <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Month Upload (Tx)</div>
            <div className="font-mono" style={{ fontSize: '15px', fontWeight: 700, color: '#4ade80' }}>
              {currentSummary.totalTxMb.toFixed(1)} <span style={{ fontSize: '10px' }}>MB</span>
            </div>
          </div>
          <div style={{ backgroundColor: 'rgba(0,0,0,0.2)', padding: '6px 10px', borderRadius: '4px' }}>
            <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Total Monthly Bandwidth</div>
            <div className="font-mono" style={{ fontSize: '15px', fontWeight: 700, color: '#facc15' }}>
              {currentSummary.totalMb.toFixed(1)} <span style={{ fontSize: '10px' }}>MB</span>
            </div>
          </div>
          <div style={{ backgroundColor: 'rgba(0,0,0,0.2)', padding: '6px 10px', borderRadius: '4px' }}>
            <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>CSV Export</div>
            <div style={{ fontSize: '11.5px', fontWeight: 600, color: '#60a5fa', marginTop: '2px', display: 'flex', alignItems: 'center', gap: '4px' }}>
              <FileSpreadsheet size={12} /> RFC-4180
            </div>
          </div>
        </div>

        {/* Monthly History Table */}
        <table className="data-table">
          <thead>
            <tr>
              <th>Month</th>
              <th>Adapter</th>
              <th>Application</th>
              <th>Download (Rx)</th>
              <th>Upload (Tx)</th>
              <th>Total MB</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {monthlyRecords.length === 0 ? (
              <tr>
                <td colSpan={7} style={{ textAlign: 'center', padding: '16px', color: 'var(--text-muted)' }}>
                  No historical entries recorded.
                </td>
              </tr>
            ) : (
              monthlyRecords.map(rec => (
                <tr key={rec.id}>
                  <td className="font-mono" style={{ fontWeight: 600, color: '#93c5fd' }}>{rec.yearMonth}</td>
                  <td>
                    <span style={{ background: 'rgba(59, 130, 246, 0.15)', color: '#93c5fd', padding: '1px 6px', borderRadius: '3px', fontSize: '11px' }}>
                      {rec.interfaceName}
                    </span>
                  </td>
                  <td style={{ fontWeight: 600 }}>{rec.appName || 'All Process Traffic'}</td>
                  <td className="font-mono" style={{ color: '#38bdf8' }}>
                    <ArrowDown size={10} /> {rec.rxMb.toFixed(1)} MB
                  </td>
                  <td className="font-mono" style={{ color: '#4ade80' }}>
                    <ArrowUp size={10} /> {rec.txMb.toFixed(1)} MB
                  </td>
                  <td className="font-mono" style={{ fontWeight: 700 }}>
                    {rec.totalMb.toFixed(1)} MB
                  </td>
                  <td className="font-mono" style={{ fontSize: '10.5px', color: 'var(--text-muted)' }}>
                    {rec.updatedAt ? new Date(rec.updatedAt).toLocaleTimeString() : 'Recent'}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};
