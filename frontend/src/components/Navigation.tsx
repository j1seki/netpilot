import React from 'react';
import {
  LayoutDashboard,
  Cpu,
  Network,
  Split,
  BarChart2,
  ArrowRightLeft,
  Bug,
  Settings as SettingsIcon,
  Compass,
} from 'lucide-react';

export type NavTab =
  | 'overview'
  | 'applications'
  | 'networks'
  | 'rules'
  | 'analytics'
  | 'failover'
  | 'test-debug'
  | 'settings';

interface NavigationProps {
  currentTab: NavTab;
  onSelect: (tab: NavTab) => void;
  brandName: string;
  version: string;
}

export const Navigation: React.FC<NavigationProps> = ({
  currentTab,
  onSelect,
  brandName,
  version,
}) => {
  return (
    <aside className="sidebar">
      {/* Brand Header */}
      <div className="brand-header">
        <div className="brand-icon">
          <Compass size={20} />
        </div>
        <div className="brand-info">
          <h1>{brandName}</h1>
          <span className="brand-badge">v{version}</span>
        </div>
      </div>

      {/* Nav Menu Items */}
      <nav className="nav-menu">
        <div
          className={`nav-item ${currentTab === 'overview' ? 'active' : ''}`}
          onClick={() => onSelect('overview')}
        >
          <LayoutDashboard size={18} />
          <span>Overview</span>
        </div>

        <div
          className={`nav-item ${currentTab === 'applications' ? 'active' : ''}`}
          onClick={() => onSelect('applications')}
        >
          <Cpu size={18} />
          <span>Applications</span>
        </div>

        <div
          className={`nav-item ${currentTab === 'networks' ? 'active' : ''}`}
          onClick={() => onSelect('networks')}
        >
          <Network size={18} />
          <span>Networks</span>
        </div>

        <div
          className={`nav-item ${currentTab === 'rules' ? 'active' : ''}`}
          onClick={() => onSelect('rules')}
        >
          <Split size={18} />
          <span>Routing Rules</span>
        </div>

        <div
          className={`nav-item ${currentTab === 'analytics' ? 'active' : ''}`}
          onClick={() => onSelect('analytics')}
        >
          <BarChart2 size={18} />
          <span>Traffic Analytics</span>
        </div>

        <div
          className={`nav-item ${currentTab === 'failover' ? 'active' : ''}`}
          onClick={() => onSelect('failover')}
        >
          <ArrowRightLeft size={18} />
          <span>Failover</span>
        </div>

        {/* First-class Test & Debug Section */}
        <div
          className={`nav-item test-debug ${currentTab === 'test-debug' ? 'active' : ''}`}
          onClick={() => onSelect('test-debug')}
        >
          <Bug size={18} color="#c084fc" />
          <span style={{ fontWeight: 600 }}>Test & Debug</span>
        </div>

        <div
          className={`nav-item ${currentTab === 'settings' ? 'active' : ''}`}
          onClick={() => onSelect('settings')}
        >
          <SettingsIcon size={18} />
          <span>Settings</span>
        </div>
      </nav>

      {/* Footer info */}
      <div className="sidebar-footer">
        <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
          Wails 3 + React Architecture
        </div>
        <div style={{ fontSize: '10.5px', color: 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: '#10b981' }} />
          Engine: Paced & Active
        </div>
      </div>
    </aside>
  );
};
