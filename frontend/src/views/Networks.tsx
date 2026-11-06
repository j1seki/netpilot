import React, { useEffect, useState } from 'react';
import { api, InterfaceInfo, CapabilityReport } from '../api/wails';
import { BarGauge } from '../components/Charts';
import { Network, ArrowDown, ArrowUp, Zap, Sliders, ShieldCheck } from 'lucide-react';

export const Networks: React.FC = () => {
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [caps, setCaps] = useState<CapabilityReport | null>(null);

  // Modal state for configuring adapter rate limit & quota
  const [selectedIface, setSelectedIface] = useState<InterfaceInfo | null>(null);
  const [editRxLimit, setEditRxLimit] = useState(0);
  const [editTxLimit, setEditTxLimit] = useState(0);
  const [editQuotaMb, setEditQuotaMb] = useState(0);

  const loadData = async () => {
    const [ifaces, c] = await Promise.all([
      api.getInterfaces(),
      api.getCapabilities(),
    ]);
    setInterfaces(ifaces);
    setCaps(c);
  };

  useEffect(() => {
    loadData().catch((e) => console.error(e));
    const interval = setInterval(() => loadData().catch((e) => console.error(e)), 2000);
    return () => clearInterval(interval);
  }, []);

  const handleSaveCap = async () => {
    if (!selectedIface) return;
    await api.saveAdapterCap({
      interfaceName: selectedIface.name,
      rxLimitMbps: editRxLimit,
      txLimitMbps: editTxLimit,
      dataQuotaMb: editQuotaMb,
      dataUsedMb: selectedIface.dataUsedMb || 0,
      enabled: true,
    });
    setSelectedIface(null);
    loadData().catch((e) => console.error(e));
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      {/* Virtualization banner */}
      {caps?.environment.isVm && (
        <div style={{
          padding: '8px 12px',
          borderRadius: 'var(--radius-sm)',
          backgroundColor: 'rgba(59, 130, 246, 0.08)',
          border: '1px solid rgba(59, 130, 246, 0.25)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontSize: '11.5px',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <ShieldCheck size={15} color="#60a5fa" />
            <span style={{ fontWeight: 600, color: 'white' }}>Live Host Network Environment:</span>
            <span style={{ color: 'var(--text-secondary)' }}>Direct kernel traffic accounting active across host adapters.</span>
          </div>
          <span className="font-mono" style={{ fontSize: '10px', color: '#93c5fd' }}>
            {caps.environment.vmType} ({caps.environment.inferredHostOs})
          </span>
        </div>
      )}

      {/* Grid of Compact Adapter Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: '10px' }}>
        {interfaces.map(iface => {
          const quotaUsed = iface.dataUsedMb || 0;
          const quotaTotal = iface.dataQuotaMb || 0;
          const hasCap = (iface.rxLimitMbps || 0) > 0 || (iface.txLimitMbps || 0) > 0 || quotaTotal > 0;

          return (
            <div key={iface.name} className="card" style={{ marginBottom: 0, display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {/* Header */}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <div style={{ width: '28px', height: '28px', borderRadius: '4px', background: 'rgba(59, 130, 246, 0.15)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <Network size={15} color="#60a5fa" />
                  </div>
                  <div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                      <span style={{ fontSize: '13.5px', fontWeight: 700 }}>{iface.name}</span>
                      <span style={{ fontSize: '9.5px', color: iface.isHardware ? '#60a5fa' : '#94a3b8', background: 'rgba(255,255,255,0.06)', padding: '1px 5px', borderRadius: '3px' }}>
                        {iface.isHardware ? 'Hardware' : 'Virtual / Bridge'}
                      </span>
                    </div>
                    <div className="font-mono" style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
                      {iface.ip || 'No IP Assigned'}
                    </div>
                  </div>
                </div>

                <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  <span className={`status-pill ${iface.isUp ? 'pass' : 'fail'}`} style={{ fontSize: '10px', padding: '2px 6px' }}>
                    <Zap size={10} /> {iface.linkState || (iface.isUp ? 'UP' : 'DOWN')}
                  </span>
                </div>
              </div>

              {/* Instant Throughput Row with Sparklines */}
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px', background: 'rgba(0,0,0,0.2)', padding: '8px 10px', borderRadius: '4px' }}>
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '10.5px', color: '#38bdf8' }}>
                    <ArrowDown size={11} /> Download (Rx)
                  </div>
                  <div className="font-mono" style={{ fontSize: '15px', fontWeight: 700, color: 'white', marginTop: '2px' }}>
                    {(iface.rxMbps || 0).toFixed(2)} <span style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Mbps</span>
                  </div>
                  {iface.rxLimitMbps && iface.rxLimitMbps > 0 ? (
                    <div className="font-mono" style={{ fontSize: '10px', color: '#f59e0b', marginTop: '2px' }}>
                      Cap: {iface.rxLimitMbps} Mbps
                    </div>
                  ) : null}
                </div>

                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '10.5px', color: '#4ade80' }}>
                    <ArrowUp size={11} /> Upload (Tx)
                  </div>
                  <div className="font-mono" style={{ fontSize: '15px', fontWeight: 700, color: 'white', marginTop: '2px' }}>
                    {(iface.txMbps || 0).toFixed(2)} <span style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Mbps</span>
                  </div>
                  {iface.txLimitMbps && iface.txLimitMbps > 0 ? (
                    <div className="font-mono" style={{ fontSize: '10px', color: '#f59e0b', marginTop: '2px' }}>
                      Cap: {iface.txLimitMbps} Mbps
                    </div>
                  ) : null}
                </div>
              </div>

              {/* Rate Limit & Monthly Quota Meters */}
              {hasCap && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', background: 'rgba(255,255,255,0.02)', padding: '6px 8px', borderRadius: '4px' }}>
                  {quotaTotal > 0 && (
                    <BarGauge
                      value={quotaUsed / 1024}
                      max={quotaTotal / 1024}
                      label="Monthly Quota Consumed"
                      unit="GB"
                      height={4}
                    />
                  )}
                  {iface.rxLimitMbps && iface.rxLimitMbps > 0 ? (
                    <BarGauge
                      value={iface.rxMbps || 0}
                      max={iface.rxLimitMbps}
                      label="Rx Rate Cap Load"
                      unit="Mbps"
                      height={4}
                      color="#38bdf8"
                    />
                  ) : null}
                </div>
              )}

              {/* Action Button */}
              <div style={{ display: 'flex', justifyContent: 'flex-end', paddingTop: '2px' }}>
                <button
                  className="btn btn-secondary btn-sm"
                  onClick={() => {
                    setSelectedIface(iface);
                    setEditRxLimit(iface.rxLimitMbps || 0);
                    setEditTxLimit(iface.txLimitMbps || 0);
                    setEditQuotaMb(iface.dataQuotaMb || 0);
                  }}
                  style={{ fontSize: '11px', padding: '4px 8px', display: 'flex', alignItems: 'center', gap: '4px' }}
                >
                  <Sliders size={11} /> Configure Speed Cap
                </button>
              </div>
            </div>
          );
        })}
      </div>

      {/* Speed & Quota Modal */}
      {selectedIface && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: 'rgba(0,0,0,0.7)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 999,
        }}>
          <div style={{
            backgroundColor: 'var(--bg-secondary)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-md)',
            padding: '20px',
            width: '420px',
            boxShadow: '0 20px 40px rgba(0,0,0,0.5)',
          }}>
            <h3 style={{ fontSize: '15px', fontWeight: 600, marginBottom: '6px' }}>
              Speed Limit & Data Cap: {selectedIface.name}
            </h3>
            <p style={{ fontSize: '11.5px', color: 'var(--text-secondary)', marginBottom: '14px' }}>
              Set hardware download/upload caps (Mbps) and monthly bandwidth quota.
            </p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px', marginBottom: '16px' }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                <div>
                  <label style={{ fontSize: '11.5px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Rx Limit (Mbps)
                  </label>
                  <input
                    type="number"
                    min="0"
                    step="5"
                    value={editRxLimit}
                    onChange={e => setEditRxLimit(parseFloat(e.target.value) || 0)}
                    style={{ width: '100%', padding: '6px 10px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '4px', color: 'white', fontSize: '12px' }}
                  />
                </div>

                <div>
                  <label style={{ fontSize: '11.5px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                    Tx Limit (Mbps)
                  </label>
                  <input
                    type="number"
                    min="0"
                    step="5"
                    value={editTxLimit}
                    onChange={e => setEditTxLimit(parseFloat(e.target.value) || 0)}
                    style={{ width: '100%', padding: '6px 10px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '4px', color: 'white', fontSize: '12px' }}
                  />
                </div>
              </div>

              <div>
                <label style={{ fontSize: '11.5px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                  Monthly Data Cap (MB, 0=unlimited)
                </label>
                <input
                  type="number"
                  min="0"
                  step="1000"
                  value={editQuotaMb}
                  onChange={e => setEditQuotaMb(parseFloat(e.target.value) || 0)}
                  style={{ width: '100%', padding: '6px 10px', backgroundColor: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: '4px', color: 'white', fontSize: '12px' }}
                />
              </div>
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button className="btn btn-secondary btn-sm" onClick={() => setSelectedIface(null)}>
                Cancel
              </button>
              <button className="btn btn-primary btn-sm" onClick={handleSaveCap}>
                Save Settings
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
