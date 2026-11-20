import type { ProcessInfo } from './api/wails';

export type ProcessSortKey = 'name' | 'pid' | 'networkMbps' | 'cpuPercent' | 'memoryBytes';
export type ProcessSort = { key: ProcessSortKey; direction: 'ascending' | 'descending' };
export type ProcessMetrics = Pick<ProcessInfo, ProcessSortKey | 'networkSource'>;
type ProcessGroup = ProcessMetrics & {
  key: string;
  processes: ProcessInfo[];
};
const processMetricKeys = ['networkMbps', 'cpuPercent', 'memoryBytes'] as const;

function compareProcesses(a: ProcessMetrics, b: ProcessMetrics, { key, direction }: ProcessSort) {
  const order = direction === 'ascending' ? 1 : -1;
  if (key === 'name') return order * a.name.localeCompare(b.name) || a.pid - b.pid;
  const left = a[key], right = b[key];
  if (left == null && right != null) return 1;
  if (left != null && right == null) return -1;
  return order * ((left ?? 0) - (right ?? 0)) || a.name.localeCompare(b.name) || a.pid - b.pid;
}

export function groupProcesses(apps: ProcessInfo[], query: string, sort: ProcessSort) {
  const groups = new Map<string, ProcessGroup>();
  for (const app of apps) {
    if (query && !app.name.toLowerCase().includes(query) && !app.executable.toLowerCase().includes(query) && String(app.pid) !== query) {
      continue;
    }
    const path = app.executable;
    const windowsPath = /^[a-z]:[\\/]|^\\\\/i.test(path);
    const key = windowsPath ? path.replace(/\\/g, '/').toLowerCase()
      : path.startsWith('/') ? path : `name:${app.name}`;
    let group = groups.get(key);
    if (!group) {
      group = { key, name: app.name, pid: app.pid, processes: [] };
      groups.set(key, group);
    }
    group.processes.push(app);
    if (app.pid < group.pid) {
      group.pid = app.pid;
      group.name = app.name;
    }
    for (const metric of processMetricKeys) {
      const value = app[metric];
      if (value != null) group[metric] = (group[metric] ?? 0) + value;
    }
  }
  for (const group of groups.values()) {
    group.processes.sort((a, b) => compareProcesses(a, b, sort));
  }
  return Array.from(groups.values()).sort((a, b) => compareProcesses(a, b, sort));
}

export function formatProcessRate(mbps: number | undefined): string {
  if (mbps == null) return '—';
  if (mbps === 0) return '0';
  if (mbps >= 1) return `${mbps.toFixed(2)} Mbps`;
  if (mbps >= 0.001) return `${(mbps * 1000).toFixed(1)} Kbps`;
  return `${Math.round(mbps * 1_000_000)} bps`;
}
