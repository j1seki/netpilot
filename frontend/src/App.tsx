import React, { useEffect, useState } from 'react';
import {
  api,
  BrandingInfo,
  InterfaceInfo,
  NetMergeStatus,
  subscribeTelemetry,
  engineVersionLabel,
} from './api/wails';
import { listenForDesktopTests } from './desktopTests';
import { Sparkline } from './components/Charts';
import { DashboardView } from './views/DashboardView';
import { NetMergeView } from './views/NetMergeView';
import { ConnectivityView } from './views/ConnectivityView';
import { RoutingRules } from './views/RoutingRules';
import { Analytics } from './views/Analytics';
import { TestDebug } from './views/TestDebug';
import { Tools } from './views/Tools';
import { Settings } from './views/Settings';
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
const openTTL = new URLSearchParams(window.location.search).get('tab') === 'ttl';

function initialTab(): MainTab {
  // Deep links are handy for docs and for jumping straight to a page:
  // index.html?tab=usage#/usage-style URLs from the tray/docs.
  const requested = new URLSearchParams(window.location.search).get('tab') as MainTab | null;
  if (openTTL) return 'connectivity';
  return requested && MAIN_TABS.includes(requested) ? requested : 'dashboard';
}

export const App: React.FC = () => {
  useEffect(listenForDesktopTests, []);
  const startTab = initialTab();
  const [currentTab, setCurrentTab] = useState<MainTab>(startTab);
  const [branding, setBranding] = useState<BrandingInfo | null>(null);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [netMergeStatus, setNetMergeStatus] = useState<NetMergeStatus | null>(null);
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

  const loadHeaderData = async () => {
    try {
      const [b, ifaces, nm] = await Promise.all([
        api.getBranding(),
        api.getInterfaces(),
        api.getNetMergeStatus(),
      ]);
      setBranding(b);
      setInterfaces(ifaces);
      setNetMergeStatus(nm);

      const totalRx = ifaces.reduce((acc, i) => acc + (i.rxMbps || 0), 0);
      setGlobalRxHist(prev => [...prev.slice(1), totalRx]);
    } catch (_) {}
  };

  useEffect(() => {
    loadHeaderData();
    const interval = setInterval(loadHeaderData, 2500);
    return () => clearInterval(interval);
  }, []);

  // Live telemetry from the backend's Server-Sent Events stream. The polling effect above
  // is a fallback for environments where EventSource cannot reach /api/events.
  useEffect(() => {
    return subscribeTelemetry(payload => {
      if (payload.interfaces) {
        setInterfaces(payload.interfaces);
        const totalRx = payload.interfaces.reduce((acc, i) => acc + (i.rxMbps || 0), 0);
        setGlobalRxHist(prev => [...prev.slice(1), totalRx]);
      }
      if (payload.netMerge) {
        setNetMergeStatus(payload.netMerge);
      }
    });
  }, []);

  const switchTab = (tab: MainTab) => {
    setCurrentTab(tab);
    setVisitedTabs(prev => ({ ...prev, [tab]: true }));
  };

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
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      height: '100vh',
      width: '100vw',
      backgroundColor: '#09090b',
      color: '#fafafa',
      overflow: 'hidden',
    }}>
      {/* Top header */}
      <header className="app-header" style={{
        backgroundColor: '#0c0c0f',
        borderBottom: '1px solid #232329',
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
            backgroundColor: isMergeRunning ? 'rgba(16, 185, 129, 0.14)' : '#18181b',
            border: `1px solid ${isMergeRunning ? 'rgba(16, 185, 129, 0.4)' : '#27272a'}`,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}>
            <GitMerge size={16} color={isMergeRunning ? '#10b981' : '#a1a1aa'} />
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div>
              <div className="app-brand-status" style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span style={{ fontSize: '14.5px', fontWeight: 700, color: '#fafafa', letterSpacing: '-0.02em' }}>
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
                  color: isMergeRunning ? '#34d399' : '#a1a1aa',
                  border: `1px solid ${isMergeRunning ? 'rgba(16, 185, 129, 0.3)' : '#27272a'}`,
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
                    color: routingMode === 'backup' ? '#fbbf24' : '#38bdf8',
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

        {/* Center: Live Host Bandwidth Telemetry + Quick Power Switch */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: '14px',
            backgroundColor: '#121215',
            border: '1px solid #232329',
            borderRadius: '7px',
            padding: '4px 12px',
          }}>
            <div style={{ width: '88px', height: '20px' }}>
              <Sparkline data={globalRxHist} color={isMergeRunning ? '#10b981' : '#38bdf8'} width={88} height={20} />
            </div>

            <div className="font-mono" style={{ display: 'flex', alignItems: 'center', gap: '12px', fontSize: '11.5px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: '#38bdf8', fontWeight: 600 }}>
                <ArrowDown size={12} />
                <span>{activeRxStr}</span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: '#f59e0b', fontWeight: 600 }}>
                <ArrowUp size={12} />
                <span>{activeTxStr}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Navigation */}
        <nav className="app-nav" aria-label="Main navigation">
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
        </nav>
      </header>

      {/* Main Content Area */}
      <main className="app-content" style={{
        flex: 1,
        overflowY: 'auto',
        backgroundColor: '#09090b',
      }}>
        <div style={{ display: currentTab === 'dashboard' ? 'block' : 'none' }}>
          <DashboardView />
        </div>
        {visitedTabs.netmerge && (
          <div style={{ display: currentTab === 'netmerge' ? 'block' : 'none' }}>
            <NetMergeView />
          </div>
        )}
        {visitedTabs.connectivity && (
          <div style={{ display: currentTab === 'connectivity' ? 'block' : 'none' }}>
            <ConnectivityView initialTTL={openTTL} />
          </div>
        )}
        {visitedTabs.rules && (
          <div style={{ display: currentTab === 'rules' ? 'block' : 'none' }}>
            <RoutingRules />
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
      </main>
    </div>
  );
};
