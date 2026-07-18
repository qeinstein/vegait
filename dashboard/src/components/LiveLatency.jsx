import React from 'react';
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts';
import ChartTooltip from './ChartTooltip';
import { COLORS } from '../theme';

/**
 * LiveLatency — per-minute average and p95 upstream latency (single y-axis,
 * both series in milliseconds — never a dual axis).
 */
export default function LiveLatency({ data, loading, minutes }) {
  const points = data || [];
  const fmtMs = (v) => `${Math.round(v * 10) / 10}`;

  return (
    <div className="panel fade">
      <div className="panel__head">
        <div>
          <div className="panel__title">Latency</div>
          <div className="panel__meta">per-minute · last {minutes}m · ms</div>
        </div>
        <div className="legend">
          <span className="legend__item"><span className="legend__swatch" style={{ background: COLORS.latAvg }} />Avg</span>
          <span className="legend__item"><span className="legend__swatch" style={{ background: COLORS.latP95 }} />p95</span>
        </div>
      </div>

      {loading && !data ? (
        <div className="loading"><div className="spinner" />Loading…</div>
      ) : points.length === 0 ? (
        <div className="empty">No latency samples yet.<br />Allowed requests record upstream latency.</div>
      ) : (
        <div className="chart-wrap chart-wrap--tall">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={points} margin={{ top: 6, right: 12, left: -6, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={COLORS.grid} vertical={false} />
              <XAxis dataKey="label" tick={{ fill: COLORS.axis, fontSize: 11 }} axisLine={{ stroke: COLORS.grid }} tickLine={false} minTickGap={24} />
              <YAxis tick={{ fill: COLORS.axis, fontSize: 11 }} axisLine={false} tickLine={false} width={44}
                tickFormatter={(v) => `${v}`} />
              <Tooltip content={<ChartTooltip unit="ms" fmt={fmtMs} />} cursor={{ stroke: COLORS.axis, strokeWidth: 1 }} />
              <Line type="monotone" dataKey="avg_latency_ms" name="Avg" stroke={COLORS.latAvg}
                strokeWidth={2} dot={false} isAnimationActive={false} />
              <Line type="monotone" dataKey="p95_latency_ms" name="p95" stroke={COLORS.latP95}
                strokeWidth={2} dot={false} isAnimationActive={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
