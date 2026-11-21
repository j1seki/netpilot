import React, { useEffect, useState } from 'react';
import {
  api,
  CapabilityReport,
  InterfaceInfo,
  ScenarioResult,
  SuiteReport,
  LogEntry,
  UnifiedTestReport,
} from '../api/wails';
import { DonutChart } from '../components/Charts';
import { DiagnosticMCP } from '../components/DiagnosticMCP';
import {
  Play,
  RefreshCw,
  Download,
  AlertTriangle,
  Server,
  Cpu,
  Monitor,
  Zap,
  ShieldCheck,
  ShieldAlert,
} from 'lucide-react';

type SubTab =
  | 'unified'
  | 'platforms'
  | 'scenarios'
  | 'capabilities'
  | 'logs'
  | 'diagnostics'
  | 'mcp';

function ScenarioAssertions({ scenario }: { scenario: ScenarioResult }) {
  const assertions = scenario.assertions ?? [];
  if (assertions.length === 0) return <>{scenario.error || 'No assertions recorded'}</>;
  const passed = assertions.filter(assertion => assertion.passed).length;
  return <details>
    <summary>{passed}/{assertions.length} assertions passed</summary>
    {scenario.error && <p>{scenario.error}</p>}
    <ul>{assertions.map((item, index) => <li key={`${item.name}-${index}`}>
      {item.name} · {item.passed ? 'Pass' : 'Fail'} · Expected {item.expected}, got {item.actual}
      {item.detail && ` · ${item.detail}`}
    </li>)}</ul>
  </details>;
}

