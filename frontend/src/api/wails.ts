// Centralized frontend API client for Wails 3 backend services and types.

export interface BrandingInfo {
  name: string;
  id: string;
  version: string;
  description: string;
  vendor: string;
  copyright: string;
  supportUrl: string;
  windowTitle: string;
}

export interface EnvironmentDetails {
  runtimeOs: string;
  runtimeArch: string;
  kernelRelease: string;
  isVm: boolean;
  vmType: string;
  inferredHostOs: string;
  isElevated: boolean;
  hasSudo: boolean;
  physicalNicCount: number;
  virtualNicCount: number;
}

export interface CapabilityVerdict {
  id: string;
  name: string;
  category: string;
  status: string;
  reason: string;
}

export interface CapabilityReport {
  environment: EnvironmentDetails;
  capabilities: CapabilityVerdict[];
}

export interface ExecutionModeState {
  currentMode: 'LIVE' | 'DRY_RUN';
  description: string;
  allowsSystemMod: boolean;
  requiresRoot: boolean;
  activeWarning?: string;
}

export interface InterfaceInfo {
  name: string;
  label?: string;
  displayName?: string;
  ip: string;
  wanIp?: string;
  isp?: string;
  location?: string;
  isUp: boolean;
  isHardware: boolean;
  isDefault: boolean;
  isVirtual: boolean;
  latencyRouterMs: number;
  latencyWebMs: number;
  rxMbps: number;
  txMbps: number;
  linkState: string;
  lastProbeAt?: string;
  rxLimitMbps?: number;
  txLimitMbps?: number;
  dataQuotaMb?: number;
  dataUsedMb?: number;
}

export interface ProcessInfo {
  pid: number;
  name: string;
  executable: string;
  commandLine: string;
  iconHint: string;
  bytesSent: number;
  bytesReceived: number;
  currentMbps: number;
  assignedNic: string;
  ruleMatched?: string;
  lastActive: string;
  socketCount?: number;
  customDns?: string;
  bandwidthLimitMbps?: number;
  dataQuotaMb?: number;
  dataUsedMb?: number;
  quotaExceeded?: boolean;
}

export interface RuleRecord {
  id: string;
  appName: string;
  appPath: string;
  domainPattern: string;
  preferredInterface: string;
  fallbackInterface: string;
  priority: number;
  bandwidthLimitMbps: number;
  dataQuotaMb: number;
  dataUsedMb: number;
  customDns?: string;
  enabled: boolean;
}

export interface AdapterCapRecord {
  interfaceName: string;
  rxLimitMbps: number;
  txLimitMbps: number;
  dataQuotaMb: number;
  dataUsedMb: number;
  enabled: boolean;
}

export interface MonthlyUsageRecord {
  id: number;
  yearMonth: string;
  interfaceName: string;
  appName: string;
  bytesIn: number;
  bytesOut: number;
  rxMb: number;
  txMb: number;
  totalMb: number;
  updatedAt: string;
}

export interface MonthlySummary {
  yearMonth: string;
  totalRxMb: number;
  totalTxMb: number;
  totalMb: number;
  topApp?: string;
  topNic?: string;
}

export interface MonthlyHistoryResponse {
  records: MonthlyUsageRecord[];
  summaries: MonthlySummary[];
}

export type UsagePeriod = 'today' | 'week' | 'month' | 'all';

export interface UsageHistoryRecord {
  period: string;
  interfaceName: string;
  appName: string;
  bytesIn: number;
  bytesOut: number;
  rxMb: number;
  txMb: number;
  totalMb: number;
  updatedAt: string;
}

export interface UsageHistorySummary {
  period: string;
  totalRxMb: number;
  totalTxMb: number;
  totalMb: number;
}

export interface UsageHistoryResponse {
  period: UsagePeriod;
  from: string;
  to: string;
  records: UsageHistoryRecord[];
  summaries: UsageHistorySummary[];
  totalRxMb: number;
  totalTxMb: number;
  totalMb: number;
}

