import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  api,
  BrandingInfo,
  InterfaceInfo,
  NetMergeStatus,
  subscribeTelemetry,
  engineVersionLabel,
} from './api/wails';
import { listenForDesktopTests } from './desktopTests';
import { NotificationBanner } from './components/Notifications';
import { Sparkline } from './components/Charts';
import { DashboardView } from './views/DashboardView';
import { NetMergeView } from './views/NetMergeView';
import { ConnectivityView } from './views/ConnectivityView';
import { RoutingRules } from './views/RoutingRules';
import { Analytics } from './views/Analytics';
import { TestDebug } from './views/TestDebug';
import { Tools } from './views/Tools';
import { Settings } from './views/Settings';
import { SimpleView } from './views/SimpleView';
import {
  GitMerge,
  ArrowDown,
  ArrowUp,
  LayoutDashboard,
  Split,
  BarChart2,
  Bug,
  Settings as SettingsIcon,
  Wrench,
  Zap,
  ShieldAlert,
  Smartphone,
} from 'lucide-react';

export type MainTab =
  | 'dashboard'
  | 'netmerge'
  | 'rules'
  | 'usage'
  | 'tools'
  | 'test-suite'
  | 'settings'
  | 'connectivity';

const MAIN_TABS: MainTab[] = ['dashboard', 'netmerge', 'rules', 'usage', 'tools', 'test-suite', 'settings', 'connectivity'];
const requestedTab = new URLSearchParams(window.location.search).get('tab');
const openTTL = requestedTab === 'ttl';
const viewPreference = 'netpilot.viewMode';
type ViewMode = 'simple' | 'advanced';

function initialTab(): MainTab {
  // Deep links are handy for docs and for jumping straight to a page:
  // index.html?tab=usage#/usage-style URLs from the tray/docs.
  const requested = requestedTab as MainTab | null;
  if (openTTL) return 'connectivity';
  return requested && MAIN_TABS.includes(requested) ? requested : 'dashboard';
}

function initialView(): ViewMode {
  if (openTTL || MAIN_TABS.includes(requestedTab as MainTab)) return 'advanced';
  try {
    return localStorage.getItem(viewPreference) === 'advanced' ? 'advanced' : 'simple';
  } catch {
    return 'simple';
  }
}

