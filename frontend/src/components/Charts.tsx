import React from 'react';

// ==========================================
// 1. Sparkline (Ultra compact inline trends)
// ==========================================
interface SparklineProps {
  data: number[];
  color?: string;
  width?: number;
  height?: number;
  strokeWidth?: number;
  fill?: boolean;
}

export const Sparkline: React.FC<SparklineProps> = ({
  data,
  color = '#38bdf8',
  width = 100,
  height = 24,
  strokeWidth = 1.5,
  fill = true,
}) => {
  if (!data || data.length < 2) {
    return (
      <svg width={width} height={height} style={{ overflow: 'visible' }}>
        <line x1="0" y1={height / 2} x2={width} y2={height / 2} stroke={color} strokeDasharray="2 2" strokeWidth={1} opacity={0.4} />
      </svg>
    );
  }

  const min = Math.min(...data);
  const max = Math.max(...data) || 1;
  const range = max - min || 1;
  const step = width / (data.length - 1);

  const points = data.map((val, i) => {
    const x = i * step;
    const y = height - ((val - min) / range) * (height - 4) - 2;
    return `${x},${y}`;
  });

  const pathD = `M ${points.join(' L ')}`;
  const areaD = `${pathD} L ${width},${height} L 0,${height} Z`;
  const gradId = `spark-grad-${color.replace('#', '')}-${Math.floor(Math.random() * 10000)}`;

  return (
    <svg width={width} height={height} style={{ overflow: 'visible', display: 'block' }}>
      <defs>
        <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.35} />
          <stop offset="100%" stopColor={color} stopOpacity={0.0} />
        </linearGradient>
      </defs>
      {fill && <path d={areaD} fill={`url(#${gradId})`} />}
      <path d={pathD} fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
};

// ==========================================
// 2. ThroughputGraph (Multi-WAN Live Timeline)
// ==========================================
interface ThroughputGraphProps {
  rxHistory: number[];
  txHistory: number[];
  height?: number;
  rxLabel?: string;
  txLabel?: string;
}

