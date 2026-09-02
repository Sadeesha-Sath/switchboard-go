export interface UsageWindow {
  status: string;
  percent: number;
  resetsAt?: string;
}

export interface SummaryWindow {
  average_percent: number;
  total_remaining_percent: number;
  min_percent: number;
  max_percent: number;
  earliest_reset_at?: string;
}

export interface UsageSummaryPool {
  rolling: SummaryWindow;
  weekly: SummaryWindow;
  monthly: SummaryWindow;
}

export interface UsageSummary {
  total_keys: number;
  available_keys: number;
  exhausted_keys: number;
  active_sessions: number;
  routing_strategy: string;
  proactive_threshold_percent: number;
  pool_usage?: UsageSummaryPool;
}

export interface PerKeyUsage {
  index: number;
  key_hint?: string;
  state: string;
  priority: number;
  weight: number;
  current: boolean;
  eligible: boolean;
  retry_after_seconds?: number;
  rolling: UsageWindow;
  weekly: UsageWindow;
  monthly: UsageWindow;
  last_checked_at?: string;
  error?: string;
}

export interface AggregatedUsageResponse {
  rolling: UsageWindow;
  weekly: UsageWindow;
  monthly: UsageWindow;
  summary: UsageSummary;
  keys: PerKeyUsage[];
}

export interface PerKeyStatus {
  index: number;
  key_hint?: string;
  state: string;
  priority: number;
  weight: number;
  last_429_time?: string;
  current: boolean;
  eligible: boolean;
  retry_after_seconds?: number;
}

export interface StatusResponse {
  current_key_index: number;
  keys: PerKeyStatus[];
  retry_exhausted_after_seconds: number;
  note: string;
}

export interface HTTPRequestMetric {
  endpoint: string;
  method: string;
  status: number;
  count: number;
}

export interface HTTPDurationMetric {
  endpoint: string;
  method: string;
  duration_seconds_sum: number;
  duration_seconds_count: number;
}

export interface UpstreamRequestMetric {
  key_index: number;
  priority: number;
  status: number;
  count: number;
  duration_seconds_sum: number;
  duration_seconds_count: number;
}

export interface KeyExhaustionMetric {
  key_index: number;
  count: number;
}

export interface KeySwitchMetric {
  from_key: number;
  to_key: number;
  reason: string;
  count: number;
}

export interface ValidateKeyResult {
  index: number;
  state: string;
  status: number;
  error?: string;
}

export interface ValidateKeysResponse {
  results: ValidateKeyResult[];
}

export interface MetricsSnapshot {
  generated_at: string;
  http_requests: HTTPRequestMetric[];
  http_durations: HTTPDurationMetric[];
  upstream_requests: UpstreamRequestMetric[];
  key_exhaustions: KeyExhaustionMetric[];
  key_switches: KeySwitchMetric[];
  active_sessions: number;
  model_aliases?: Record<string, string>;
}

export interface WorkspaceModelRow {
  model: string;
  name: string;
  cost: number;
  quota_cost: number;
  multiplier?: number;
  contribution_percent: number;
  estimated: boolean;
}

export interface WorkspaceWindowSnapshot {
  status: string;
  usage_usd: number;
  limit_usd: number;
  usage_percent: number;
  reset_in_sec: number;
  rows: WorkspaceModelRow[];
}

export interface WorkspaceStatus {
  id: string;
  name: string;
  windows: Record<string, WorkspaceWindowSnapshot>;
  error?: string;
}

export interface WorkspaceUsageSnapshot {
  enabled: boolean;
  updated_at?: string;
  error?: string;
  workspaces: WorkspaceStatus[];
}
