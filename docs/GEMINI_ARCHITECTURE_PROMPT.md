# Gemini prompt — Architecture diagrams

Paste the block below into **Gemini** (use a model with image generation, e.g.
"Nano Banana"/Imagen in the Gemini app). It asks for **two** images: a **system
architecture diagram** and a **request-flow / sequence diagram**, both in a clean
**black-&-white, n8n-workflow style**. Everything Gemini needs to be accurate is
included, so you don't have to explain the repo.

> Tip: if the first image is close but a label is off, reply "keep the layout,
> fix only <X>" so it edits rather than regenerating from scratch.

---

## PROMPT — copy everything below this line

You are a senior software architect and diagram designer. Generate **two separate,
production-quality images** for the README of a backend system. Use a clean,
minimalist **black-and-white "n8n workflow" aesthetic**:

- **Monochrome only** — white/off-white canvas (#ffffff / #fafafa) with a **subtle
  light-grey dot-grid background**. All nodes, text, borders, and connectors in
  **black and shades of grey** — no color anywhere.
- **Nodes are n8n-style cards:** white rounded rectangles (~12px radius) with a
  thin 1.5px black/dark-grey border and a soft grey drop-shadow; a small
  **monochrome line-icon** in the top-left of each card; a bold node title and
  smaller grey subtitle text.
- **Connectors are smooth curved (bezier) lines** in dark grey with small
  arrowheads, flowing left→right, entering/leaving cards at rounded connection
  dots — exactly like the n8n canvas. Since there is no color, distinguish flow
  types with **line style + short text labels on the line**: solid line = request
  path, **dashed line = asynchronous / non-blocking**.
- Modern sans-serif (Inter/Helvetica), generous spacing, high contrast, crisp and
  legible, no clutter. Landscape 16:10. Do not misspell any label.

The system is a **"Global Rate Limiter as a Service"** — a high-availability
distributed API gateway. Tech stack: **Go** (backend), **Redis 7** (rate-limit
state), **PostgreSQL 16** (analytics/billing logs), **React** (dashboard),
**Docker Compose** (orchestration).

### IMAGE 1 — System Architecture (n8n-style node graph)

Lay out as connected n8n-style nodes, left → right:

1. **Left — Client Microservices (N instances):** three stacked node cards labeled
   "Service A — 100 req/min", "Service B — 5000 req/min", "Service C … n". Curved
   connectors from these into the gateway, labeled on the line "HTTP + X-Client-ID".
2. **Center — Rate Limiter Gateway · Cluster (stateless):** one large container
   node (or grouped frame) holding **two identical gateway instance nodes**
   ("Gateway :8080 · Instance 1", "Gateway :8080 · Instance 2") to convey
   horizontal scaling, with the note "Any instance serves any request — limits
   stay accurate via shared Redis state." Inside, show the numbered request
   pipeline as small stacked steps:
   - 1. Extract X-Client-ID · look up quota
   - 2. Atomic sliding-window check (Redis Lua) — < few ms
   - 3. Allowed → reverse-proxy to third-party API
   - · Rejected → 429 + Retry-After
   - 4. Measure latency → async batched log to PostgreSQL
   Also note "Admin API + dashboard on :8081". Below the pipeline, a distinct
   **"Fail-safe (circuit breaker)" callout node** (dashed border, small warning
   line-icon): "Redis unreachable → fail-open to a local in-memory token bucket.
   No total outage."
3. **Right — State & Analytics:** a **Redis 7** node ("Sorted-set sliding window
   + Lua · atomic, cluster-wide accuracy"), a **PostgreSQL 16** node ("Request
   logs · billing · analytics · percentiles p50/p95/p99"), and a **Mock
   Third-Party API :9090** node ("variable latency 10–200ms, 5% errors"). Curved
   connectors from the gateway: solid "check" → Redis, **dashed "async log"** →
   PostgreSQL, solid "proxy" → Mock API.
4. **Bottom — Observability:** a **React Dashboard** node ("served by admin :8081
   · live throughput & latency, KPI billboards, latency percentiles, daily trend,
   per-client filters") with a **dashed** connector "reads analytics" up to the
   gateway/PostgreSQL.

### IMAGE 2 — Request Flow (sequence diagram, same b/w n8n style)

A clean left-to-right **sequence diagram** with these lifelines, in order:
**Client → Gateway → Redis → Third-Party API → PostgreSQL (async)**. Each actor is
an n8n-style card header with a vertical grey lifeline; messages are horizontal
arrows with labels. Show the "allowed" and "rate-limited" paths:

- Client → Gateway: `GET /proxy/... (X-Client-ID)`
- Gateway → Redis: atomic sliding-window `EVALSHA` check (note "< few ms")
- **Allowed branch:** Redis → Gateway `OK (count < limit)`; Gateway → Third-Party
  API `forward request`; Third-Party API → Gateway `200 + body (measure latency)`;
  Gateway → Client `200 + X-Upstream-Latency-Ms`; Gateway ⇢ PostgreSQL
  `async batched log` (**dashed** = non-blocking).
- **Rate-limited branch:** Redis → Gateway `DENY (count ≥ limit)`; Gateway →
  Client `429 + Retry-After`; Gateway ⇢ PostgreSQL `async log` (**dashed**).
- Add a small side note: "If Redis is down → circuit breaker → local token bucket
  (fail-open)."

Keep it strictly black-and-white, uncluttered, and readable at README width.
Export each image as a separate high-resolution PNG.

## END OF PROMPT

---

## Note

The rendered diagrams generated from this prompt ship in the repo root as
[`architecture.png`](../architecture.png) (Image 1) and
[`sequence.png`](../sequence.png) (Image 2), and are embedded in the README.
