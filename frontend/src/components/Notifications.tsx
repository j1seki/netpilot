import { useEffect, useState } from 'react';
import { Bell, ExternalLink, X } from 'lucide-react';
import { api, type NotificationPreferences, type NotificationStatus } from '../api/wails';

const topics: [keyof NotificationPreferences, string][] = [
  ['links', 'Link and failover events'],
  ['quotas', 'Adapter quota alerts'],
  ['updates', 'Update alerts'],
  ['desktop', 'Also show desktop banners'],
];

export function Notifications() {
  const [status, setStatus] = useState<NotificationStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    api.getNotifications().then(setStatus).catch(err => {
      setFailed(true);
      setMessage(String(err));
    });
  }, []);

  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setMessage('');
    setFailed(false);
    try {
      await action();
    } catch (err) {
      setFailed(true);
      setMessage(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return <section className="card" style={{ marginBottom: 0 }} aria-labelledby="notifications-heading">
    <div className="card-title"><Bell size={14} color="var(--text-warning)" /><span id="notifications-heading">Notifications</span></div>
    <p className="rule-editor-note">Enabled events appear in NetPilot. Desktop banners also depend on system permissions and Do Not Disturb.</p>
    <div className="notification-options">
      {topics.map(([key, label]) => <label key={key}>
        <input type="checkbox" checked={status?.preferences[key] ?? false} disabled={busy || !status} onChange={e => {
          if (!status) return;
          const preferences = { ...status.preferences, [key]: e.target.checked };
          void run(async () => setStatus(await api.saveNotifications(preferences)));
        }} /> {label}
      </label>)}
    </div>
    <div className="mobile-actions">
      <button className="btn btn-secondary btn-sm" disabled={busy || !status} onClick={() => run(async () => {
        const result = await api.sendTestNotification();
        if (result.desktop === 'unavailable') {
          setFailed(true);
          setMessage(`In-app test delivered. Desktop delivery failed. ${result.desktopError || ''}`);
        } else {
          setMessage(result.desktop === 'off'
            ? 'In-app test delivered. Desktop banners are off.'
            : 'In-app test delivered and submitted to the desktop notification service.');
        }
      })}><Bell size={12} /> Send test</button>
      <button className="btn btn-secondary btn-sm" disabled={busy} onClick={() => run(() => api.openNotificationSettings())}>
        <ExternalLink size={12} /> System notification settings
      </button>
    </div>
    {message && <p role={failed ? 'alert' : 'status'} className="rule-editor-note" style={{ color: failed ? 'var(--text-danger)' : 'var(--text-success)', marginTop: 8 }}>{message}</p>}
  </section>;
}

export function NotificationBanner() {
  const [latest, setLatest] = useState<NotificationStatus['latest']>();
  const [dismissed, setDismissed] = useState('');
  useEffect(() => {
    let cancelled = false;
    const load = () => api.getNotifications().then(status => {
      if (!cancelled) setLatest(status.latest);
    }).catch(() => {});
    void load();
    const timer = setInterval(load, 2000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, []);

  if (!latest || latest.at === dismissed) return null;
  return <div className="notification-banner">
    <div role="status"><strong>{latest.title}</strong><p>{latest.body}</p></div>
    <button type="button" className="btn btn-secondary btn-sm" aria-label="Dismiss notification" onClick={() => setDismissed(latest.at)}><X size={14} /></button>
  </div>;
}
