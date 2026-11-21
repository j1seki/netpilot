export const volumeUnits = ['MB', 'GB', 'MiB', 'GiB'] as const;
export const rateUnits = ['Kbps', 'Mbps', 'Gbps', 'MB/s'] as const;

export type VolumeUnit = typeof volumeUnits[number];
export type RateUnit = typeof rateUnits[number];

const bytesPerUnit: Record<VolumeUnit, number> = { MB: 1e6, GB: 1e9, MiB: 2 ** 20, GiB: 2 ** 30 };
const rateMultiplier: Record<RateUnit, number> = { Kbps: 1000, Mbps: 1, Gbps: 0.001, 'MB/s': 0.125 };

export function volumeValue(mib: number, unit: VolumeUnit) {
  return mib * bytesPerUnit.MiB / bytesPerUnit[unit];
}

export function rateValue(mbps: number, unit: RateUnit) {
  return mbps * rateMultiplier[unit];
}

export function formatMetric(value: number) {
  return value > 0 && value < 0.01 ? '<0.01' : value.toLocaleString(undefined, { maximumFractionDigits: 2 });
}
