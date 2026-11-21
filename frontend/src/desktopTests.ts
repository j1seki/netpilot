import type { ProcessInfo } from './api/wails';
import { rateValue, volumeValue } from './metricUnits';
import { formatProcessRate, groupProcesses } from './processes';

function checkProcessSort() {
  const process = (pid: number, executable: string, networkMbps?: number): ProcessInfo => ({
    pid, name: executable, executable, networkMbps, commandLine: '', iconHint: '',
    bytesSent: 0, bytesReceived: 0, currentMbps: 0, assignedNic: '', lastActive: '',
  });
  const rows = [
    process(1, 'C:\\Apps\\Browser.exe', 0.5),
    process(2, 'c:/apps/browser.exe', 1.5),
    process(3, 'Idle', 0),
    process(4, 'Unavailable'),
  ];
  const ascending = groupProcesses(rows, '', { key: 'networkMbps', direction: 'ascending' });
  const descending = groupProcesses(rows, '', { key: 'networkMbps', direction: 'descending' });
  return ascending.map(row => row.pid).join() === '3,1,4'
    && descending.map(row => row.pid).join() === '1,3,4'
    && ascending[1].networkMbps === 2
    && ascending[1].processes.map(row => row.pid).join() === '1,2'
    && descending[0].processes.map(row => row.pid).join() === '2,1'
    && formatProcessRate(ascending[0].networkMbps) === '0'
    && formatProcessRate(ascending[2].networkMbps) === '—';
}

export function listenForDesktopTests() {
  const respond = (event: Event) => {
    const id = (event as CustomEvent<unknown>).detail;
    if (typeof id !== 'string') return;
    void fetch('/api/testrunner/desktop-result', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, passed: event.type === 'netpilot:test-process-sort' ? checkProcessSort() : checkMetricUnits() }),
    }).catch(() => {});
  };
  window.addEventListener('netpilot:test-process-sort', respond);
  window.addEventListener('netpilot:test-metric-units', respond);
  return () => {
    window.removeEventListener('netpilot:test-process-sort', respond);
    window.removeEventListener('netpilot:test-metric-units', respond);
  };
}

function checkMetricUnits() {
  return volumeValue(1024, 'GiB') === 1
    && volumeValue(1024, 'GB') === 1.073741824
    && volumeValue(1, 'MB') === 1.048576
    && rateValue(8, 'MB/s') === 1
    && rateValue(1, 'Kbps') === 1000
    && rateValue(1000, 'Gbps') === 1;
}