export interface NetMergeSlotStats {
  Name: string;
  Index: number;
  Present: boolean;
  State: string;
  State6: string;
  Weight: number;
  Weight6: number;
  Active: number;
  Total: number;
  RateIn: number;
  RateOut: number;
  LinkIn: number;
  LinkOut: number;
  Failed: number;
  LimitMbps: number;
}

export interface NetMergeBinaryInfo {
  path: string;
  trayPath?: string;
  customPath: string;
  engineSource?: 'builtin' | 'external';
  exists: boolean;
  source: 'builtin' | 'custom' | 'same-folder' | 'path' | 'system' | 'none';
  version: string;
  isValid: boolean;
  error?: string;
}

export interface NetMergeConfig {
  autoPause: boolean;
  autoWeight: boolean;
  ipv6Mode: string;
  mtu: number;
  includeNics: string;
  rescanSec: number;
  socksAddress: string;
  weights: Record<string, number>;
  showConsole: boolean;
  autostart: boolean;
  engineSource?: 'builtin' | 'external';
  binaryPath?: string;
  routingMode?: 'speed' | 'backup';
  primaryAdapter?: string;
  secondaryAdapter?: string;
  closeAction?: 'minimize' | 'close';
}

export interface NetMergeStatus {
  isRunning: boolean;
  pid: number;
  mode: string;
  routingMode?: 'speed' | 'backup';
  defaultAdapter?: string;
  activeAdapter?: string;
  v6State: string;
  uptimeSec: number;
  autostart: boolean;
  socksRunning: boolean;
  socksAddress: string;
  config: NetMergeConfig;
  nics: NetMergeSlotStats[];
  limits: Record<string, number>;
  binary?: NetMergeBinaryInfo;
}

export interface TTLSetting {
  id: string;
  kind: string;
  family: string;
  scope: string;
  interface?: string;
  interfaceIdx?: number;
  name: string;
  description: string;
  value: number | null;
  valueOrigin?: string;
  explicit: boolean;
  writable: boolean;
  writableReason?: string;
}

export interface TTLReport {
  schema: string;
  at: string;
  appVersion?: string;
  capabilities: {
    platform: string;
    supported: boolean;
    readable: boolean;
    notes: string[];
  };
  settings: TTLSetting[];
  elevated: boolean;
  elevationNote?: string;
  warnings?: string[];
}

export interface TTLOutcome {
  schema?: string;
  action: string;
  dryRun: boolean;
  applied?: number;
  failed?: number;
  changes: Array<{
    setting: any;
    from: number | null;
    to: number;
    outcome?: string;
    detail?: string;
    verified?: boolean;
    reverted?: boolean;
    error?: string;
  }>;
  rolledBack?: boolean;
  rollbackIncomplete?: boolean;
  rollbackDetail?: string[];
  notes?: string[];
  at: string;
}

export interface ScenarioAssertion {
  name: string;
  expected: string;
  actual: string;
  passed: boolean;
}

export interface ScenarioResult {
  id: string;
  name: string;
  category: string;
  description: string;
  status: 'PASS' | 'FAIL' | 'SKIPPED' | 'BLOCKED';
  durationMs: number;
  assertions?: ScenarioAssertion[];
  error?: string;
  affectsConnection?: boolean;
  disruptionWarning?: string;
}

export interface TestCaseResult {
  name: string;
  classification: string;
  status: 'PASS' | 'FAIL' | 'SKIPPED' | 'BLOCKED' | 'NOT RUN';
  durationMs: number;
  output?: string;
  reason?: string;
  affectsConnection?: boolean;
  disruptionWarning?: string;
}

export interface SuiteReport {
  suiteName?: string;
  suite?: string;
  targetOs: string;
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  blocked: number;
  durationMs: number;
  cases: TestCaseResult[];
}

