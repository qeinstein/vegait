import React from 'react';

/**
 * ClientsTable — the configured per-client rate limits.
 */
export default function ClientsTable({ clients, loading }) {
  return (
    <div className="panel fade">
      <div className="panel__head">
        <div className="panel__title">Configured Clients</div>
        <div className="panel__meta">{clients.length} client{clients.length === 1 ? '' : 's'}</div>
      </div>

      {loading && clients.length === 0 ? (
        <div className="loading"><div className="spinner" /></div>
      ) : clients.length === 0 ? (
        <div className="empty">No clients configured.</div>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Client ID</th>
              <th className="num">Rate Limit</th>
              <th className="num">Window</th>
              <th className="num">Effective</th>
            </tr>
          </thead>
          <tbody>
            {clients.map((c) => (
              <tr key={c.client_id}>
                <td className="client-id mono">{c.client_id}</td>
                <td className="num"><span className="pill">{c.rate_limit} req</span></td>
                <td className="num">{c.window_seconds}s</td>
                <td className="num" style={{ color: 'var(--text-secondary)' }}>
                  {(c.rate_limit / c.window_seconds * 60).toFixed(0)} req/min
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
