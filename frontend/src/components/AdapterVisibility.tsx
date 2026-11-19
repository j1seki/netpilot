import { useEffect, useState } from 'react';
import { ChevronRight, Eye, EyeOff } from 'lucide-react';
import { InterfaceInfo } from '../api/wails';

type Group = 'connected' | 'virtual' | 'inactive' | 'hidden';
const labels: Record<Group, string> = { connected: 'Connected', virtual: 'Virtual / VPN', inactive: 'Inactive', hidden: 'Hidden' };
const groups = Object.keys(labels) as Group[];
const key = 'netpilot.adapter-visibility.v1';
const defaults = { collapsed: ['virtual', 'inactive', 'hidden'] as Group[], hidden: [] as string[] };

export function useAdapterVisibility<T extends InterfaceInfo>(adapters: T[]) {
  const [prefs, setPrefs] = useState(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(key) || 'null');
      if (Array.isArray(saved?.hidden) && Array.isArray(saved?.collapsed)) return saved as typeof defaults;
    } catch { /* Default to a tidy list if storage is unavailable. */ }
    return defaults;
  });
  useEffect(() => {
    try { localStorage.setItem(key, JSON.stringify(prefs)); } catch { /* Still works for this session. */ }
  }, [prefs]);
  const byGroup: Record<Group, T[]> = { connected: [], virtual: [], inactive: [], hidden: [] };
  for (const adapter of adapters) {
    const group = prefs.hidden.includes(adapter.name) ? 'hidden'
      : adapter.isDefault ? 'connected'
      : adapter.isVirtual ? 'virtual' : adapter.isUp ? 'connected' : 'inactive';
    byGroup[group].push(adapter);
  }
  const setHidden = (name: string, hidden: boolean) => setPrefs(prev => ({
    ...prev, hidden: hidden ? [...new Set([...prev.hidden, name])] : prev.hidden.filter(item => item !== name),
  }));
  return {
    sections: groups.map(group => ({
      group, label: labels[group], adapters: byGroup[group], open: !prefs.collapsed.includes(group),
      toggle: () => setPrefs(prev => ({ ...prev, collapsed: prev.collapsed.includes(group)
        ? prev.collapsed.filter(item => item !== group) : [...prev.collapsed, group] })),
    })).filter(section => section.adapters.length > 0),
    setHidden,
    restore: () => setPrefs(prev => ({ ...prev, hidden: [] })),
  };
}

export function AdapterGroupButton({ label, count, open, toggle }: { label: string; count: number; open: boolean; toggle: () => void }) {
  return <button type="button" className="adapter-group-button" aria-expanded={open} onClick={toggle}>
    <ChevronRight size={15} aria-hidden="true" /><span>{label}</span><span className="adapter-count">{count}</span>
  </button>;
}

export function AdapterHideButton({ name, hidden, toggle }: { name: string; hidden: boolean; toggle: () => void }) {
  const label = `${hidden ? 'Show' : 'Hide'} ${name} ${hidden ? 'in' : 'from'} the dashboard`;
  return <button type="button" className="btn btn-secondary btn-sm adapter-hide" aria-label={label} title={label} onClick={toggle}>
    {hidden ? <Eye size={14} /> : <EyeOff size={14} />}
  </button>;
}
