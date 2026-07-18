import React from 'react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts';
import ChartTooltip from './ChartTooltip';
import { COLORS } from '../theme';
import { formatNumber } from '../hooks';

/**
 * TrendChart — daily allowed vs rate-limited requests over the selected window
 * (10 / 15 / 30 days), the long-horizon counterpart to the live view.
 */
export default function TrendChart({ data, loading, days }) {
  const points = data || [];

  return (
    <div className="panel fade">
      <div className="panel__head">
        <div>
          <div className="panel__title">Daily Trend</div>
          <div className="panel__meta">per-day · last {days} days</div>
        </div>
        <div className="legend">
          <span className="legend__item"><span className="legend__swatch" style={{ background: COLORS.allowed }} />Allowed</span>
          <span className="legend__item"><span className="legend__swatch" style={{ background: COLORS.blocked }} />Rate-limited</span>
        </div>
      </div>

      {loading && !data ? (
        <div className="loading"><div className="spinner" />Loading…</div>
      ) : points.length === 0 ? (
        <div className="empty">No historical data yet.<br />Send requests through the proxy to populate this chart.</div>
      ) : (
        <div className="chart-wrap">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={points} margin={{ top: 6, right: 12, left: -6, bottom: 0 }}>
              <defs>
                <linearGradient id="trendAllowed" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={COLORS.allowed} stopOpacity={0.45} />
                  <stop offset="100%" stopColor={COLORS.allowed} stopOpacity={0.03} />
                </linearGradient>
                <linearGradient id="trendBlocked" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={COLORS.blocked} stopOpacity={0.45} />
                  <stop offset="100%" stopColor={COLORS.blocked} stopOpacity={0.03} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke={COLORS.grid} vertical={false} />
              <XAxis dataKey="label" tick={{ fill: COLORS.axis, fontSize: 11 }} axisLine={{ stroke: COLORS.grid }} tickLine={false} minTickGap={24} />
              <YAxis tick={{ fill: COLORS.axis, fontSize: 11 }} axisLine={false} tickLine={false} width={44} />
              <Tooltip content={<ChartTooltip fmt={formatNumber} />} cursor={{ stroke: COLORS.axis, strokeWidth: 1 }} />
              <Area type="monotone" dataKey="allowed_requests" name="Allowed"
                stroke={COLORS.allowed} strokeWidth={2} fill="url(#trendAllowed)" isAnimationActive={false} />
              <Area type="monotone" dataKey="blocked_requests" name="Rate-limited"
                stroke={COLORS.blocked} strokeWidth={2} fill="url(#trendBlocked)" isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
