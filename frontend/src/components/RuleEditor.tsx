import React, { useEffect, useRef, useState } from 'react';
import { X } from 'lucide-react';
import { api, type InterfaceInfo, type ProcessInfo, type RuleRecord } from '../api/wails';

export type RuleDraft = { rule?: RuleRecord; appName?: string; appPath?: string; example?: Partial<RuleRecord> };

export function RuleEditor({ draft, interfaces, close, saved, simple = false, eligibleVirtual = [] }: {
  draft: RuleDraft;
  interfaces: InterfaceInfo[];
  close: () => void;
  saved: () => void;
  simple?: boolean;
  eligibleVirtual?: string[];
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const original = draft.rule;
  const [rule, setRule] = useState<RuleRecord>(() => original ?? {
    id: `rule-${crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`}`,
    appName: draft.appPath ? '' : draft.appName ?? '',
    appPath: draft.appPath ?? '',
    domainPattern: '',
    preferredInterface: '',
    fallbackInterface: '',
    priority: 50,
    bandwidthLimitMbps: 0,
    dataQuotaMb: 0,
    dataUsedMb: 0,
    customDns: '',
    enabled: true,
    ...draft.example,
  });
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [apps, setApps] = useState<ProcessInfo[] | null>(null);
  const [selectedApp, setSelectedApp] = useState('');
  const [appsError, setAppsError] = useState('');
  const set = <K extends keyof RuleRecord>(key: K, value: RuleRecord[K]) =>
    setRule(current => ({ ...current, [key]: value }));

  useEffect(() => {
    const modal = dialog.current;
    if (!modal) return;
    if (typeof modal.showModal === 'function') {
      modal.showModal();
      return () => modal.close();
    }
    modal.setAttribute('open', '');
    modal.setAttribute('data-fallback', '');
    modal.focus();
  }, []);

  useEffect(() => {
    if (!simple || original) return;
    let disposed = false;
    api.getApplications().then(processes => {
      const unique = new Map<string, ProcessInfo>();
      for (const process of processes) {
        const key = process.executable || process.name;
        if (key && !unique.has(key)) unique.set(key, process);
      }
      if (!disposed) setApps([...unique.values()].sort((a, b) => a.name.localeCompare(b.name)));
    }).catch(() => {
      if (!disposed) setAppsError('Could not list running apps. You can enter an app manually.');
    });
    return () => { disposed = true; };
  }, [simple, original]);

  const pickApp = (pid: string) => {
    setSelectedApp(pid);
    const app = apps?.find(process => String(process.pid) === pid);
    setRule(current => ({
      ...current,
      appName: app?.executable ? '' : app?.name ?? '',
      appPath: app?.executable ?? '',
    }));
  };

  const adapters = interfaces.filter(link => !link.isVirtual || eligibleVirtual.includes(link.name));
  const hasAdapter = (name: string) => adapters.some(link => link.name === name);
  const preferredAvailable = hasAdapter(rule.preferredInterface);
  const fallbackAvailable = !rule.fallbackInterface || hasAdapter(rule.fallbackInterface);
  const options = (excluded: string) => adapters.map(link => (
    <option key={link.name} value={link.name} disabled={link.name === excluded}>
      {link.displayName || link.name}{!link.isUp ? ' · down' : ''}
    </option>
  ));

  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!preferredAvailable || !fallbackAvailable) {
      setError('Choose adapters detected by NetPilot.');
      return;
    }
    setSaving(true);
    setError('');
    try {
      await api.saveRule({
        ...rule,
        appName: rule.appName.trim(),
        appPath: rule.appPath.trim(),
        domainPattern: rule.domainPattern?.trim(),
        customDns: rule.customDns?.trim(),
      });
      saved();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const extraFields = <>
    <label>Destinations<input value={rule.domainPattern ?? ''} onChange={e => set('domainPattern', e.target.value)} placeholder="*.example.com, 10.0.0.0/8" /></label>
    <div className="grid-2">
      <label>Shared speed cap (Mbps)<input type="number" min="0" step="any" value={rule.bandwidthLimitMbps} onChange={e => set('bandwidthLimitMbps', Number(e.target.value))} /></label>
      <label>Data quota (MB)<input type="number" min="0" step="any" value={rule.dataQuotaMb} onChange={e => set('dataQuotaMb', Number(e.target.value))} /></label>
    </div>
    <div className="grid-2">
      <label>Custom DNS server IP<input value={rule.customDns ?? ''} onChange={e => set('customDns', e.target.value)} placeholder="1.1.1.1" /></label>
      <label>Priority<input type="number" step="1" required value={rule.priority} onChange={e => set('priority', Number(e.target.value))} /></label>
    </div>
    <p className="rule-editor-note">Use an IP address with an optional port. Custom DNS applies to SOCKS hostname lookups and attributable port 53 traffic captured by NetPilot.</p>
  </>;

  return <dialog ref={dialog} className="rule-editor-dialog" role="dialog" aria-modal="true" aria-labelledby="rule-editor-title" tabIndex={-1}
    onCancel={event => { event.preventDefault(); if (!saving) close(); }}
    onKeyDown={event => { if (event.key === 'Escape' && event.currentTarget.hasAttribute('data-fallback') && !saving) close(); }}>
    <form className="card rule-editor" onSubmit={save}>
      <div className="card-title">
        <h3 id="rule-editor-title">{original ? 'Edit routing policy' : 'New routing policy'}</h3>
        <button type="button" className="btn btn-secondary btn-sm" onClick={close} disabled={saving} aria-label="Close policy editor"><X size={14} /></button>
      </div>
      {!simple && <p className="rule-editor-note">Set a name, path, or destination. A match on any one can select the policy. The highest-priority match wins.</p>}
      {simple && !original && <>
        <label>Running app<select value={selectedApp} onChange={event => pickApp(event.target.value)} aria-busy={!apps && !appsError}>
          <option value="">{!apps && !appsError ? 'Loading apps…' : 'Enter an app manually'}</option>
          {apps?.map(app => <option key={app.pid} value={app.pid}>{app.name}{app.executable && ` · ${app.executable}`}</option>)}
        </select></label>
        {appsError && <p className="rule-editor-note">{appsError}</p>}
      </>}
      {!selectedApp && <>
        <label>Application name<input value={rule.appName} onChange={e => set('appName', e.target.value)} placeholder="chrome.exe" /></label>
        <label>Executable or macOS app path<input value={rule.appPath} onChange={e => set('appPath', e.target.value)} placeholder="/usr/bin/example" /></label>
      </>}
      <div className="grid-2">
        <label>Preferred adapter<select required value={preferredAvailable ? rule.preferredInterface : ''} onChange={e => set('preferredInterface', e.target.value)}>
          <option value="" disabled>Choose a detected adapter</option>
          {options(rule.fallbackInterface)}
        </select></label>
        <label>Fallback adapter<select value={rule.fallbackInterface} onChange={e => set('fallbackInterface', e.target.value)}>
          {!fallbackAvailable && <option value={rule.fallbackInterface} disabled>Choose a replacement or None</option>}
          <option value="">None</option>
          {options(rule.preferredInterface)}
        </select></label>
      </div>
      {!adapters.length && <p className="rule-editor-note">No eligible adapters detected.</p>}
      {rule.preferredInterface && !preferredAvailable && <p className="rule-editor-note">Saved preferred route {rule.preferredInterface} needs a detected adapter before saving edits.</p>}
      {!fallbackAvailable && <p className="rule-editor-note">Saved fallback route {rule.fallbackInterface} needs a detected adapter or None.</p>}
      <p className="rule-editor-note">A pinned rule with no usable configured adapter blocks new matching flows.</p>
      {simple ? <>
        <label className="rule-editor-enabled"><input type="checkbox" checked={rule.enabled} onChange={event => set('enabled', event.target.checked)} /> Enable rule</label>
        <details className="rule-editor-options"><summary>Advanced rule options</summary><div>{extraFields}</div></details>
      </> : extraFields}
      {error && <p role="alert" style={{ color: 'var(--text-danger)' }}>{error}</p>}
      <div className="rule-editor-actions">
        <button type="button" className="btn btn-secondary btn-sm" disabled={saving} onClick={close}>Cancel</button>
        <button type="submit" className="btn btn-primary btn-sm" disabled={saving || !preferredAvailable || !fallbackAvailable}>{saving ? 'Saving…' : 'Save policy'}</button>
      </div>
    </form>
  </dialog>;
}
