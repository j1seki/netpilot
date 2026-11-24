import React, { useState } from 'react';
import { X } from 'lucide-react';
import { api, type InterfaceInfo, type RuleRecord } from '../api/wails';

export type RuleDraft = { rule?: RuleRecord; appName?: string; appPath?: string; example?: Partial<RuleRecord> };

export function RuleEditor({ draft, interfaces, close, saved }: {
  draft: RuleDraft;
  interfaces: InterfaceInfo[];
  close: () => void;
  saved: () => void;
}) {
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
  const set = <K extends keyof RuleRecord>(key: K, value: RuleRecord[K]) =>
    setRule(current => ({ ...current, [key]: value }));

  const adapters = interfaces.filter(link => !link.isVirtual);
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
      setError('Choose physical adapters detected by this PC.');
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

  return <div className="rule-editor-overlay" onKeyDown={e => { if (e.key === 'Escape' && !saving) close(); }}>
    <form className="card rule-editor" role="dialog" aria-modal="true" aria-labelledby="rule-editor-title" onSubmit={save}>
      <div className="card-title">
        <h3 id="rule-editor-title">{original ? 'Edit routing policy' : 'New routing policy'}</h3>
        <button type="button" className="btn btn-secondary btn-sm" onClick={close} disabled={saving} aria-label="Close policy editor"><X size={14} /></button>
      </div>
      <p className="rule-editor-note">Set a name, path, or destination. A match on any one can select the policy. The highest-priority match wins.</p>
      <label>Application name<input autoFocus value={rule.appName} onChange={e => set('appName', e.target.value)} placeholder="chrome.exe" /></label>
      <label>Executable or macOS app path<input value={rule.appPath} onChange={e => set('appPath', e.target.value)} placeholder="/usr/bin/example" /></label>
      <label>Destinations<input value={rule.domainPattern ?? ''} onChange={e => set('domainPattern', e.target.value)} placeholder="*.example.com, 10.0.0.0/8" /></label>
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
      {!adapters.length && <p className="rule-editor-note">No physical adapters detected.</p>}
      {rule.preferredInterface && !preferredAvailable && <p className="rule-editor-note">Saved preferred route {rule.preferredInterface} needs a detected adapter before saving edits.</p>}
      {!fallbackAvailable && <p className="rule-editor-note">Saved fallback route {rule.fallbackInterface} needs a detected adapter or None.</p>}
      <p className="rule-editor-note">A pinned rule with no usable configured adapter blocks new matching flows.</p>
      <div className="grid-2">
        <label>Shared speed cap (Mbps)<input type="number" min="0" step="any" value={rule.bandwidthLimitMbps} onChange={e => set('bandwidthLimitMbps', Number(e.target.value))} /></label>
        <label>Data quota (MB)<input type="number" min="0" step="any" value={rule.dataQuotaMb} onChange={e => set('dataQuotaMb', Number(e.target.value))} /></label>
      </div>
      <div className="grid-2">
        <label>Custom DNS server IP<input value={rule.customDns ?? ''} onChange={e => set('customDns', e.target.value)} placeholder="1.1.1.1" /></label>
        <label>Priority<input type="number" step="1" required value={rule.priority} onChange={e => set('priority', Number(e.target.value))} /></label>
      </div>
      <p className="rule-editor-note">Use an IP address with an optional port. Custom DNS applies to SOCKS hostname lookups and attributable port 53 traffic captured by NetPilot.</p>
      {error && <p role="alert" style={{ color: '#fb7185' }}>{error}</p>}
      <div className="rule-editor-actions">
        <button type="button" className="btn btn-secondary btn-sm" disabled={saving} onClick={close}>Cancel</button>
        <button type="submit" className="btn btn-primary btn-sm" disabled={saving || !preferredAvailable || !fallbackAvailable}>{saving ? 'Saving…' : 'Save policy'}</button>
      </div>
    </form>
  </div>;
}
