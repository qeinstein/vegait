import React from 'react';
import { formatNumber } from '../hooks';

/**
 * LoggerStats — health of the async logging pipeline that drains request
 * telemetry to PostgreSQL. Dropped/errors turn amber/red when non-zero.
 */
export default function LoggerStats({ stats, loading }) {
  if (loading && !stats) return null;
  const s = stats || {};

  const items = [
    { label: 'Logged to DB', value: formatNumber(s.total_logged), color: 'var(--good)' },
    { label: 'Dropped (buffer full)', value: formatNumber(s.total_dropped), color: s.total_dropped > 0 ? 'var(--warning)' : 'var(--text-primary)' },
    { label: 'Flush Errors', value: formatNumber(s.total_flush_errors), color: s.total_flush_errors > 0 ? 'var(--critical)' : 'var(--text-primary)' },
  ];

  return (
    <div className="logger fade">
      {items.map((i) => (
        <div key={i.label} className="logger__cell">
          <div className="logger__value" style={{ color: i.color }}>{i.value}</div>
          <div className="logger__label">{i.label}</div>
        </div>
      ))}
    </div>
  );
}
