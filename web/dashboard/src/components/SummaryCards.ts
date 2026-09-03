import type { AggregatedUsageResponse } from '../types';
import { esc } from '../utils';

export function renderServerInfo(usage: AggregatedUsageResponse, baseUrl: string): string {
  const s = usage.summary;
  const host = baseUrl || window.location.host || '127.0.0.1:8495';
  const cleanHost = host.replace(/^https?:\/\//, '');
  const strat = s.routing_strategy || 'session_sticky';
  const thresh = s.proactive_threshold_percent || 95;
  return `${esc(cleanHost)} · ${esc(strat)} · proactive threshold ${thresh}%`;
}