export interface UnifiedTestReport {
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  blocked: number;
  disruptiveCount: number;
  durationMs: number;
  suites: SuiteReport[];
  scenarios: ScenarioResult[];
  ranAt: string;
}

export interface InterfaceHealth {
  name: string;
  state: 'UP' | 'DEGRADED' | 'DOWN' | 'RECOVERING';
  consecutiveFailures: number;
  consecutiveSuccesses: number;
  lastProbeAt: string;
  lastLatencyMs: number;
  isFlapping: boolean;
  flapCount: number;
}

export interface LogEntry {
  timestamp: string;
  level: 'DEBUG' | 'INFO' | 'WARN' | 'ERROR';
  component: string;
  phase?: string;
  operationId?: string;
  operationName?: string;
  correlationId?: string;
  executionMode: string;
  targetPlatform: string;
  message: string;
  durationMs?: number;
  fields?: Record<string, any>;
  error?: string;
}

export interface TimeSeriesPoint {
  timestamp: string;
  totalRxMbps: number;
  totalTxMbps: number;
  byInterface: Record<string, number>;
}

export interface AnalyticsSnapshot {
  nics: any[];
  apps: any[];
  latestPoint: TimeSeriesPoint;
  history: TimeSeriesPoint[];
}

export interface PingResult {
  host: string;
  sent: number;
  received: number;
  packetLoss: number;
  minMs: number;
  avgMs: number;
  maxMs: number;
  durationMs: number;
  output?: string;
  error?: string;
}

export interface DNSResult {
  host: string;
  type: string;
  records: string[];
  durationMs: number;
  error?: string;
}

export interface PortResult {
  host: string;
  port: number;
  open: boolean;
  latencyMs: number;
  error?: string;
}

export interface AdapterLabelRecord {
  interfaceName: string;
  label: string;
  updatedAt?: string;
}

export interface ResetUsageResult {
  adapterCounters: number;
  dailyRows: number;
  monthlyRows: number;
}

export interface ConfigImportResult {
  rulesImported: number;
  adapterCapsImported: number;
  labelsImported: number;
  netMergeImported: boolean;
}

export interface ConfigBundle {
  schema: string;
  version: number;
  exportedAt: string;
  appVersion: string;
  rules: RuleRecord[];
  adapterCaps: AdapterCapRecord[];
  adapterLabels?: AdapterLabelRecord[];
  netMerge?: NetMergeConfig;
}

// Fetch from Wails backend HTTP router (/api/...) or local loopback daemon (http://127.0.0.1:4545/api/...)
async function fetchApi<T>(endpoint: string, options?: RequestInit): Promise<T | null> {
  const urls = [`/api/${endpoint}`, `http://127.0.0.1:4545/api/${endpoint}`];
  let lastErr: Error | null = null;

  for (const url of urls) {
    try {
      const res = await fetch(url, options);
      if (res.ok) {
        return await res.json() as T;
      }
      try {
        const errBody = await res.json();
        if (errBody && errBody.error) {
          lastErr = new Error(errBody.error);
          break;
        }
      } catch (_) {}
    } catch (_) {
      // Try next URL
    }
  }

  if (lastErr) {
    throw lastErr;
  }

  const w = (window as any).wails;
  if (w?.Call?.ByName) {
    try {
      const parts = endpoint.split('?')[0];
      const methodName = parts.charAt(0).toUpperCase() + parts.slice(1);
      return await w.Call.ByName(`main.NetPilotService.${methodName}`) as T;
    } catch (_) {}
  }
  throw new Error(`Backend unavailable for '${endpoint}' (no live data source; refusing to return dummy data)`);
}

function requireApi<T>(res: T | null, endpoint: string): T {
  if (res === null || res === undefined) {
    throw new Error(`Backend returned no data for '${endpoint}' (refusing to return dummy data)`);
  }
  return res;
}

