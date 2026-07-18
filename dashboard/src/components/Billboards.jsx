import React from 'react';
import { formatNumber } from '../hooks';
import { COLORS } from '../theme';

/**
 * Billboards — New Relic-style KPI tiles. A big tabular number per metric with
 * a small color accent (identity is reinforced by the label text, never color
 * alone). Covers throughput, pass/block rates, and the full latency spread.
 */
export default function Billboards({ summary, loading }) {
  const s = summary || {};
  const total = s.total_requests || 0;
  const passRate = total ? (s.allowed_requests / total) * 100 : 0;
  const blockRate = total ? (s.blocked_requests / total) * 100 : 0;

  const tiles = [
    {
      label: 'Total Requests', accent: COLORS.accent,
      value: formatNumber(total), sub: 'in selected window',
    },
    {
      label: 'Success Rate', accent: COLORS.good,
      value: total ? `${passRate.toFixed(1)}` : '—', unit: total ? '%' : '',
      sub: <><strong>{formatNumber(s.allowed_requests)}</strong> allowed</>,
    },
    {
      label: 'Rate-Limited', accent: COLORS.blocked,
      value: formatNumber(s.blocked_requests),
      sub: total ? <><strong>{blockRate.toFixed(1)}%</strong> of traffic (429)</> : '429 responses',
    },
    {
      label: 'Avg Latency', accent: COLORS.latAvg,
      value: fmt(s.avg_latency_ms), unit: 'ms', sub: 'mean upstream time',
    },
    {
      label: 'p50 Latency', accent: COLORS.latAvg,
      value: fmt(s.p50_latency_ms), unit: 'ms', sub: 'median',
    },
    {
      label: 'p95 Latency', accent: COLORS.latP95,
      value: fmt(s.p95_latency_ms), unit: 'ms', sub: '95th percentile',
    },
    {
      label: 'p99 Latency', accent: COLORS.latP95,
      value: fmt(s.p99_latency_ms), unit: 'ms', sub: '99th percentile',
    },
  ];

  return (
    <div className="billboards">
      {tiles.map((t) => (
        <div key={t.label} className="billboard fade">
          <div className="billboard__label">
            <span className="billboard__accent" style={{ background: t.accent }} />
            {t.label}
          </div>
          <div className="billboard__value">
            {loading && !summary ? '—' : t.value}
            {t.unit && <span className="billboard__unit">{t.unit}</span>}
          </div>
          <div className="billboard__sub">{t.sub}</div>
        </div>
      ))}
    </div>
  );
}

// Latency values arrive as numbers; show one decimal, or em dash when absent.
function fmt(ms) {
  if (ms === null || ms === undefined) return '—';
  return (Math.round(ms * 10) / 10).toString();
}
