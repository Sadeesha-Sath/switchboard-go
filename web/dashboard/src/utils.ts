export function esc(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

export function fmtPct(v: number | undefined): string {
  if (v === undefined || Number.isNaN(v)) {
    return '—';
  }
  return `${Math.round(v * 10) / 10}%`;
}

export function usageClass(percent: number | undefined, threshold: number): string {
  if (percent === undefined || Number.isNaN(percent)) {
    return 'ok';
  }
  if (percent >= threshold) {
    return 'critical';
  }
  if (percent >= 70) {
    return 'warn';
  }
  return 'ok';
}

export function fmtDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) {
    return `${h}h ${m}m`;
  }
  if (m > 0) {
    return `${m}m ${sec}s`;
  }
  return `${sec}s`;
}

export function fmtCountdown(iso: string | undefined, now: number = Date.now()): string {
  if (!iso) {
    return '—';
  }
  const t = Date.parse(iso);
  if (Number.isNaN(t)) {
    return '—';
  }
  const diff = (t - now) / 1000;
  if (diff <= 0) {
    return 'reset due';
  }
  return `in ${fmtDuration(diff)}`;
}

export function fmtTimeAgo(iso: string | undefined, now: number = Date.now()): string {
  if (!iso) {
    return 'never';
  }
  const t = Date.parse(iso);
  if (Number.isNaN(t)) {
    return '—';
  }
  const diff = (now - t) / 1000;
  if (diff < 0) {
    return 'just now';
  }
  if (diff < 60) {
    return 'just now';
  }
  return `${fmtDuration(diff)} ago`;
}

export function fmtLocalTime(iso: string | undefined): string {
  if (!iso) {
    return '—';
  }
  const t = Date.parse(iso);
  if (Number.isNaN(t)) {
    return '—';
  }
  return new Date(t).toLocaleTimeString();
}

export function fmtAvgMs(sumSeconds: number, count: number): string {
  if (count <= 0) {
    return '—';
  }
  return `${Math.round((sumSeconds / count) * 1000)}ms`;
}