export const api = {
  async getBranding(): Promise<BrandingInfo> {
    const res = await fetchApi<BrandingInfo>('branding');
    return requireApi(res, 'branding');
  },

  async getCapabilities(): Promise<CapabilityReport> {
    const res = await fetchApi<CapabilityReport>('capabilities');
    return requireApi(res, 'capabilities');
  },

  async getExecutionMode(): Promise<ExecutionModeState> {
    const res = await fetchApi<ExecutionModeState>('mode');
    return requireApi(res, 'mode');
  },

  async setExecutionMode(mode: 'LIVE' | 'DRY_RUN', forceLive: boolean): Promise<ExecutionModeState> {
    const res = await fetchApi<ExecutionModeState>('mode', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode, forceLive }),
    });
    return requireApi(res, 'mode');
  },

  async getInterfaces(): Promise<InterfaceInfo[]> {
    const res = await fetchApi<InterfaceInfo[]>('interfaces');
    return requireApi(res, 'interfaces');
  },

  async getApplications(): Promise<ProcessInfo[]> {
    const res = await fetchApi<ProcessInfo[]>('applications');
    return requireApi(res, 'applications');
  },

  async getRules(): Promise<RuleRecord[]> {
    const res = await fetchApi<RuleRecord[]>('rules');
    return requireApi(res, 'rules');
  },

  async saveRule(rule: RuleRecord): Promise<void> {
    await fetchApi<void>('rules', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(rule),
    });
  },

  async deleteRule(id: string): Promise<void> {
    await fetchApi<void>(`rules?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  },

  async runAllScenarios(): Promise<ScenarioResult[]> {
    const res = await fetchApi<ScenarioResult[]>('scenarios');
    return requireApi(res, 'scenarios');
  },

  async runPlatformSuite(suite: string): Promise<SuiteReport> {
    const res = await fetchApi<SuiteReport>(`suite?suite=${encodeURIComponent(suite)}`);
    return requireApi(res, `suite?suite=${suite}`);
  },

  async runAllUnified(skipDisruptive: boolean = true): Promise<UnifiedTestReport> {
    const res = await fetchApi<UnifiedTestReport>(`testrunner/run-all?skipDisruptive=${skipDisruptive}`);
    return requireApi(res, `testrunner/run-all?skipDisruptive=${skipDisruptive}`);
  },

  async getFailoverStatus(): Promise<InterfaceHealth[]> {
    const res = await fetchApi<InterfaceHealth[]>('failover');
    return requireApi(res, 'failover');
  },

  async queryLogs(opts: any): Promise<LogEntry[]> {
    const params = new URLSearchParams();
    if (opts.level) params.set('level', opts.level);
    if (opts.component) params.set('component', opts.component);
    if (opts.searchText) params.set('search', opts.searchText);
    if (opts.errorsOnly) params.set('errorsOnly', 'true');
    const res = await fetchApi<LogEntry[]>(`logs?${params.toString()}`);
    return requireApi(res, `logs?${params.toString()}`);
  },

  async getAnalytics(): Promise<AnalyticsSnapshot> {
    const res = await fetchApi<AnalyticsSnapshot>('analytics');
    return requireApi(res, 'analytics');
  },

  async exportDiagnosticReport(strictRedact: boolean): Promise<string> {
    for (const prefix of ['/api', 'http://127.0.0.1:4545/api']) {
      try {
        const res = await fetch(`${prefix}/diagnostics?strict=${strictRedact}`);
        if (res.ok) {
          return await res.text();
        }
      } catch (_) {}
    }
    throw new Error('Backend unavailable for diagnostics export (refusing to return dummy data)');
  },

  async getAdapterCaps(): Promise<AdapterCapRecord[]> {
    const res = await fetchApi<AdapterCapRecord[]>('adapter-caps');
    return requireApi(res, 'adapter-caps');
  },

  async saveAdapterCap(cap: AdapterCapRecord): Promise<void> {
    await fetchApi<void>('adapter-caps', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cap),
    });
  },

  async getUsageHistory(period: UsagePeriod): Promise<UsageHistoryResponse> {
    const url = `usage-history?period=${encodeURIComponent(period)}`;
    const res = await fetchApi<UsageHistoryResponse>(url);
    return requireApi(res, url);
  },

  async getMonthlyHistory(month?: string): Promise<MonthlyHistoryResponse> {
    const url = month ? `monthly-history?month=${encodeURIComponent(month)}` : 'monthly-history';
    const res = await fetchApi<MonthlyHistoryResponse>(url);
    return requireApi(res, url);
  },

  exportMonthlyCsvUrl(month?: string): string {
    return month ? `/api/monthly-history/export?month=${encodeURIComponent(month)}` : '/api/monthly-history/export';
  },

  async exportMonthlyCsvText(month?: string): Promise<string> {
    const suffix = month ? `monthly-history/export?month=${encodeURIComponent(month)}` : 'monthly-history/export';
    for (const prefix of ['/api', 'http://127.0.0.1:4545/api']) {
      try {
        const res = await fetch(`${prefix}/${suffix}`);
        if (res.ok) {
          return await res.text();
        }
      } catch (_) {}
    }
    throw new Error('Backend unavailable for monthly-history export (refusing to return dummy data)');
  },

  // Net-Merge Engine Integration
  async getNetMergeStatus(): Promise<NetMergeStatus> {
    const res = await fetchApi<NetMergeStatus>('netmerge/status');
    return requireApi(res, 'netmerge/status');
  },

  async toggleNetMerge(on: boolean): Promise<void> {
    await fetchApi<void>('netmerge/toggle', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ on }),
    });
  },

  async setNetMergeAutostart(enabled: boolean): Promise<void> {
    await fetchApi<void>('netmerge/autostart', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
    });
  },

  async setNetMergeLimit(adapter: string, mbps: number): Promise<void> {
    await fetchApi<void>('netmerge/limit', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ adapter, mbps }),
    });
  },

  async cleanupNetMerge(): Promise<void> {
    await fetchApi<void>('netmerge/cleanup', {
      method: 'POST',
    });
  },

  async controlNetMergeSocks(action: 'start' | 'stop', address?: string): Promise<void> {
    await fetchApi<void>('netmerge/socks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action, address }),
    });
  },

  async getNetMergeConfig(): Promise<NetMergeConfig> {
    const res = await fetchApi<NetMergeConfig>('netmerge/config');
    return requireApi(res, 'netmerge/config');
  },

  async saveNetMergeConfig(cfg: NetMergeConfig): Promise<void> {
    await fetchApi<void>('netmerge/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cfg),
    });
  },

  async getNetMergeBinaryInfo(): Promise<NetMergeBinaryInfo> {
    const res = await fetchApi<NetMergeBinaryInfo>('netmerge/binary');
    return requireApi(res, 'netmerge/binary');
  },

  async setNetMergeBinaryPath(path: string): Promise<NetMergeBinaryInfo> {
    const res = await fetchApi<NetMergeBinaryInfo>('netmerge/binary', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    });
    if (res) return res;
    throw new Error('Failed to set net-merge binary path');
  },

  async setNetMergeEngineSource(source: 'builtin' | 'external', path: string = ''): Promise<NetMergeBinaryInfo> {
    const res = await fetchApi<NetMergeBinaryInfo>('netmerge/binary', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ source, path }),
    });
    if (res) return res;
    throw new Error('Failed to update net-merge engine source');
  },

  async setNetMergeRoutingMode(mode: 'speed' | 'backup', primary: string = '', secondary: string = ''): Promise<void> {
    await fetchApi<void>('netmerge/routing-mode', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode, primary, secondary }),
    });
  },

  async getCloseAction(): Promise<'minimize' | 'close'> {
    const res = await fetchApi<{ closeAction: 'minimize' | 'close' }>('system/close-action');
    return requireApi(res, 'system/close-action').closeAction;
  },

  async setCloseAction(action: 'minimize' | 'close'): Promise<'minimize' | 'close'> {
    const res = await fetchApi<{ closeAction: 'minimize' | 'close' }>('system/close-action', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action }),
    });
    return requireApi(res, 'system/close-action').closeAction;
  },

  async setConsoleVisible(show: boolean): Promise<void> {
    await fetchApi<void>('system/console', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ show }),
    });
  },

  // Network tools (ping, DNS, TCP port check, Wake-on-LAN)
  async pingHost(host: string, count: number = 4, timeoutMs: number = 1000): Promise<PingResult> {
    const res = await fetchApi<PingResult>('tools/ping', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ host, count, timeoutMs }),
    });
    return requireApi(res, 'tools/ping');
  },

  async lookupDns(host: string, recordType: string = 'A'): Promise<DNSResult> {
    const res = await fetchApi<DNSResult>('tools/dns', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ host, type: recordType }),
    });
    return requireApi(res, 'tools/dns');
  },

  async checkTcpPort(host: string, port: number, timeoutMs: number = 3000): Promise<PortResult> {
    const res = await fetchApi<PortResult>('tools/port', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ host, port, timeoutMs }),
    });
    return requireApi(res, 'tools/port');
  },

  async wakeOnLan(mac: string, broadcast: string = '', port: number = 9): Promise<void> {
    await fetchApi<void>('tools/wol', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mac, broadcast, port }),
    });
  },

  // Adapter friendly names
  async getAdapterLabels(): Promise<AdapterLabelRecord[]> {
    const res = await fetchApi<AdapterLabelRecord[]>('adapter-labels');
    return requireApi(res, 'adapter-labels');
  },

  async saveAdapterLabel(interfaceName: string, label: string): Promise<void> {
    await fetchApi<void>('adapter-labels', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ interfaceName, label }),
    });
  },

  async deleteAdapterLabel(interfaceName: string): Promise<void> {
    await fetchApi<void>(`adapter-labels?interface=${encodeURIComponent(interfaceName)}`, {
      method: 'DELETE',
    });
  },

  // Usage maintenance
  async resetDataUsage(scope: 'adapter' | 'daily' | 'monthly' | 'all', interfaceName: string = ''): Promise<ResetUsageResult> {
    const res = await fetchApi<ResetUsageResult>('usage/reset', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ scope, interface: interfaceName }),
    });
    return requireApi(res, 'usage/reset');
  },

  // Configuration backup & restore
  async exportConfigBundle(): Promise<ConfigBundle> {
    const res = await fetchApi<ConfigBundle>('config/export');
    return requireApi(res, 'config/export');
  },

  async importConfigBundle(bundle: ConfigBundle): Promise<ConfigImportResult> {
    const res = await fetchApi<ConfigImportResult>('config/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(bundle),
    });
    return requireApi(res, 'config/import');
  },

  // TTL & Hop Limit Management
  async inspectTTL(): Promise<TTLReport> {
    const res = await fetchApi<TTLReport>('ttl/inspect');
    return requireApi(res, 'ttl/inspect');
  },

  async applyTTL(req: {
    bumpIPv4: number;
    bumpIPv6: number;
    policy: string;
    ifaces: string[];
    dryRun: boolean;
  }): Promise<TTLOutcome> {
    const res = await fetchApi<TTLOutcome>('ttl/apply', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    return requireApi(res, 'ttl/apply');
  },

  async restoreTTL(dryRun: boolean = false): Promise<TTLOutcome> {
    const res = await fetchApi<TTLOutcome>('ttl/restore', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dryRun }),
    });
    return requireApi(res, 'ttl/restore');
  },

  async getTTLHistory(): Promise<any[]> {
    const res = await fetchApi<any[]>('ttl/history');
    return requireApi(res, 'ttl/history');
  },
};
