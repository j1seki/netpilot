import { useEffect, useRef } from 'react';

export type TrafficPoint = { rx: number; tx: number };

function drawSeries(
  ctx: CanvasRenderingContext2D,
  points: TrafficPoint[],
  direction: keyof TrafficPoint,
  color: string,
  capacity: number,
  width: number,
  height: number,
) {
  if (!points.length) return;

  const inset = 4;
  const bottom = height - inset;
  const step = (width - inset * 2) / (capacity - 1);
  const positions = points.map((point, index) => ({
    x: inset + (capacity - points.length + index) * step,
    y: bottom - point[direction] * (height - inset * 2),
  }));
  const first = positions[0];
  const last = positions[positions.length - 1];
  const line = new Path2D();
  line.moveTo(first.x, first.y);
  for (let i = 1; i < positions.length - 1; i++) {
    const point = positions[i];
    const next = positions[i + 1];
    line.quadraticCurveTo(point.x, point.y, (point.x + next.x) / 2, (point.y + next.y) / 2);
  }
  line.lineTo(last.x, last.y);

  ctx.strokeStyle = color;
  ctx.lineWidth = 1.6;
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';
  if (direction === 'rx' && positions.length > 1) {
    const area = new Path2D(line);
    area.lineTo(last.x, bottom);
    area.lineTo(first.x, bottom);
    area.closePath();
    const gradient = ctx.createLinearGradient(0, inset, 0, bottom);
    gradient.addColorStop(0, color);
    gradient.addColorStop(1, 'transparent');
    ctx.globalAlpha = 0.2;
    ctx.fillStyle = gradient;
    ctx.fill(area);
    ctx.globalAlpha = 1;
  }

  ctx.stroke(line);
  ctx.fillStyle = color;
  ctx.beginPath();
  ctx.arc(last.x, last.y, 4, 0, Math.PI * 2);
  ctx.globalAlpha = 0.2;
  ctx.fill();
  ctx.globalAlpha = 1;
  ctx.beginPath();
  ctx.arc(last.x, last.y, 2.4, 0, Math.PI * 2);
  ctx.fill();
}

export function SimpleTrafficChart({ samples, capacity }: { samples: TrafficPoint[]; capacity: number }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const displayed = useRef<TrafficPoint[]>([]);
  const paint = useRef<() => void>(() => {});
  const animate = useRef(0);
  const target = useRef<TrafficPoint[]>([]);

  useEffect(() => {
    const element = canvas.current;
    const ctx = element?.getContext('2d');
    if (!element || !ctx) return;

    const theme = window.matchMedia('(prefers-color-scheme: dark)');
    const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
    let width = 0;
    let height = 0;
    let download = '';
    let upload = '';

    paint.current = () => {
      ctx.clearRect(0, 0, width, height);
      if (width < 8 || height < 8) return;
      drawSeries(ctx, displayed.current, 'rx', download, capacity, width, height);
      drawSeries(ctx, displayed.current, 'tx', upload, capacity, width, height);
    };

    const resize = () => {
      const scale = Math.min(window.devicePixelRatio || 1, 2);
      width = element.clientWidth;
      height = element.clientHeight;
      const pixelWidth = Math.round(width * scale);
      const pixelHeight = Math.round(height * scale);
      if (element.width !== pixelWidth) element.width = pixelWidth;
      if (element.height !== pixelHeight) element.height = pixelHeight;
      ctx.setTransform(scale, 0, 0, scale, 0, 0);
      paint.current();
    };
    const applyPalette = () => {
      const style = getComputedStyle(element);
      download = style.getPropertyValue('--text-info').trim();
      upload = style.getPropertyValue('--accent-warning').trim();
      paint.current();
    };
    const applyMotionPreference = () => {
      if (!motion.matches) return;
      cancelAnimationFrame(animate.current);
      displayed.current = target.current;
      paint.current();
    };

    applyPalette();
    resize();
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(resize);
    observer?.observe(element);
    window.addEventListener('resize', resize);
    // Older WKWebView versions implement the original media query listener API.
    theme.addListener(applyPalette);
    motion.addListener(applyMotionPreference);
    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', resize);
      theme.removeListener(applyPalette);
      motion.removeListener(applyMotionPreference);
      cancelAnimationFrame(animate.current);
    };
  }, [capacity]);

  useEffect(() => {
    cancelAnimationFrame(animate.current);
    const maximum = Math.max(0.001, ...samples.map(point => Math.max(point.rx, point.tx)));
    const next = samples.map(point => ({ rx: point.rx / maximum, tx: point.tx / maximum }));
    const previous = displayed.current;
    target.current = next;

    if (previous.length === next.length && next.every((point, i) => point.rx === previous[i].rx && point.tx === previous[i].tx)) return;
    if (!previous.length || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      displayed.current = next;
      paint.current();
      return;
    }

    const started = performance.now();
    const drawFrame = (now: number) => {
      const progress = Math.min(1, (now - started) / 440);
      const eased = 1 - (1 - progress) ** 3;
      displayed.current = next.map((point, index) => {
        const from = previous[index + previous.length - next.length] ?? previous[0];
        return {
          rx: from.rx + (point.rx - from.rx) * eased,
          tx: from.tx + (point.tx - from.tx) * eased,
        };
      });
      paint.current();
      if (progress < 1) animate.current = requestAnimationFrame(drawFrame);
    };
    animate.current = requestAnimationFrame(drawFrame);
    return () => cancelAnimationFrame(animate.current);
  }, [samples]);

  return <canvas ref={canvas} className="simple-chart" aria-hidden="true" />;
}