export const TestDebug: React.FC = () => {
  const [activeTab, setActiveTab] = useState<SubTab>('unified');

  // Subsystem states
  const [caps, setCaps] = useState<CapabilityReport | null>(null);
  const [scenarios, setScenarios] = useState<ScenarioResult[]>([]);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [diagJson, setDiagJson] = useState('');

  // One-click unified runner state
  const [unifiedReport, setUnifiedReport] = useState<UnifiedTestReport | null>(null);
  const [isRunningAll, setIsRunningAll] = useState(false);
  const [skipDisruptive, setSkipDisruptive] = useState(true);
  const [tetherAdapters, setTetherAdapters] = useState<InterfaceInfo[]>([]);
  const [tetherAdapter, setTetherAdapter] = useState('');
  const [runLabel, setRunLabel] = useState('');
  const [finishedLabel, setFinishedLabel] = useState('');
  const [exporting, setExporting] = useState(false);
  const [exportNotice, setExportNotice] = useState('');

  // Platform suite state
  const [activeSuiteReport, setActiveSuiteReport] = useState<SuiteReport | null>(null);
  const [selectedSuite, setSelectedSuite] = useState<'common' | 'linux' | 'windows' | 'macos'>('common');
  const [isRunningSingleSuite, setIsRunningSingleSuite] = useState(false);

  // Log filter states
  const [logSearch, setLogSearch] = useState('');
  const [logLevel, setLogLevel] = useState('');
  const [isLogPaused, setIsLogPaused] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');

  const loadInitial = async () => {
    try {
      const [c, l] = await Promise.all([
        api.getCapabilities(),
        api.queryLogs({}),
      ]);
      setCaps(c);
      setLogs(l);
      setErrorMsg('');
    } catch (err: any) {
      setErrorMsg(`Could not load diagnostics: ${err?.message || String(err)}`);
    }
  };

  useEffect(() => {
    loadInitial().catch((e) => console.error(e));
  }, []);

  const refreshTetherAdapters = async () => {
    try {
      const interfaces = await api.getInterfaces();
      const available = interfaces.filter(iface => iface.isUp && !iface.isVirtual);
      setTetherAdapters(available);
      setTetherAdapter(prev => available.some(iface => iface.name === prev) ? prev : '');
    } catch (err) {
      setErrorMsg(`Could not refresh adapters: ${err instanceof Error ? err.message : String(err)}`);
    }
  };

  useEffect(() => { void refreshTetherAdapters(); }, []);

  // Periodic log refresh
  useEffect(() => {
    if (isLogPaused || activeTab !== 'logs') return;
    const interval = setInterval(async () => {
      const l = await api.queryLogs({ level: logLevel, searchText: logSearch });
      setLogs(l);
    }, 2500);
    return () => clearInterval(interval);
  }, [isLogPaused, logLevel, logSearch, activeTab]);

  // One-Click Run All Execution
  const handleRunAllUnified = async () => {
    setIsRunningAll(true);
    setErrorMsg('');
    try {
      const report = await api.runAllUnified(tetherAdapter, skipDisruptive);
      setUnifiedReport(report);
      setScenarios(report.scenarios || []);
      setFinishedLabel(runLabel.trim());
    } catch (err: any) {
      setErrorMsg(`Unified test run failed: ${err?.message || String(err)}`);
    } finally {
      setIsRunningAll(false);
    }
  };

  const handleRunSingleSuite = async (suite: 'common' | 'linux' | 'windows' | 'macos') => {
    setIsRunningSingleSuite(true);
    setSelectedSuite(suite);
    setErrorMsg('');
    try {
      const rep = await api.runPlatformSuite(suite);
      setActiveSuiteReport(rep);
    } catch (err: any) {
      setErrorMsg(`Platform suite failed: ${err?.message || String(err)}`);
    } finally {
      setIsRunningSingleSuite(false);
    }
  };

  const handleGenerateDiagnostics = async () => {
    setErrorMsg('');
    try {
      const json = await api.exportDiagnosticReport(true);
      setDiagJson(json);
    } catch (err: any) {
      setErrorMsg(`Could not generate diagnostic bundle: ${err?.message || String(err)}`);
    }
  };

  const exportReport = async (kind: 'tests' | 'diagnostics', report: unknown) => {
    setExporting(true);
    setExportNotice('');
    setErrorMsg('');
    try {
      const contents = typeof report === 'string' ? JSON.parse(report) : report;
      if (await api.saveDiagnosticFile(kind, contents)) setExportNotice('JSON report saved.');
    } catch (err) {
      setErrorMsg(`Export failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setExporting(false);
    }
  };

  // Build chart segments for test report
  const chartSegments = unifiedReport ? [
    { label: 'Passed', value: unifiedReport.passed, color: '#10b981' },
    { label: 'Skipped', value: unifiedReport.skipped, color: '#f59e0b' },
    { label: 'Blocked', value: unifiedReport.blocked, color: '#8b5cf6' },
    { label: 'Failed', value: unifiedReport.failed, color: '#ef4444' },
  ].filter(s => s.value > 0) : [];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      {exportNotice && <p role="status">{exportNotice}</p>}
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

      {/* Test & Debug Compact Tab Bar */}
      <div className="tab-bar diagnostics-tabs" role="group" aria-label="Diagnostics views">
        <button className={`tab-btn ${activeTab === 'unified' ? 'active' : ''}`} aria-pressed={activeTab === 'unified'} onClick={() => setActiveTab('unified')}>
          Test suite
        </button>
        <button className={`tab-btn ${activeTab === 'platforms' ? 'active' : ''}`} aria-pressed={activeTab === 'platforms'} onClick={() => setActiveTab('platforms')}>
          Platforms
        </button>
        <button className={`tab-btn ${activeTab === 'scenarios' ? 'active' : ''}`} aria-pressed={activeTab === 'scenarios'} onClick={() => setActiveTab('scenarios')}>
          Scenarios
        </button>
        <button className={`tab-btn ${activeTab === 'capabilities' ? 'active' : ''}`} aria-pressed={activeTab === 'capabilities'} onClick={() => setActiveTab('capabilities')}>
          Capabilities
        </button>
        <button className={`tab-btn ${activeTab === 'logs' ? 'active' : ''}`} aria-pressed={activeTab === 'logs'} onClick={() => setActiveTab('logs')}>
          Logs ({logs.length})
        </button>
        <button className={`tab-btn ${activeTab === 'diagnostics' ? 'active' : ''}`} aria-pressed={activeTab === 'diagnostics'} onClick={() => { setActiveTab('diagnostics'); handleGenerateDiagnostics(); }}>
          Redacted bundle
        </button>
        <button className={`tab-btn ${activeTab === 'mcp' ? 'active' : ''}`} aria-pressed={activeTab === 'mcp'} onClick={() => setActiveTab('mcp')}>MCP</button>
      </div>

      {/* VIEW 1: ONE-CLICK UNIFIED SUITE & SCENARIO RUNNER */}
      {activeTab === 'unified' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          <section className="card diagnostics-run" aria-labelledby="diagnostics-run-title">
            <div className="card-title">
              <div className="panel-identity">
                <span className="panel-icon"><Zap size={16} aria-hidden="true" /></span>
                <div>
                  <h2 id="diagnostics-run-title">Run diagnostics</h2>
                  <p>Run network, app and system checks and export the results. Checks show when hardware or permissions are required.</p>
                </div>
              </div>
              {isRunningAll && <span className="status-pill blocked" role="status">Running tests</span>}
            </div>
            <div className="diagnostic-controls">
              <div className="diagnostic-field">
                <label htmlFor="diagnostics-run-label">Run label (optional)</label>
                <input id="diagnostics-run-label" placeholder="Baseline or phone connected" value={runLabel} maxLength={100}
                  onChange={e => setRunLabel(e.target.value)} disabled={isRunningAll} />
              </div>
              <details className="diagnostic-link-options">
                <summary>Mobile link test (optional){tetherAdapter && ` · ${tetherAdapter}`}</summary>
                <p id="diagnostics-link-help">Leave on Auto for a general run. Choose your PC’s Wi-Fi or USB adapter to test a specific hotspot connection. If live failover runs, that adapter is used as the backup link.</p>
                <div className="diagnostic-field">
                  <label htmlFor="diagnostics-tether">Hotspot adapter</label>
                  <div className="diagnostic-field-row">
                    <select id="diagnostics-tether" aria-describedby="diagnostics-link-help" value={tetherAdapter} onChange={e => setTetherAdapter(e.target.value)} disabled={isRunningAll}>
                      <option value="">Auto-detect USB or cellular adapter</option>
                      {tetherAdapters.map(iface => <option key={iface.name} value={iface.name}>{iface.displayName || iface.name}</option>)}
                    </select>
                    <button type="button" className="btn btn-secondary btn-sm" onClick={refreshTetherAdapters} disabled={isRunningAll}>
                      <RefreshCw size={13} aria-hidden="true" /> Refresh adapters
                    </button>
                  </div>
                </div>
              </details>
            </div>
            <div className="diagnostics-run-footer">
              <div className="diagnostic-actions">
                {unifiedReport && <button type="button" className="btn btn-secondary" disabled={exporting || isRunningAll}
                  onClick={() => exportReport('tests', { ...unifiedReport, label: finishedLabel })}>
                  <Download size={13} aria-hidden="true" /> Export test results
                </button>}
                <button type="button" className={`btn diagnostic-safety ${skipDisruptive ? 'diagnostic-safety-on' : 'diagnostic-safety-off'}`}
                  aria-label="Skip disruptive tests" aria-pressed={skipDisruptive}
                  onClick={() => setSkipDisruptive(value => !value)} disabled={isRunningAll}>
                  {skipDisruptive ? <ShieldCheck size={14} aria-hidden="true" /> : <ShieldAlert size={14} aria-hidden="true" />}
                  {skipDisruptive ? 'Skip disruptive tests' : 'Allow disruptive tests'}
                </button>
                <button type="button" className="btn btn-primary" onClick={handleRunAllUnified} disabled={isRunningAll}>
                  <Play size={13} aria-hidden="true" />
                  {isRunningAll ? 'Running tests…' : 'Run all tests'}
                </button>
              </div>
            </div>
            {!skipDisruptive && <p className="diagnostic-feedback diagnostic-warning">
              <AlertTriangle size={14} aria-hidden="true" />
              Includes tests that inject link failure and route rollback into test traffic.
            </p>}
          </section>

          {/* Metrics & Donut Chart Summary */}
          {unifiedReport && (
            <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 2fr', gap: '10px' }}>
              <div className="card" style={{ marginBottom: 0 }}>
                <div className="card-title">
                  <span>{finishedLabel || (unifiedReport.safeMode ? 'Results · disruptive checks skipped' : 'Full diagnostic results')}</span>
                  <span className="font-mono" style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                    {unifiedReport.durationMs.toFixed(1)} ms total
                  </span>
                </div>
                <DonutChart
                  segments={chartSegments}
                  size={95}
                  thickness={14}
                  centerTitle={`${unifiedReport.passed}/${unifiedReport.total}`}
                  centerSub="PASSED"
                />
              </div>

              <div className="card" style={{ marginBottom: 0, display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(90px, 1fr))', gap: '8px', padding: '12px' }}>
                <div style={{ background: 'rgba(16, 185, 129, 0.1)', padding: '8px 10px', borderRadius: '4px' }}>
                  <div style={{ fontSize: '10.5px', color: '#6ee7b7' }}>Passed</div>
                  <div className="font-mono" style={{ fontSize: '20px', fontWeight: 700, color: '#34d399' }}>
                    {unifiedReport.passed}
                  </div>
                </div>
                <div style={{ background: 'rgba(245, 158, 11, 0.1)', padding: '8px 10px', borderRadius: '4px' }}>
                  <div style={{ fontSize: '10.5px', color: '#fde68a' }}>Skipped</div>
                  <div className="font-mono" style={{ fontSize: '20px', fontWeight: 700, color: '#fbbf24' }}>
                    {unifiedReport.skipped}
                  </div>
                </div>
                <div style={{ background: 'rgba(139, 92, 246, 0.1)', padding: '8px 10px', borderRadius: '4px' }}>
                  <div style={{ fontSize: '10.5px', color: '#c4b5fd' }}>Blocked</div>
                  <div className="font-mono" style={{ fontSize: '20px', fontWeight: 700, color: '#a78bfa' }}>
                    {unifiedReport.blocked}
                  </div>
                </div>
                <div style={{ background: 'rgba(239, 68, 68, 0.1)', padding: '8px 10px', borderRadius: '4px' }}>
                  <div style={{ fontSize: '10.5px', color: '#fca5a5' }}>Failed</div>
                  <div className="font-mono" style={{ fontSize: '20px', fontWeight: 700, color: '#f87171' }}>
                    {unifiedReport.failed}
                  </div>
                </div>

              </div>
            </div>
          )}

          {/* Results: Platform Suites Table */}
          {unifiedReport && unifiedReport.suites && unifiedReport.suites.length > 0 && (
            <div className="card" style={{ marginBottom: 0 }}>
              <div className="card-title">
                <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  <Server size={14} color="#60a5fa" /> Platform Unit & Integration Suites
                </span>
                <span className="font-mono" style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                  {unifiedReport.suites.reduce((sum, s) => sum + s.total, 0)} test cases
                </span>
              </div>
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Test Name</th>
                    <th>Classification</th>
                    <th>Network Impact</th>
                    <th>Status</th>
                    <th>Duration</th>
                    <th>Evidence</th>
                  </tr>
                </thead>
                <tbody>
                  {unifiedReport.suites.flatMap(s => s.cases.map((c, idx) => (
                    <tr key={`${s.targetOs}_${c.name}_${idx}`}>
                      <td style={{ fontWeight: 600 }}>{c.name}</td>
                      <td>
                        <span style={{ fontSize: '10px', background: 'rgba(255,255,255,0.06)', padding: '2px 5px', borderRadius: '3px' }}>
                          {c.classification}
                        </span>
                      </td>
                      <td>
                        {c.affectsConnection ? (
                          <span style={{ fontSize: '10px', background: 'rgba(245, 158, 11, 0.15)', color: '#fbbf24', padding: '2px 6px', borderRadius: '3px', display: 'inline-flex', alignItems: 'center', gap: '4px' }} title={c.disruptionWarning}>
                            <AlertTriangle size={10} /> Disruptive
                          </span>
                        ) : (
                          <span style={{ fontSize: '10px', background: 'rgba(16, 185, 129, 0.12)', color: '#34d399', padding: '2px 6px', borderRadius: '3px', display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                            <ShieldCheck size={10} /> Safe
                          </span>
                        )}
                      </td>
                      <td>
                        <span className={`status-pill ${
                          c.status === 'PASS' ? 'pass' :
                          c.status === 'SKIPPED' || c.status === 'NOT RUN' ? 'skipped' :
                          c.status === 'BLOCKED' ? 'blocked' : 'fail'
                        }`} style={{ fontSize: '10px', padding: '1px 6px' }}>
                          {c.status}
                        </span>
                      </td>
                      <td className="font-mono" style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                        {c.durationMs.toFixed(2)} ms
                      </td>
                      <td style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
                        {c.reason || c.output || '—'}

                      </td>
                    </tr>
                  )))}
                </tbody>
              </table>
            </div>
          )}

          {/* Results: Deterministic Scenarios Table */}
          <div className="card" style={{ marginBottom: 0 }}>
            <div className="card-title">
              <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <Zap size={14} color="#facc15" /> Deterministic Scenarios
              </span>
              <span className="font-mono" style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                {scenarios.length} scenarios
              </span>
            </div>
            <table className="data-table">
              <thead>
                <tr>
                  <th>Scenario Name</th>
                  <th>Category</th>
                  <th>Network Impact</th>
                  <th>Status</th>
                  <th>Duration</th>
                  <th>Assertions / Warnings</th>
                </tr>
              </thead>
              <tbody>
                {scenarios.length === 0 && (
                  <tr>
                    <td colSpan={6} style={{ textAlign: 'center', padding: '22px', color: 'var(--text-muted)' }}>
                      No scenarios have run yet. Select <strong>Run all tests</strong>, or open <strong>Scenarios</strong> to run one at a time.
                    </td>
                  </tr>
                )}
                {scenarios.map(sc => (
                  <tr key={sc.id}>
                    <td>
                      <div style={{ fontWeight: 600 }}>{sc.name}</div>
                      <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>{sc.description}</div>
                    </td>
                    <td>
                      <span style={{ fontSize: '10px', background: 'rgba(255,255,255,0.06)', padding: '2px 5px', borderRadius: '3px' }}>
                        {sc.category}
                      </span>
                    </td>
                    <td>
                      {sc.affectsConnection ? (
                        <span
                          style={{ fontSize: '10px', background: 'rgba(245, 158, 11, 0.15)', color: '#fbbf24', padding: '2px 6px', borderRadius: '3px', display: 'inline-flex', alignItems: 'center', gap: '4px' }}
                          title={sc.disruptionWarning}
                        >
                          <AlertTriangle size={10} /> Disrupts Link
                        </span>
                      ) : (
                        <span style={{ fontSize: '10px', background: 'rgba(16, 185, 129, 0.12)', color: '#34d399', padding: '2px 6px', borderRadius: '3px', display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                          <ShieldCheck size={10} /> Safe
                        </span>
                      )}
                    </td>
                    <td>
                      <span className={`status-pill ${sc.status === 'PASS' ? 'pass' : sc.status === 'SKIPPED' ? 'skipped' : 'fail'}`} style={{ fontSize: '10px', padding: '1px 6px' }}>
                        {sc.status}
                      </span>
                    </td>
                    <td className="font-mono" style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                      {sc.durationMs.toFixed(2)} ms
                    </td>
                    <td style={{ fontSize: '11px' }}>
                      <ScenarioAssertions scenario={sc} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* VIEW 2: Platform Suites */}
      {activeTab === 'platforms' && (() => {
        const runtimeOs = (caps?.environment?.runtimeOs || '').toLowerCase();
        const isWin = runtimeOs === 'windows';
        const isLinux = runtimeOs === 'linux';
        const isMac = runtimeOs === 'darwin' || runtimeOs === 'macos';
        const hostLabel = runtimeOs ? runtimeOs.toUpperCase() : 'Host OS';
        return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '10px' }}>
            {/* Windows Suite */}
            <div className="card" style={{ marginBottom: 0 }}>
              <div className="card-title">
                <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  <Monitor size={15} color="#38bdf8" /> Windows Suite
                </span>
                <span className={`status-pill ${isWin ? 'pass' : 'skipped'}`} style={{ fontSize: '10px' }}>
                  {isWin ? 'Native Live' : `Blocked in ${hostLabel}`}
                </span>
              </div>
              <p style={{ fontSize: '11px', color: 'var(--text-secondary)', marginBottom: '10px' }}>
                Toolhelp32 processes, Wintun.dll packet driver, IP Helper route tables.
              </p>
              <button className={`btn ${isWin ? 'btn-primary' : 'btn-secondary'} btn-sm`} style={{ width: '100%' }} onClick={() => handleRunSingleSuite('windows')} disabled={isRunningSingleSuite}>
                <Play size={11} /> Run Windows Suite
              </button>
            </div>

            {/* Linux Suite */}
            <div className="card" style={{ marginBottom: 0 }}>
              <div className="card-title">
                <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  <Server size={15} color="#4ade80" /> Linux Suite
                </span>
                <span className={`status-pill ${isLinux ? 'pass' : 'skipped'}`} style={{ fontSize: '10px' }}>
                  {isLinux ? 'Native Live' : `Blocked in ${hostLabel}`}
                </span>
              </div>
              <p style={{ fontSize: '11px', color: 'var(--text-secondary)', marginBottom: '10px' }}>
                /proc process scanning, /proc/net/dev traffic counters, sysfs inspection.
              </p>
              <button className={`btn ${isLinux ? 'btn-primary' : 'btn-secondary'} btn-sm`} style={{ width: '100%' }} onClick={() => handleRunSingleSuite('linux')} disabled={isRunningSingleSuite}>
                <Play size={11} /> Run Linux Suite
              </button>
            </div>

            {/* macOS Suite */}
            <div className="card" style={{ marginBottom: 0 }}>
              <div className="card-title">
                <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  <Cpu size={15} color="#c084fc" /> macOS Suite
                </span>
                <span className={`status-pill ${isMac ? 'pass' : 'skipped'}`} style={{ fontSize: '10px' }}>
                  {isMac ? 'Native Live' : `Blocked in ${hostLabel}`}
                </span>
              </div>
              <p style={{ fontSize: '11px', color: 'var(--text-secondary)', marginBottom: '10px' }}>
                BSD routing sockets, utun devices, PF packet filter anchors.
              </p>
              <button className={`btn ${isMac ? 'btn-primary' : 'btn-secondary'} btn-sm`} style={{ width: '100%' }} onClick={() => handleRunSingleSuite('macos')} disabled={isRunningSingleSuite}>
                <Play size={11} /> Run macOS Suite
              </button>
            </div>
          </div>

          {activeSuiteReport && (
            <div className="card" style={{ marginBottom: 0 }}>
              <div className="card-title">
                <span>{selectedSuite.toUpperCase()} Suite Results</span>
                <span className="font-mono" style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                  Duration: {activeSuiteReport.durationMs.toFixed(2)} ms
                </span>
              </div>
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Test Name</th>
                    <th>Classification</th>
                    <th>Status</th>
                    <th>Duration</th>
                    <th>Reason / Details</th>
                  </tr>
                </thead>
                <tbody>
                  {activeSuiteReport.cases.map(c => (
                    <tr key={c.name}>
                      <td style={{ fontWeight: 600 }}>{c.name}</td>
                      <td>{c.classification}</td>
                      <td>
                        <span className={`status-pill ${c.status === 'PASS' ? 'pass' : c.status === 'BLOCKED' ? 'blocked' : 'fail'}`} style={{ fontSize: '10px' }}>
                          {c.status}
                        </span>
                      </td>
                      <td className="font-mono" style={{ fontSize: '11px' }}>{c.durationMs.toFixed(2)} ms</td>
                      <td style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>{c.output || c.reason || '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
        );
      })()}

      {/* VIEW 3: Scenario Explorer */}
      {activeTab === 'scenarios' && (
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span>Deterministic Scenario Catalog</span>
            <button className="btn btn-primary btn-sm" onClick={() => api.runAllScenarios().then(setScenarios)}>
              <Play size={11} /> Re-run Scenarios
            </button>
          </div>
          <table className="data-table">
            <thead>
              <tr>
                <th>Scenario</th>
                <th>Category</th>
                <th>Network Impact</th>
                <th>Status</th>
                <th>Duration</th>
                <th>Assertions</th>
              </tr>
            </thead>
            <tbody>
              {scenarios.map(sc => (
                <tr key={sc.id}>
                  <td>
                    <div style={{ fontWeight: 600 }}>{sc.name}</div>
                    <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>{sc.description}</div>
                  </td>
                  <td>{sc.category}</td>
                  <td>
                    {sc.affectsConnection ? (
                      <span style={{ fontSize: '10px', background: 'rgba(245, 158, 11, 0.15)', color: '#fbbf24', padding: '2px 5px', borderRadius: '3px' }}>
                        ⚠️ Disrupts Link
                      </span>
                    ) : (
                      <span style={{ fontSize: '10px', background: 'rgba(16, 185, 129, 0.12)', color: '#34d399', padding: '2px 5px', borderRadius: '3px' }}>
                        🛡️ Safe
                      </span>
                    )}
                  </td>
                  <td>
                    <span className={`status-pill ${sc.status === 'PASS' ? 'pass' : 'fail'}`} style={{ fontSize: '10px' }}>
                      {sc.status}
                    </span>
                  </td>
                  <td className="font-mono" style={{ fontSize: '11px' }}>{sc.durationMs.toFixed(2)} ms</td>
                  <td style={{ fontSize: '11px' }}><ScenarioAssertions scenario={sc} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* VIEW 4: Capabilities Matrix */}
      {activeTab === 'capabilities' && (
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span>Subsystem Capability Matrix</span>
            <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
              Environment: {caps?.environment.runtimeOs}/{caps?.environment.runtimeArch} ({caps?.environment.vmType})
            </span>
          </div>
          <table className="data-table">
            <thead>
              <tr>
                <th>Capability</th>
                <th>Category</th>
                <th>Status</th>
                <th>Hardware / Environment Reason</th>
              </tr>
            </thead>
            <tbody>
              {caps?.capabilities.map(cap => (
                <tr key={cap.id}>
                  <td style={{ fontWeight: 600 }}>{cap.name}</td>
                  <td>{cap.category}</td>
                  <td>
                    <span className={`status-pill ${cap.status.includes('verified') ? 'pass' : cap.status.includes('blocked') ? 'blocked' : 'skipped'}`} style={{ fontSize: '10px' }}>
                      {cap.status}
                    </span>
                  </td>
                  <td style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>{cap.reason}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* VIEW 5: Structured Logs */}
      {activeTab === 'logs' && (
        <div className="card" style={{ marginBottom: 0 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
            <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
              <input
                type="text"
                placeholder="Filter logs..."
                value={logSearch}
                onChange={e => setLogSearch(e.target.value)}
                style={{ padding: '5px 10px', fontSize: '11.5px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '4px', color: 'white' }}
              />
              <select
                value={logLevel}
                onChange={e => setLogLevel(e.target.value)}
                style={{ padding: '5px 10px', fontSize: '11.5px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '4px', color: 'white' }}
              >
                <option value="">All Levels</option>
                <option value="INFO">INFO</option>
                <option value="WARN">WARN</option>
                <option value="ERROR">ERROR</option>
              </select>
            </div>
            <button className="btn btn-secondary btn-sm" onClick={() => setIsLogPaused(!isLogPaused)}>
              {isLogPaused ? 'Resume Stream' : 'Pause Stream'}
            </button>
          </div>
          <div style={{ maxHeight: '380px', overflowY: 'auto', background: 'rgba(0,0,0,0.3)', borderRadius: '4px', padding: '8px', fontFamily: 'monospace', fontSize: '11px' }}>
            {logs.map((l, idx) => (
              <div key={idx} style={{ padding: '2px 0', borderBottom: '1px solid rgba(255,255,255,0.03)' }}>
                <span style={{ color: 'var(--text-muted)' }}>{l.timestamp ? new Date(l.timestamp).toLocaleTimeString() : ''} </span>
                <span style={{ color: l.level === 'ERROR' ? '#f87171' : l.level === 'WARN' ? '#fbbf24' : '#38bdf8', fontWeight: 600 }}>[{l.level}] </span>
                <span style={{ color: '#c084fc' }}>[{l.component}] </span>
                <span style={{ color: '#e2e8f0' }}>{l.message}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {activeTab === 'mcp' && <DiagnosticMCP />}

      {/* VIEW 6: Sanitized Diagnostic Report */}
      {activeTab === 'diagnostics' && (
        <div className="card" style={{ marginBottom: 0 }}>
          <div className="card-title">
            <span>Redacted System Diagnostic Bundle</span>
            <button className="btn btn-secondary btn-sm" disabled={!diagJson || exporting}
              onClick={() => exportReport('diagnostics', diagJson)}>
              <Download size={12} /> Export bundle
            </button>
            <button className="btn btn-secondary btn-sm" onClick={handleGenerateDiagnostics}>
              Regenerate
            </button>
          </div>
          <pre style={{ maxHeight: '380px', overflowY: 'auto', background: 'rgba(0,0,0,0.35)', padding: '10px', borderRadius: '4px', fontSize: '11px', color: '#93c5fd' }}>
            {diagJson || 'Generating bundle...'}
          </pre>
        </div>
      )}
    </div>
  );
};
