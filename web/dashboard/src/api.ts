import type {
  AggregatedUsageResponse,
  MetricsSnapshot,
  ProxyConfigPatch,
  ProxyConfigResponse,
  StatusResponse,
  ValidateKeysResponse,
  WorkspaceUsageSnapshot,
} from './types';

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export function normalizeBase(base: string): string {
  return base.trim().replace(/\/+$/, '');
}

function authHeaders(apiKey: string): Record<string, string> {
  return apiKey ? { Authorization: `Bearer ${apiKey}` } : {};
}

async function getJSON<T>(base: string, path: string, apiKey: string): Promise<T> {
  const res = await fetch(`${normalizeBase(base)}${path}`, { headers: authHeaders(apiKey) });
  if (res.status === 401) {
    throw new ApiError(401, 'Invalid or missing proxy API key');
  }
  if (!res.ok) {
    throw new ApiError(res.status, `Request failed with status ${res.status}`);
  }
  return (await res.json()) as T;
}

async function postJSON<T>(base: string, path: string, apiKey: string, body?: unknown): Promise<T> {
  const res = await fetch(`${normalizeBase(base)}${path}`, {
    method: 'POST',
    headers: { ...authHeaders(apiKey), 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 401) {
    throw new ApiError(401, 'Invalid or missing proxy API key');
  }
  if (!res.ok) {
    throw new ApiError(res.status, `Request failed with status ${res.status}`);
  }
  return (await res.json()) as T;
}

export async function fetchUsage(
  base: string,
  apiKey: string,
  refresh = false,
): Promise<AggregatedUsageResponse> {
  return getJSON<AggregatedUsageResponse>(base, refresh ? '/usage?refresh=true' : '/usage', apiKey);
}

export async function fetchStatus(base: string, apiKey: string): Promise<StatusResponse> {
  return getJSON<StatusResponse>(base, '/admin/status', apiKey);
}

export async function fetchMetrics(base: string): Promise<MetricsSnapshot> {
  // Metrics data is unauthenticated on the server (parity with GET /metrics).
  return getJSON<MetricsSnapshot>(base, '/dashboard/api/metrics.json', '');
}

export async function validateKeys(base: string, apiKey: string): Promise<ValidateKeysResponse> {
  return postJSON<ValidateKeysResponse>(base, '/admin/validate-keys', apiKey);
}

export async function resetKey(base: string, apiKey: string, index: number): Promise<StatusResponse> {
  return postJSON<StatusResponse>(base, '/admin/reset-key', apiKey, { index });
}

export async function resetAllKeys(base: string, apiKey: string): Promise<StatusResponse> {
  return postJSON<StatusResponse>(base, '/admin/reset-all-keys', apiKey);
}

export async function reloadConfig(base: string, apiKey: string): Promise<void> {
  await postJSON(base, '/admin/reload', apiKey);
}

export async function fetchWorkspaceUsage(
  base: string,
  apiKey: string,
): Promise<WorkspaceUsageSnapshot> {
  return getJSON<WorkspaceUsageSnapshot>(base, '/admin/workspace-usage', apiKey);
}

export async function fetchProxyConfig(
  base: string,
  apiKey: string,
): Promise<ProxyConfigResponse> {
  return getJSON<ProxyConfigResponse>(base, '/admin/config', apiKey);
}

export async function patchProxyConfig(
  base: string,
  apiKey: string,
  patch: ProxyConfigPatch,
): Promise<ProxyConfigResponse> {
  const res = await fetch(`${normalizeBase(base)}/admin/config`, {
    method: 'PATCH',
    headers: { ...authHeaders(apiKey), 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
  if (res.status === 401) {
    throw new ApiError(401, 'Invalid or missing proxy API key');
  }
  if (!res.ok) {
    let message = `Request failed with status ${res.status}`;
    try {
      const body = (await res.json()) as { error?: { message?: string } };
      if (body.error?.message) {
        message = body.error.message;
      }
    } catch {
      // Keep the default message when the body is not JSON.
    }
    throw new ApiError(res.status, message);
  }
  return (await res.json()) as ProxyConfigResponse;
}
