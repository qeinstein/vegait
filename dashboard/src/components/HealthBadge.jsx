import React from 'react';

/**
 * HealthBadge — system status pill. Status is conveyed by icon + text label
 * (not color alone), per accessibility guidance.
 */
export default function HealthBadge({ health, loading }) {
  if (loading && !health) {
    return (
      <div className="health">
        <div className="spinner" style={{ width: 12, height: 12, borderWidth: 2 }} />
        <span>Checking…</span>
      </div>
    );
  }

  const isHealthy = health?.status === 'healthy';
  const variant = isHealthy ? 'healthy' : 'degraded';

  return (
    <div className={`health health--${variant}`} title="Postgres + Redis connectivity">
      <span className="health__dot" />
      <span>{isHealthy ? '✓ Operational' : '⚠ Degraded'}</span>
      {health && (
        <span className="health__detail">
          PG {health.postgres ? 'up' : 'down'} · Redis {health.redis ? 'up' : 'down'}
        </span>
      )}
    </div>
  );
}
