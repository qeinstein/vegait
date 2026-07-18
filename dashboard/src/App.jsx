import React, { useState, useCallback, useEffect } from 'react';
import { useAPI } from './hooks';
import {
  fetchHealth, fetchSummary, fetchTrend, fetchClients, fetchLoggerStats, fetchLive,
} from './api';

import HealthBadge from './components/HealthBadge';
import FilterBar from './components/FilterBar';
import Billboards from './components/Billboards';
import LiveThroughput from './components/LiveThroughput';
import LiveLatency from './components/LiveLatency';
import TrendChart from './components/TrendChart';
import ClientsTable from './components/ClientsTable';
import LoggerStats from './components/LoggerStats';

/**
 * App — Rate Limiter observability dashboard (New Relic-style).
 *
 * Layout:
 *   Top bar (brand · live indicator · health)
 *   Filters (client · live window · trend window)
 *   Overview  — KPI billboards (throughput, rates, latency spread)
 *   Live      — per-minute throughput + latency (updates every few seconds)
 *   History   — daily trend
 *   Pipeline  — async logger stats
 *   Clients   — configured limits
 */
export default function App() {
  const [selectedClient, setSelectedClient] = useState('');
  const [days, setDays] = useState(30);
  const [liveMinutes, setLiveMinutes] = useState(15);

  const health = useAPI(() => fetchHealth(), []);
  const clients = useAPI(() => fetchClients(), []);
  const loggerStats = useAPI(() => fetchLoggerStats(), []);
  const summary = useAPI(
    useCallback(() => fetchSummary(selectedClient, days), [selectedClient, days]),
    [selectedClient, days],
  );
  const trend = useAPI(
    useCallback(() => fetchTrend(selectedClient, days), [selectedClient, days]),
    [selectedClient, days],
  );
  const live = useAPI(
    useCallback(() => fetchLive(selectedClient, liveMinutes), [selectedClient, liveMinutes]),
    [selectedClient, liveMinutes],
  );

  // Fast cadence for the live surfaces; slower for historical/config data.
  useEffect(() => {
    const fast = setInterval(() => {
      health.refetch();
      summary.refetch();
      live.refetch();
      loggerStats.refetch();
    }, 3000);
    const slow = setInterval(() => {
      trend.refetch();
      clients.refetch();
    }, 20000);
    return () => { clearInterval(fast); clearInterval(slow); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedClient, days, liveMinutes]);

  const hasError = summary.error || live.error || trend.error;

  return (
    <div className="app">
      <header className="topbar">
        <div className="topbar__brand">
          <div className="topbar__logo">R</div>
          <div>
            <div className="topbar__title">Rate Limiter</div>
            <div className="topbar__subtitle">Global API Gateway · Observability</div>
          </div>
        </div>
        <div className="topbar__right">
          <span className="live-pill"><span className="live-pill__dot" />Live</span>
          <HealthBadge health={health.data} loading={health.loading} />
        </div>
      </header>

      <FilterBar
        clients={clients.data || []}
        selectedClient={selectedClient}
        onClientChange={setSelectedClient}
        days={days}
        onDaysChange={setDays}
        liveMinutes={liveMinutes}
        onLiveMinutesChange={setLiveMinutes}
      />

      {hasError && (
        <div className="banner">
          ⚠ Error loading data: {summary.error || live.error || trend.error}.
          Ensure the admin server is reachable on port 8081.
        </div>
      )}

      <div className="section-label">Overview · {selectedClient || 'all clients'} · last {days} days</div>
      <Billboards summary={summary.data} loading={summary.loading} />

      <div className="section-label">Live · real-time</div>
      <div className="grid-2">
        <LiveThroughput data={live.data} loading={live.loading} minutes={liveMinutes} />
        <LiveLatency data={live.data} loading={live.loading} minutes={liveMinutes} />
      </div>

      <div className="section-label">History</div>
      <TrendChart data={trend.data} loading={trend.loading} days={days} />

      <div className="section-label">Logging Pipeline</div>
      <LoggerStats stats={loggerStats.data} loading={loggerStats.loading} />

      <div className="section-label">Clients</div>
      <ClientsTable clients={clients.data || []} loading={clients.loading} />
    </div>
  );
}
