import React, { useEffect, useState } from 'react';
import {
  api,
  BrandingInfo,
  InterfaceInfo,
  NetMergeStatus,
} from './api/wails';
import { Sparkline } from './components/Charts';
import { DashboardView } from './views/DashboardView';
import { NetMergeView } from './views/NetMergeView';
import { TTLView } from './views/TTLView';
import { RoutingRules } from './views/RoutingRules';
import { Analytics } from './views/Analytics';
import { TestDebug } from './views/TestDebug';
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
  ShieldCheck,
} from 'lucide-react';

export type MainTab =
  | 'dashboard'
  | 'netmerge'
  | 'ttl'
  | 'rules'
  | 'usage'
  | 'test-suite'
  | 'settings';

export const App: React.FC = () => {
  const [currentTab, setCurrentTab] = useState<MainTab>('dashboard');
  const [branding, setBranding] = useState<BrandingInfo | null>(null);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [netMergeStatus, setNetMergeStatus] = useState<NetMergeStatus | null>(null);
  const [visitedTabs, setVisitedTabs] = useState<Record<MainTab, boolean>>({
    dashboard: true,
    netmerge: false,
    ttl: false,
    rules: false,
    usage: false,
    'test-suite': false,
    settings: false,
  });

  // Real aggregate rolling history for top-bar sparkline
  const [globalRxHist, setGlobalRxHist] = useState<number[]>([0, 0, 0, 0, 0, 0, 0]);

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

  const switchTab = (tab: MainTab) => {
    setCurrentTab(tab);
    setVisitedTabs(prev => ({ ...prev, [tab]: true }));
  };

  const totalRx = interfaces.reduce((acc, i) => acc + (i.rxMbps || 0), 0);
  const totalTx = interfaces.reduce((acc, i) => acc + (i.txMbps || 0), 0);
  const activeRxStr = totalRx >= 1 ? `${totalRx.toFixed(1)} Mbit/s` : `${(totalRx * 1000).toFixed(0)} kbit/s`;
  const activeTxStr = totalTx >= 1 ? `${totalTx.toFixed(1)} Mbit/s` : `${(totalTx * 1000).toFixed(0)} kbit/s`;

  const ifaceNames = interfaces.map(i => i.name).join(', ') || 'No active adapters';
  const isMergeRunning = netMergeStatus?.isRunning ?? false;

  const navItems: { id: MainTab; label: string; icon: React.ReactNode }[] = [
    { id: 'dashboard', label: 'Dashboard', icon: <LayoutDashboard size={13} /> },
    { id: 'netmerge', label: 'Net-Merge', icon: <GitMerge size={13} /> },
    { id: 'ttl', label: 'TTL & Hop Limit', icon: <ShieldCheck size={13} /> },
    { id: 'rules', label: 'Rules', icon: <Split size={13} /> },
    { id: 'usage', label: 'Usage', icon: <BarChart2 size={13} /> },
    { id: 'test-suite', label: 'Diagnostics', icon: <Bug size={13} /> },
    { id: 'settings', label: 'Settings', icon: <SettingsIcon size={13} /> },
  ];

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      height: '100vh',
      width: '100vw',
      backgroundColor: '#090b10',
      color: '#f1f5f9',
      overflow: 'hidden',
    }}>
      {/* Clean Native Desktop Header Bar */}
      <header style={{
        height: '52px',
        minHeight: '52px',
        backgroundColor: '#0f131b',
        borderBottom: '1px solid #1e2636',
        padding: '0 16px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        userSelect: 'none',
      }}>
        {/* Left: Application Identity & Status */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <div style={{
            width: '30px',
            height: '30px',
            borderRadius: '6px',
            backgroundColor: isMergeRunning ? '#10b981' : '#0284c7',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}>
            <GitMerge size={17} color="#ffffff" />
          </div>

          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '7px' }}>
              <span style={{ fontSize: '14px', fontWeight: 700, color: '#f1f5f9', letterSpacing: '-0.01em' }}>
                {branding?.name || 'NetPilot'}
              </span>
              <span style={{
                fontSize: '9.5px',
                fontWeight: 700,
                padding: '1px 5px',
                borderRadius: '3px',
                backgroundColor: isMergeRunning ? 'rgba(16, 185, 129, 0.16)' : 'rgba(148, 163, 184, 0.12)',
                color: isMergeRunning ? '#34d399' : '#94a3b8',
                border: `1px solid ${isMergeRunning ? 'rgba(16, 185, 129, 0.3)' : 'rgba(148, 163, 184, 0.2)'}`,
              }}>
                {isMergeRunning ? 'MERGING ON' : 'STANDBY'}
              </span>
            </div>

            <div style={{ fontSize: '10.5px', color: '#64748b' }}>
              {ifaceNames}
            </div>
          </div>
        </div>

        {/* Center: Live Host Throughput */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '18px' }}>
          <div style={{ width: '110px', height: '24px' }}>
            <Sparkline data={globalRxHist} color="#38bdf8" width={110} height={24} />
          </div>

          <div className="font-mono" style={{ display: 'flex', alignItems: 'center', gap: '14px', fontSize: '12px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: '#38bdf8', fontWeight: 600 }}>
              <ArrowDown size={13} />
              <span>{activeRxStr}</span>
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: '4px', color: '#eab308', fontWeight: 600 }}>
              <ArrowUp size={13} />
              <span>{activeTxStr}</span>
            </div>
          </div>
        </div>

        {/* Right: Navigation Tabs (No Duplicate Start Button) */}
        <nav style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
          {navItems.map(item => {
            const active = currentTab === item.id;
            return (
              <button
                key={item.id}
                onClick={() => switchTab(item.id)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '5px',
                  padding: '6px 11px',
                  borderRadius: '5px',
                  fontSize: '11.5px',
                  fontWeight: active ? 600 : 500,
                  cursor: 'pointer',
                  backgroundColor: active ? '#182030' : 'transparent',
                  color: active ? '#f1f5f9' : '#94a3b8',
                  border: `1px solid ${active ? '#2b3954' : 'transparent'}`,
                  transition: 'all 0.12s ease',
                }}
              >
                {item.icon}
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>
      </header>

      {/* Main Content Area — Keep visited views mounted so switching tabs never loses state or re-runs probes */}
      <main style={{
        flex: 1,
        overflowY: 'auto',
        padding: '14px 16px',
        backgroundColor: '#090b10',
      }}>
        <div style={{ display: currentTab === 'dashboard' ? 'block' : 'none' }}>
          <DashboardView />
        </div>
        {visitedTabs.netmerge && (
          <div style={{ display: currentTab === 'netmerge' ? 'block' : 'none' }}>
            <NetMergeView />
          </div>
        )}
        {visitedTabs.ttl && (
          <div style={{ display: currentTab === 'ttl' ? 'block' : 'none' }}>
            <TTLView />
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
