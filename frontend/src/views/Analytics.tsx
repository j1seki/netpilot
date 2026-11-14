import React, { useEffect, useState } from 'react';
import {
  api,
  InterfaceInfo,
  ProcessInfo,
  MonthlySummary,
  NetMergeStatus,
  UsageHistoryResponse,
  UsagePeriod,
} from '../api/wails';
import { DonutChart, MonthlyBarChart } from '../components/Charts';
import { ArrowDown, ArrowUp, Calendar, Download, FileSpreadsheet, RotateCcw, Trash2 } from 'lucide-react';

const PERIODS: { id: UsagePeriod; label: string; hint: string }[] = [
  { id: 'today', label: 'Today', hint: 'Today (00:00 → now)' },
  { id: 'week', label: 'Last 7 Days', hint: 'Rolling last 7 days' },
  { id: 'month', label: 'This Month', hint: 'Current calendar month' },
  { id: 'all', label: 'All Months', hint: 'Every recorded month' },
];

function emptyUsage(period: UsagePeriod): UsageHistoryResponse {
  return {
    period,
    from: '',
    to: '',
    records: [],
    summaries: [],
    totalRxMb: 0,
    totalTxMb: 0,
    totalMb: 0,
  };
}

function csvEscape(value: unknown): string {
  const text = String(value ?? '');
  return /[",\n\r]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
}

export const Analytics: React.FC = () => {
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [apps, setApps] = useState<ProcessInfo[]>([]);
  const [usage, setUsage] = useState<UsageHistoryResponse>(emptyUsage('month'));
  const [monthlySummaries, setMonthlySummaries] = useState<MonthlySummary[]>([]);
  const [nmStatus, setNmStatus] = useState<NetMergeStatus | null>(null);
  const [period, setPeriod] = useState<UsagePeriod>('month');
  const [isExporting, setIsExporting] = useState(false);
  const [resetBusy, setResetBusy] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');

  useEffect(() => {
    let cancelled = false;

    const loadData = async () => {
      const [ifacesRes, appsRes, usageRes, monthlyRes, nmRes] = await Promise.allSettled([
        api.getInterfaces(),
        api.getApplications(),
        api.getUsageHistory(period),
        api.getMonthlyHistory('all'),
        api.getNetMergeStatus(),
      ]);

      if (cancelled) return;
      const problems: string[] = [];

      if (ifacesRes.status === 'fulfilled') setInterfaces(ifacesRes.value);
      else problems.push(`adapters: ${String(ifacesRes.reason)}`);

      if (appsRes.status === 'fulfilled') setApps(appsRes.value);
      else problems.push(`processes: ${String(appsRes.reason)}`);

      if (usageRes.status === 'fulfilled') setUsage(usageRes.value);
      else problems.push(`usage: ${String(usageRes.reason)}`);

      if (monthlyRes.status === 'fulfilled') setMonthlySummaries(monthlyRes.value.summaries || []);
      else problems.push(`monthly chart: ${String(monthlyRes.reason)}`);

      if (nmRes.status === 'fulfilled') setNmStatus(nmRes.value);
      else problems.push(`net-merge: ${String(nmRes.reason)}`);

      setErrorMsg(problems.length > 0 ? `Some analytics data could not be loaded — ${problems.join('; ')}` : '');
    };

    loadData().catch((e) => console.error(e));
    const interval = setInterval(() => loadData().catch((e) => console.error(e)), 3000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [period]);

  const isMergeRunning = nmStatus?.isRunning ?? false;
  const defaultAdapter = interfaces.find(i => i.isDefault);
  const busiestAdapter = [...interfaces].sort(
    (a, b) => (b.rxMbps + b.txMbps) - (a.rxMbps + a.txMbps)
  )[0];
  const mergedLoad = interfaces.reduce((acc, i) => acc + (i.rxMbps || 0) + (i.txMbps || 0), 0);
  const primaryLoad = isMergeRunning
    ? mergedLoad
    : (defaultAdapter ? (defaultAdapter.rxMbps || 0) + (defaultAdapter.txMbps || 0) : 0);
  const busiestLoad = busiestAdapter ? (busiestAdapter.rxMbps || 0) + (busiestAdapter.txMbps || 0) : 0;
  const periodMeta = PERIODS.find(p => p.id === period) || PERIODS[2];

  const barChartData = monthlySummaries.map(s => ({
    month: s.yearMonth,
    rxMb: s.totalRxMb,
    txMb: s.totalTxMb,
  }));

  const appSegments = apps.slice(0, 5).map((app, idx) => ({
    label: app.name,
    value: Math.max(1, app.socketCount || 1),
    color: idx === 0 ? '#38bdf8' : idx === 1 ? '#4ade80' : idx === 2 ? '#facc15' : idx === 3 ? '#c084fc' : '#f87171',
  }));

  const handleExportCsv = () => {
    setIsExporting(true);
    try {
      const header = [
        'Period', 'Interface', 'Application', 'BytesIn', 'BytesOut', 'RxMB', 'TxMB', 'TotalMB', 'LastUpdated',
      ];
      const rows = usage.records.map(r => [
        r.period, r.interfaceName, r.appName || 'All Process Traffic',
        r.bytesIn, r.bytesOut, r.rxMb.toFixed(2), r.txMb.toFixed(2), r.totalMb.toFixed(2), r.updatedAt,
      ]);
      const csvText = [header, ...rows].map(row => row.map(csvEscape).join(',')).join('\n');
      const blob = new Blob([csvText], { type: 'text/csv;charset=utf-8;' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', `netpilot-usage-${period}-${new Date().toISOString().slice(0, 10)}.csv`);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } finally {
      setIsExporting(false);
    }
  };

  const handleResetUsage = async (scope: 'adapter' | 'daily' | 'monthly' | 'all') => {
    const prompts: Record<'adapter' | 'daily' | 'monthly' | 'all', string> = {
      adapter: 'Reset the data-used counters for all adapters? History is kept.',
      daily: 'Delete all daily usage history? Monthly totals are kept.',
      monthly: 'Delete all monthly usage history? Daily totals are kept.',
      all: 'Delete all recorded usage counters and history? This cannot be undone.',
    };
    if (!window.confirm(prompts[scope])) return;
    setResetBusy(true);
    try {
      const result = await api.resetDataUsage(scope, '');
      setErrorMsg('');
      alert(`Usage reset complete — ${result.adapterCounters} adapter counters, ${result.dailyRows} daily rows, ${result.monthlyRows} monthly rows updated/removed.`);
    } catch (err: any) {
      setErrorMsg(`Could not reset usage: ${err?.message || String(err)}`);
    } finally {
      setResetBusy(false);
    }
  };

  const emptyHint = period === 'today'
    ? 'No traffic has been recorded today yet. NetPilot writes a daily bucket whenever interface counters increase.'
    : period === 'week'
      ? 'No daily samples for the last 7 days yet. Keep NetPilot running while traffic flows and this fills in automatically.'
      : period === 'month'
        ? 'No usage has been recorded for the current month yet.'
        : 'No historical entries recorded.';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      {errorMsg && (
        <div style={{
          padding: '9px 12px',
          borderRadius: '7px',
          background: 'rgba(244, 63, 94, 0.12)',
          border: '1px solid rgba(244, 63, 94, 0.3)',
          color: '#fb7185',
          fontSize: '11.5px',
        }}>
          {errorMsg}
        </div>
      )}

      {/* Top 3 Compact KPIs */}
      <div className="metric-grid" style={{ marginBottom: 0 }}>
        <div className="metric-box">
          <span className="metric-label">
            {isMergeRunning ? 'Merged Throughput (net-merge)' : 'OS Default Link Load'}
          </span>
          <div className="metric-value" style={{ color: isMergeRunning ? '#10b981' : '#60a5fa' }}>
            {primaryLoad.toFixed(2)} <span style={{ fontSize: '11px' }}>Mbps</span>
          </div>
          <span className="metric-sub">
            {isMergeRunning
              ? `${interfaces.length} adapter${interfaces.length === 1 ? '' : 's'} in the merged pool`
              : defaultAdapter
                ? `${defaultAdapter.name} · OS default route${defaultAdapter.isVirtual ? ' · VPN/virtual' : ''}`
                : 'No OS default route detected'}
          </span>
        </div>

        <div className="metric-box">
          <span className="metric-label">Busiest Adapter</span>
          <div className="metric-value" style={{ color: '#38bdf8' }}>
            {busiestLoad.toFixed(2)} <span style={{ fontSize: '11px' }}>Mbps</span>
          </div>
          <span className="metric-sub">
            {busiestAdapter
              ? `${busiestAdapter.name}${busiestAdapter.isDefault ? ' · OS default' : ''}${busiestAdapter.isVirtual ? ' · VPN' : ''}`
              : 'No adapter data'}
          </span>
        </div>

        <div className="metric-box">
          <span className="metric-label">Recorded Volume ({periodMeta.label})</span>
          <div className="metric-value" style={{ color: '#facc15' }}>
            {usage.totalMb.toFixed(2)} <span style={{ fontSize: '11px' }}>MB</span>
          </div>
          <span className="metric-sub">
            {usage.from && usage.to ? `${usage.from} → ${usage.to}` : 'Real interface byte counters'}
          </span>
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

      {/* Usage History & CSV Export Section */}
      <div className="card" style={{ marginBottom: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px', gap: '12px', flexWrap: 'wrap' }}>
          <div>
            <div style={{ fontSize: '13.5px', fontWeight: 600, display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Calendar size={15} color="#60a5fa" />
              Usage History
            </div>
            <p style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
              Real traffic per period, adapter and application. {periodMeta.hint}.
            </p>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
            <div className="tab-bar" role="group" aria-label="Usage time range">
              {PERIODS.map(p => (
                <button
                  key={p.id}
                  type="button"
                  className={`tab-btn ${period === p.id ? 'active' : ''}`}
                  onClick={() => setPeriod(p.id)}
                  title={p.hint}
                >
                  {p.label}
                </button>
              ))}
            </div>

            <button
              className="btn btn-primary btn-sm"
              onClick={handleExportCsv}
              disabled={isExporting || usage.records.length === 0}
              style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '11.5px', padding: '5px 10px' }}
              title={usage.records.length === 0 ? 'No rows to export for this period' : 'Export the rows currently shown'}
            >
              <Download size={12} />
              {isExporting ? 'Exporting...' : 'Export CSV'}
            </button>

            <button
              className="btn btn-secondary btn-sm"
              onClick={() => handleResetUsage('adapter')}
              disabled={resetBusy}
              style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '11.5px', padding: '5px 10px' }}
              title="Reset adapter data-used counters without deleting history"
            >
              <RotateCcw size={12} />
              Reset Counters
            </button>

            <button
              className="btn btn-secondary btn-sm"
              onClick={() => handleResetUsage('all')}
              disabled={resetBusy}
              style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '11.5px', padding: '5px 10px', color: '#fb7185' }}
              title="Delete all recorded usage counters and history"
            >
              <Trash2 size={12} />
              Clear History
            </button>
          </div>
        </div>

        {/* Summary Strip */}
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(4, 1fr)',
          gap: '8px',
          marginBottom: '10px',
        }}>
          <div style={{ backgroundColor: 'rgba(0,0,0,0.2)', padding: '6px 10px', borderRadius: '4px' }}>
            <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Download (Rx)</div>
            <div className="font-mono" style={{ fontSize: '15px', fontWeight: 700, color: '#38bdf8' }}>
              {usage.totalRxMb.toFixed(1)} <span style={{ fontSize: '10px' }}>MB</span>
            </div>
          </div>
          <div style={{ backgroundColor: 'rgba(0,0,0,0.2)', padding: '6px 10px', borderRadius: '4px' }}>
            <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Upload (Tx)</div>
            <div className="font-mono" style={{ fontSize: '15px', fontWeight: 700, color: '#4ade80' }}>
              {usage.totalTxMb.toFixed(1)} <span style={{ fontSize: '10px' }}>MB</span>
            </div>
          </div>
          <div style={{ backgroundColor: 'rgba(0,0,0,0.2)', padding: '6px 10px', borderRadius: '4px' }}>
            <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Total Bandwidth</div>
            <div className="font-mono" style={{ fontSize: '15px', fontWeight: 700, color: '#facc15' }}>
              {usage.totalMb.toFixed(1)} <span style={{ fontSize: '10px' }}>MB</span>
            </div>
          </div>
          <div style={{ backgroundColor: 'rgba(0,0,0,0.2)', padding: '6px 10px', borderRadius: '4px' }}>
            <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Rows / Format</div>
            <div style={{ fontSize: '11.5px', fontWeight: 600, color: '#60a5fa', marginTop: '2px', display: 'flex', alignItems: 'center', gap: '4px' }}>
              <FileSpreadsheet size={12} /> {usage.records.length} · CSV
            </div>
          </div>
        </div>

        {/* Usage Table */}
        <table className="data-table">
          <thead>
            <tr>
              <th>{period === 'month' || period === 'all' ? 'Month' : 'Day'}</th>
              <th>Adapter</th>
              <th>Application</th>
              <th>Download (Rx)</th>
              <th>Upload (Tx)</th>
              <th>Total MB</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {usage.records.length === 0 ? (
              <tr>
                <td colSpan={7} style={{ textAlign: 'center', padding: '16px', color: 'var(--text-muted)' }}>
                  {emptyHint}
                </td>
              </tr>
            ) : (
              usage.records.map((rec, idx) => (
                <tr key={`${rec.period}-${rec.interfaceName}-${rec.appName}-${idx}`}>
                  <td className="font-mono" style={{ fontWeight: 600, color: '#93c5fd' }}>{rec.period}</td>
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
                    {rec.updatedAt ? new Date(rec.updatedAt).toLocaleString() : 'Recent'}
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