export const ThroughputGraph: React.FC<ThroughputGraphProps> = ({
  rxHistory,
  txHistory,
  height = 110,
  rxLabel = 'Download (Rx)',
  txLabel = 'Upload (Tx)',
}) => {
  const currentRx = rxHistory[rxHistory.length - 1] || 0;
  const currentTx = txHistory[txHistory.length - 1] || 0;

  const maxVal = Math.max(
    ...rxHistory,
    ...txHistory,
    0.5
  );

  const renderPath = (data: number[]) => {
    if (!data || data.length < 2) return '';
    const w = 1000;
    const step = w / (data.length - 1);
    const pts = data.map((v, i) => {
      const x = i * step;
      const y = height - (v / maxVal) * (height - 14) - 7;
      return `${x},${y}`;
    });
    return `M ${pts.join(' L ')}`;
  };

  const rxPath = renderPath(rxHistory);
  const txPath = renderPath(txHistory);
  const rxArea = rxPath ? `${rxPath} L 1000,${height} L 0,${height} Z` : '';

  return (
    <div style={{ position: 'relative', width: '100%', backgroundColor: 'rgba(0,0,0,0.25)', borderRadius: '6px', border: '1px solid var(--border)', padding: '10px 12px 6px 12px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '6px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '14px', fontSize: '11px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span style={{ width: '8px', height: '8px', borderRadius: '50%', backgroundColor: '#38bdf8' }} />
            <span style={{ color: 'var(--text-secondary)' }}>{rxLabel}:</span>
            <span className="font-mono" style={{ color: '#38bdf8', fontWeight: 700 }}>
              {currentRx >= 1 ? currentRx.toFixed(2) : currentRx.toFixed(3)} Mbps
            </span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span style={{ width: '8px', height: '8px', borderRadius: '50%', backgroundColor: '#4ade80' }} />
            <span style={{ color: 'var(--text-secondary)' }}>{txLabel}:</span>
            <span className="font-mono" style={{ color: '#4ade80', fontWeight: 700 }}>
              {currentTx >= 1 ? currentTx.toFixed(2) : currentTx.toFixed(3)} Mbps
            </span>
          </div>
        </div>
        <div className="font-mono" style={{ fontSize: '10px', color: 'var(--text-muted)' }}>
          Scale Peak: {maxVal >= 1 ? maxVal.toFixed(1) : maxVal.toFixed(2)} Mbps
        </div>
      </div>

      <svg viewBox={`0 0 1000 ${height}`} preserveAspectRatio="none" style={{ width: '100%', height: `${height}px`, display: 'block' }}>
        <defs>
          <linearGradient id="live-rx-grad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#38bdf8" stopOpacity={0.3} />
            <stop offset="100%" stopColor="#38bdf8" stopOpacity={0.02} />
          </linearGradient>
        </defs>

        {/* Grid lines */}
        <line x1="0" y1={height * 0.25} x2="1000" y2={height * 0.25} stroke="rgba(255,255,255,0.05)" strokeDasharray="4 4" />
        <line x1="0" y1={height * 0.50} x2="1000" y2={height * 0.50} stroke="rgba(255,255,255,0.05)" strokeDasharray="4 4" />
        <line x1="0" y1={height * 0.75} x2="1000" y2={height * 0.75} stroke="rgba(255,255,255,0.05)" strokeDasharray="4 4" />

        {/* Rx Area & Line */}
        {rxArea && <path d={rxArea} fill="url(#live-rx-grad)" />}
        {rxPath && <path d={rxPath} fill="none" stroke="#38bdf8" strokeWidth="2" strokeLinecap="round" />}

        {/* Tx Line */}
        {txPath && <path d={txPath} fill="none" stroke="#4ade80" strokeWidth="2" strokeLinecap="round" strokeDasharray="6 3" />}
      </svg>
    </div>
  );
};

// ==========================================
// 3. DonutChart (Bandwidth Distribution)
// ==========================================
interface DonutSegment {
  label: string;
  value: number;
  color: string;
}

interface DonutChartProps {
  segments: DonutSegment[];
  size?: number;
  thickness?: number;
  centerTitle?: string;
  centerSub?: string;
}

export const DonutChart: React.FC<DonutChartProps> = ({
  segments,
  size = 110,
  thickness = 16,
  centerTitle,
  centerSub,
}) => {
  const total = segments.reduce((sum, s) => sum + s.value, 0) || 1;
  const radius = (size - thickness) / 2;
  const circumference = 2 * Math.PI * radius;

  let accumulatedPercent = 0;

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
      <div style={{ position: 'relative', width: size, height: size, flexShrink: 0 }}>
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
          {/* Background Ring */}
          <circle
            cx={size / 2}
            cy={size / 2}
            r={radius}
            fill="none"
            stroke="rgba(255,255,255,0.06)"
            strokeWidth={thickness}
          />
          {segments.map((seg, idx) => {
            const percent = seg.value / total;
            const strokeDasharray = `${circumference * percent} ${circumference * (1 - percent)}`;
            const strokeDashoffset = -circumference * accumulatedPercent;
            accumulatedPercent += percent;

            return (
              <circle
                key={idx}
                cx={size / 2}
                cy={size / 2}
                r={radius}
                fill="none"
                stroke={seg.color}
                strokeWidth={thickness}
                strokeDasharray={strokeDasharray}
                strokeDashoffset={strokeDashoffset}
                strokeLinecap="butt"
                transform={`rotate(-90 ${size / 2} ${size / 2})`}
                style={{ transition: 'stroke-dasharray 0.3s ease' }}
              />
            );
          })}
        </svg>

        {/* Center label */}
        {(centerTitle || centerSub) && (
          <div style={{
            position: 'absolute',
            top: 0,
            left: 0,
            width: size,
            height: size,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            pointerEvents: 'none',
          }}>
            {centerTitle && <span className="font-mono" style={{ fontSize: '13px', fontWeight: 700, color: 'white' }}>{centerTitle}</span>}
            {centerSub && <span style={{ fontSize: '9px', color: 'var(--text-muted)' }}>{centerSub}</span>}
          </div>
        )}
      </div>

      {/* Legend */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', flex: 1, minWidth: 0 }}>
        {segments.map((seg, i) => {
          const pct = ((seg.value / total) * 100).toFixed(0);
          return (
            <div key={i} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: '11px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '6px', minWidth: 0 }}>
                <span style={{ width: '8px', height: '8px', borderRadius: '2px', backgroundColor: seg.color, flexShrink: 0 }} />
                <span style={{ color: 'var(--text-secondary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {seg.label}
                </span>
              </div>
              <span className="font-mono" style={{ fontWeight: 600, color: 'white', marginLeft: '6px' }}>
                {pct}%
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
};

// ==========================================
// 4. BarGauge (Quota & Speed Bar)
// ==========================================
interface BarGaugeProps {
  value: number;
  max: number;
  label?: string;
  unit?: string;
  height?: number;
  color?: string;
}

export const BarGauge: React.FC<BarGaugeProps> = ({
  value,
  max,
  label,
  unit = '',
  height = 5,
  color,
}) => {
  const percent = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  const barColor = color || (percent > 90 ? '#ef4444' : percent > 70 ? '#f59e0b' : '#38bdf8');

  return (
    <div style={{ width: '100%' }}>
      {label && (
        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '10.5px', marginBottom: '3px' }}>
          <span style={{ color: 'var(--text-muted)' }}>{label}</span>
          <span className="font-mono" style={{ color: 'var(--text-primary)', fontWeight: 600 }}>
            {value.toFixed(1)} / {max > 0 ? max.toFixed(0) : '∞'} {unit}
          </span>
        </div>
      )}
      <div style={{ width: '100%', height: `${height}px`, backgroundColor: 'rgba(255,255,255,0.08)', borderRadius: `${height}px`, overflow: 'hidden' }}>
        <div style={{ width: `${percent}%`, height: '100%', backgroundColor: barColor, borderRadius: `${height}px`, transition: 'width 0.3s ease' }} />
      </div>
    </div>
  );
};

// ==========================================
// 5. MonthlyBarChart (Side-by-side Rx/Tx Bars)
// ==========================================
interface MonthlyDataPoint {
  month: string;
  rxMb: number;
  txMb: number;
}

interface MonthlyBarChartProps {
  data: MonthlyDataPoint[];
  height?: number;
}

export const MonthlyBarChart: React.FC<MonthlyBarChartProps> = ({ data, height = 90 }) => {
  if (!data || data.length === 0) return null;
  const maxMb = Math.max(...data.map(d => Math.max(d.rxMb, d.txMb)), 50);

  return (
    <div style={{ width: '100%', backgroundColor: 'rgba(0,0,0,0.2)', padding: '10px 14px', borderRadius: '6px', border: '1px solid var(--border)' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px' }}>
        <span style={{ fontSize: '11.5px', fontWeight: 600, color: 'var(--text-secondary)' }}>Monthly Bandwidth Comparison</span>
        <div style={{ display: 'flex', gap: '12px', fontSize: '10.5px' }}>
          <span style={{ display: 'flex', alignItems: 'center', gap: '4px', color: '#38bdf8' }}>
            <span style={{ width: '7px', height: '7px', backgroundColor: '#38bdf8', borderRadius: '1px' }} /> Download (Rx)
          </span>
          <span style={{ display: 'flex', alignItems: 'center', gap: '4px', color: '#4ade80' }}>
            <span style={{ width: '7px', height: '7px', backgroundColor: '#4ade80', borderRadius: '1px' }} /> Upload (Tx)
          </span>
        </div>
      </div>

      <div style={{ display: 'flex', alignItems: 'flex-end', gap: '20px', height: `${height}px`, borderBottom: '1px solid rgba(255,255,255,0.1)', paddingBottom: '4px' }}>
        {data.map((item, idx) => {
          const rxH = Math.max(3, (item.rxMb / maxMb) * (height - 18));
          const txH = Math.max(3, (item.txMb / maxMb) * (height - 18));
          return (
            <div key={idx} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '4px' }}>
              <div style={{ display: 'flex', alignItems: 'flex-end', gap: '4px', height: `${height - 18}px` }}>
                <div
                  title={`Rx: ${item.rxMb.toFixed(1)} MB`}
                  style={{
                    width: '18px',
                    height: `${rxH}px`,
                    backgroundColor: '#38bdf8',
                    borderRadius: '2px 2px 0 0',
                    transition: 'height 0.4s ease',
                  }}
                />
                <div
                  title={`Tx: ${item.txMb.toFixed(1)} MB`}
                  style={{
                    width: '18px',
                    height: `${txH}px`,
                    backgroundColor: '#4ade80',
                    borderRadius: '2px 2px 0 0',
                    transition: 'height 0.4s ease',
                  }}
                />
              </div>
              <span className="font-mono" style={{ fontSize: '10.5px', color: 'var(--text-muted)' }}>
                {item.month}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
};
