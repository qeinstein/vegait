import React from 'react';

/**
 * ChartTooltip — shared crosshair tooltip for the Recharts time-series.
 * `unit` (e.g. 'ms') and `fmt` let each chart format its own values.
 */
export default function ChartTooltip({ active, payload, label, unit = '', fmt }) {
  if (!active || !payload || payload.length === 0) return null;
  const format = fmt || ((v) => new Intl.NumberFormat().format(v));
  return (
    <div className="tooltip">
      <div className="tooltip__label">{label}</div>
      {payload.map((entry) => (
        <div className="tooltip__row" key={entry.name}>
          <span className="legend__swatch" style={{ background: entry.color }} />
          {entry.name}
          <strong>{format(entry.value)}{unit}</strong>
        </div>
      ))}
    </div>
  );
}
