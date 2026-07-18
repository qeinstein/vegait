import React from 'react';

/**
 * FilterBar — client selector, the historical trend window (days), and the
 * live view window (minutes). Emits changes to the parent, which owns state.
 */
export default function FilterBar({
  clients, selectedClient, onClientChange,
  days, onDaysChange, liveMinutes, onLiveMinutesChange,
}) {
  const dayOptions = [10, 15, 30];
  const liveOptions = [
    { label: '5m', value: 5 },
    { label: '15m', value: 15 },
    { label: '60m', value: 60 },
  ];

  return (
    <div className="filters">
      <div className="filters__group">
        <span className="filters__label">Client</span>
        <select
          id="filter-client-select"
          className="select"
          value={selectedClient}
          onChange={(e) => onClientChange(e.target.value)}
        >
          <option value="">All clients</option>
          {clients.map((c) => (
            <option key={c.client_id} value={c.client_id}>
              {c.client_id} · {c.rate_limit}/{c.window_seconds}s
            </option>
          ))}
        </select>
      </div>

      <div className="filters__group" style={{ marginLeft: 'auto' }}>
        <span className="filters__label">Live</span>
        <div className="segmented">
          {liveOptions.map((o) => (
            <button
              key={o.value}
              id={`filter-live-${o.value}`}
              className={`seg ${liveMinutes === o.value ? 'seg--active' : ''}`}
              onClick={() => onLiveMinutesChange(o.value)}
            >
              {o.label}
            </button>
          ))}
        </div>
      </div>

      <div className="filters__group">
        <span className="filters__label">Trend</span>
        <div className="segmented">
          {dayOptions.map((d) => (
            <button
              key={d}
              id={`filter-days-${d}`}
              className={`seg ${days === d ? 'seg--active' : ''}`}
              onClick={() => onDaysChange(d)}
            >
              {d}d
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
