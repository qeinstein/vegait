import React from 'react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts';
import ChartTooltip from './ChartTooltip';
import { COLORS } from '../theme';
import { formatNumber } from '../hooks';

/**
 * LiveThroughput — per-minute stacked area of allowed vs rate-limited requests.
 * This is where a running load test shows up in near real time.
 */
export default function LiveThroughput({ data, loading, minutes }) {
  const points = data || [];
  const totals = points.reduce((a, p) => a + p.total_requests, 0);

  return (
    <div className="panel fade">
      <div className="panel__head">
        <div>
          <div className="panel__title">Throughput</div>
          <div className="panel__meta">per-minute · last {minutes}m</div>
        </div>
        <div className="legend">
          <span className="legend__item"><span className="legend__swatch" style={{ background: COLORS.allowed }} />Allowed</span>
          <span className="legend__item"><span className="legend__swatch" style={{ background: COLORS.blocked }} />Rate-limited</span>
        </div>
      </div>

      {loading && !data ? (
        <div className="loading"><div className="spinner" />Loading…</div>
      ) : points.length === 0 ? (
        <div className="empty">No traffic in this window yet.<br />Run <span className="mono">./loadtest.sh</span> or <span className="mono">./demo.sh</span> to generate requests.</div>
      ) : (
        <div className="chart-wrap chart-wrap--tall">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={points} margin={{ top: 6, right: 12, left: -6, bottom: 0 }}>
              <defs>
                <linearGradient id="thrAllowed" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={COLORS.allowed} stopOpacity={0.5} />
                  <stop offset="100%" stopColor={COLORS.allowed} stopOpacity={0.03} />
                </linearGradient>
                <linearGradient id="thrBlocked" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={COLORS.blocked} stopOpacity={0.5} />
                  <stop offset="100%" stopColor={COLORS.blocked} stopOpacity={0.03} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke={COLORS.grid} vertical={false} />
              <XAxis dataKey="label" tick={{ fill: COLORS.axis, fontSize: 11 }} axisLine={{ stroke: COLORS.grid }} tickLine={false} minTickGap={24} />
              <YAxis tick={{ fill: COLORS.axis, fontSize: 11 }} axisLine={false} tickLine={false} width={44} />
              <Tooltip content={<ChartTooltip fmt={formatNumber} />} cursor={{ stroke: COLORS.axis, strokeWidth: 1 }} />
              <Area type="monotone" dataKey="allowed_requests" name="Allowed" stackId="1"
                stroke={COLORS.allowed} strokeWidth={2} fill="url(#thrAllowed)" isAnimationActive={false} />
              <Area type="monotone" dataKey="blocked_requests" name="Rate-limited" stackId="1"
                stroke={COLORS.blocked} strokeWidth={2} fill="url(#thrBlocked)" isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
      <div className="panel__meta" style={{ marginTop: 8 }}>{formatNumber(totals)} requests in window</div>
    </div>
  );
}
