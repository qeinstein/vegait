import { useState, useEffect, useCallback } from 'react';

/**
 * Custom hook for data fetching with loading, error, and refresh support.
 * Automatically refetches when `deps` change.
 */
export function useAPI(fetchFn, deps = []) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const refetch = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchFn()
      .then(setData)
      .catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }, deps);

  useEffect(() => {
    refetch();
  }, [refetch]);

  return { data, loading, error, refetch };
}

/**
 * Formats a number with locale-aware separators.
 */
export function formatNumber(n) {
  if (n === null || n === undefined) return '—';
  return new Intl.NumberFormat().format(n);
}

/**
 * Formats milliseconds to a readable string.
 */
export function formatLatency(ms) {
  if (ms === null || ms === undefined) return '—';
  if (ms < 1) return '<1ms';
  return `${Math.round(ms * 100) / 100}ms`;
}