export const App: React.FC = () => {
  useEffect(listenForDesktopTests, []);
  const startTab = initialTab();
  const [currentTab, setCurrentTab] = useState<MainTab>(startTab);
  const [view, setView] = useState(initialView);
  const [advancedMounted, setAdvancedMounted] = useState(() => view === 'advanced');
  const [branding, setBranding] = useState<BrandingInfo | null>(null);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [interfacesLoaded, setInterfacesLoaded] = useState(false);
  const [interfacesError, setInterfacesError] = useState('');
  const [netMergeStatus, setNetMergeStatus] = useState<NetMergeStatus | null>(null);
  const [statusError, setStatusError] = useState('');
  const statusRequest = useRef(0);
  const [visitedTabs, setVisitedTabs] = useState<Record<MainTab, boolean>>({
    dashboard: startTab === 'dashboard',
    netmerge: startTab === 'netmerge',
    rules: startTab === 'rules',
    usage: startTab === 'usage',
    tools: startTab === 'tools',
    'test-suite': startTab === 'test-suite',
    settings: startTab === 'settings',
    connectivity: startTab === 'connectivity',
  });

  // Real aggregate rolling history for top-bar sparkline
  const [globalRxHist, setGlobalRxHist] = useState<number[]>([0, 0, 0, 0, 0, 0, 0, 0, 0, 0]);

  const refreshStatus = useCallback(async () => {
    const request = ++statusRequest.current;
    try {
      const status = await api.getNetMergeStatus();
      if (request === statusRequest.current) {
        setNetMergeStatus(status);
        setStatusError('');
      }
    } catch (error) {
      if (request === statusRequest.current) setStatusError(error instanceof Error ? error.message : String(error));
    }
  }, []);

  useEffect(() => {
    let disposed = false;
    api.getBranding().then(result => { if (!disposed) setBranding(result); }).catch(() => {});
    const loadHeaderData = () => {
      void refreshStatus();
      api.getInterfaces().then(result => { if (!disposed) receiveInterfaces(result); }).catch(() => {
        if (!disposed) setInterfacesError('Could not read network connections.');
      });
    };
    loadHeaderData();
    const interval = setInterval(loadHeaderData, 2500);
    return () => {
      disposed = true;
      ++statusRequest.current;
      clearInterval(interval);
    };
  }, [refreshStatus]);

  function receiveInterfaces(next: InterfaceInfo[]) {
    setInterfaces(next);
    setInterfacesLoaded(true);
    setInterfacesError('');
    const totalRx = next.reduce((acc, i) => acc + (i.rxMbps || 0), 0);
    setGlobalRxHist(prev => [...prev.slice(1), totalRx]);
  }

  // Live telemetry from the backend's Server-Sent Events stream. The polling effect above
  // is a fallback for environments where EventSource cannot reach /api/events.
  useEffect(() => {
    return subscribeTelemetry(payload => {
      if (payload.interfaces) {
        receiveInterfaces(payload.interfaces);
      }
      if (payload.netMerge) {
        ++statusRequest.current;
        setNetMergeStatus(payload.netMerge);
        setStatusError('');
      }
    });
  }, []);

  const switchTab = (tab: MainTab) => {
    setCurrentTab(tab);
    setVisitedTabs(prev => ({ ...prev, [tab]: true }));
  };

  const changeView = (next: ViewMode) => {
    if (next === 'advanced') setAdvancedMounted(true);
    setView(next);
    try { localStorage.setItem(viewPreference, next); } catch { /* storage may be disabled */ }
  };

  const openAdvanced = (tab: MainTab) => {
    switchTab(tab);
    changeView('advanced');
  };

  const viewSwitch = (
    <div className="view-switch" role="group" aria-label="Workspace view">
      <button type="button" className="app-nav-link" aria-pressed={view === 'simple'} onClick={() => changeView('simple')}>Simple</button>
      <button type="button" className="app-nav-link" aria-pressed={view === 'advanced'} onClick={() => changeView('advanced')}>Advanced</button>
    </div>
  );

  const totalRx = interfaces.reduce((acc, i) => acc + (i.rxMbps || 0), 0);
  const totalTx = interfaces.reduce((acc, i) => acc + (i.txMbps || 0), 0);
  const activeRxStr = totalRx >= 1 ? `${totalRx.toFixed(2)} Mbps` : `${(totalRx * 1000).toFixed(0)} Kbps`;
  const activeTxStr = totalTx >= 1 ? `${totalTx.toFixed(2)} Mbps` : `${(totalTx * 1000).toFixed(0)} Kbps`;

  const isMergeRunning = netMergeStatus?.isRunning ?? false;
  const routingMode = netMergeStatus?.routingMode || netMergeStatus?.config?.routingMode || 'speed';
  const defaultAdapter = isMergeRunning ? 'net-merge' : 'System Default';

  const navItems: { id: MainTab; label: string; icon: React.ReactNode }[] = [
    { id: 'dashboard', label: 'Systems', icon: <LayoutDashboard size={13} /> },
    { id: 'netmerge', label: 'Net-Merge', icon: <GitMerge size={13} /> },
    { id: 'connectivity', label: 'Mobile', icon: <Smartphone size={13} /> },
    { id: 'rules', label: 'Rules', icon: <Split size={13} /> },
    { id: 'usage', label: 'Metrics', icon: <BarChart2 size={13} /> },
    { id: 'tools', label: 'Tools', icon: <Wrench size={13} /> },
    { id: 'test-suite', label: 'Diagnostics', icon: <Bug size={13} /> },
    { id: 'settings', label: 'Settings', icon: <SettingsIcon size={13} /> },
  ];

  return (
    <div className="app-shell">
      <NotificationBanner />
      <header className="app-header" style={{
        backgroundColor: 'var(--bg-header)',
        borderBottom: '1px solid var(--border-color)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        userSelect: 'none',
      }}>
        {/* Left: Brand Identity + Default Adapter Status Pill */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <div style={{
            width: '30px',
            height: '30px',
            borderRadius: '7px',
            backgroundColor: isMergeRunning ? 'rgba(16, 185, 129, 0.14)' : 'var(--bg-tertiary)',
            border: `1px solid ${isMergeRunning ? 'rgba(16, 185, 129, 0.4)' : 'var(--border-control)'}`,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}>
            <GitMerge size={16} color={isMergeRunning ? 'var(--accent-primary)' : 'var(--text-secondary)'} />
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div>
              <div className="app-brand-status" style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span style={{ fontSize: '14.5px', fontWeight: 700, color: 'var(--text-primary)', letterSpacing: '-0.02em' }}>
                  {branding?.name || 'NetPilot'}
                </span>

                {/* Active Adapter / Standby Badge */}
                <span style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: '5px',
                  fontSize: '10.5px',
                  fontWeight: 600,
                  padding: '2px 8px',
                  borderRadius: '999px',
                  backgroundColor: isMergeRunning ? 'rgba(16, 185, 129, 0.12)' : 'rgba(113, 113, 122, 0.14)',
                  color: isMergeRunning ? 'var(--text-success)' : 'var(--text-secondary)',
                  border: `1px solid ${isMergeRunning ? 'rgba(16, 185, 129, 0.3)' : 'var(--border-control)'}`,
                }}>
                  <span className={`beszel-dot ${isMergeRunning ? 'online' : 'standby'}`} style={{ width: '6px', height: '6px' }} />
                  <span>{isMergeRunning ? `Adapter: ${defaultAdapter}` : 'Standby · System Default'}</span>
                </span>

                {/* Mode Badge when Active */}
                {isMergeRunning && (
                  <span style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '4px',
                    fontSize: '10px',
                    fontWeight: 600,
                    padding: '2px 7px',
                    borderRadius: '999px',
                    backgroundColor: routingMode === 'backup' ? 'rgba(245, 158, 11, 0.12)' : 'rgba(56, 189, 248, 0.12)',
                    color: routingMode === 'backup' ? 'var(--text-warning)' : 'var(--text-info)',
                    border: `1px solid ${routingMode === 'backup' ? 'rgba(245, 158, 11, 0.28)' : 'rgba(56, 189, 248, 0.28)'}`,
                  }}>
                    {routingMode === 'backup' ? <ShieldAlert size={10} /> : <Zap size={10} />}
                    <span>{routingMode === 'backup' ? 'BACKUP FAILOVER' : 'SPEED MERGE'}</span>
                  </span>
                )}
              </div>
              <div className="engine-version">{engineVersionLabel(netMergeStatus)}</div>
            </div>
          </div>
        </div>

        {/* Center: Live host bandwidth */}
        <div className="app-bandwidth">
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: '14px',
            backgroundColor: 'var(--bg-card)',
            border: '1px solid var(--border-color)',
            borderRadius: '7px',
            padding: '4px 12px',
          }}>
            <div style={{ width: '88px', height: '20px' }}>
              <Sparkline data={globalRxHist} color={isMergeRunning ? 'var(--accent-primary)' : 'var(--text-info)'} width={88} height={20} />
            </div>

            <div className="font-mono" style={{ display: 'flex', alignItems: 'center', gap: '12px', fontSize: '11.5px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: 'var(--text-info)', fontWeight: 600 }}>
                <ArrowDown size={12} />
                <span className="app-bandwidth-rate">{activeRxStr}</span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: 'var(--accent-warning)', fontWeight: 600 }}>
                <ArrowUp size={12} />
                <span className="app-bandwidth-rate">{activeTxStr}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Navigation */}
        {view === 'advanced' && <nav className="app-nav" aria-label="Main navigation">
          {navItems.map(item => {
            const active = currentTab === item.id;
            return (
              <button
                type="button"
                className="app-nav-link"
                key={item.id}
                aria-current={active ? 'page' : undefined}
                onClick={() => switchTab(item.id)}
              >
                {item.icon}
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>}
        {viewSwitch}
      </header>

      {view === 'simple' && <SimpleView
        interfaces={interfaces}
        interfacesLoaded={interfacesLoaded}
        interfacesError={interfacesError}
        status={netMergeStatus}
        statusError={statusError}
        refreshStatus={refreshStatus}
        openAdvanced={openAdvanced}
      />}
      {advancedMounted && <main className="app-content" hidden={view !== 'advanced'}>
        <div style={{ display: currentTab === 'dashboard' ? 'block' : 'none' }}>
          <DashboardView active={view === 'advanced' && currentTab === 'dashboard'} />
        </div>
        {visitedTabs.netmerge && (
          <div style={{ display: currentTab === 'netmerge' ? 'block' : 'none' }}>
            <NetMergeView active={view === 'advanced' && currentTab === 'netmerge'} />
          </div>
        )}
        {visitedTabs.connectivity && (
          <div style={{ display: currentTab === 'connectivity' ? 'block' : 'none' }}>
            <ConnectivityView initialTTL={openTTL} />
          </div>
        )}
        {visitedTabs.rules && (
          <div style={{ display: currentTab === 'rules' ? 'block' : 'none' }}>
            <RoutingRules active={currentTab === 'rules'} goToProcesses={() => switchTab('dashboard')} />
          </div>
        )}
        {visitedTabs.usage && (
          <div style={{ display: currentTab === 'usage' ? 'block' : 'none' }}>
            <Analytics />
          </div>
        )}
        {visitedTabs.tools && (
          <div style={{ display: currentTab === 'tools' ? 'block' : 'none' }}>
            <Tools />
          </div>
        )}
        {visitedTabs['test-suite'] && (
          <div style={{ display: currentTab === 'test-suite' ? 'block' : 'none' }}>
            <TestDebug />
          </div>
        )}
        {visitedTabs.settings && (
          <div style={{ display: currentTab === 'settings' ? 'block' : 'none' }}>
            <Settings />
          </div>
        )}
      </main>}
    </div>
  );
};
