const API_BASE = import.meta.env.VITE_API_BASE || '';

/**
 * Centralized API client for all dashboard data fetching.
 * Throws on HTTP errors so callers can handle them uniformly.
 */
async function fetchJSON(path, params = {}) {
  const url = new URL(`${API_BASE}${path}`, window.location.origin);
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== '') {
      url.searchParams.set(k, v);
    }
  });

  const res = await fetch(url.toString());
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`API ${res.status}: ${body}`);
  }
  return res.json();
}

export function fetchHealth() {
  return fetchJSON('/health/deep');
}

export function fetchSummary(clientId, days) {
  return fetchJSON('/api/analytics/summary', { client_id: clientId, days });
}

export function fetchTrend(clientId, days) {
  return fetchJSON('/api/analytics/trend', { client_id: clientId, days });
}

export function fetchLive(clientId, minutes) {
  return fetchJSON('/api/analytics/live', { client_id: clientId, minutes });
}

export function fetchClients() {
  return fetchJSON('/api/clients');
}

export function fetchLoggerStats() {
  return fetchJSON('/api/logger/stats');
}

export function upsertClient(clientId, rateLimit, windowSeconds) {
  return fetch(`${API_BASE}/api/clients/upsert`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      client_id: clientId,
      rate_limit: rateLimit,
      window_seconds: windowSeconds,
    }),
  }).then(res => {
    if (!res.ok) throw new Error(`Upsert failed: ${res.status}`);
    return res.json();
  });
}
