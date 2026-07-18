// Central chart palette. Every data-mark hue here was validated for the dark
// surface (#16202a) with the data-viz palette validator:
//   - allowed(blue) vs blocked(orange): CVD ΔE 26.8  ✓ colorblind-safe
//   - avg(blue) vs p95(magenta):        CVD ΔE 15.9  ✓ colorblind-safe
// Chrome/accent/status colors are UI ink (not series) and follow New Relic's
// dark aesthetic. Keep this file as the single source of truth for chart colors.
export const COLORS = {
  allowed: '#3987e5', // blue  — allowed throughput
  blocked: '#d95926', // orange — rate-limited (429)
  latAvg: '#3987e5',  // blue  — average latency
  latP95: '#d55181',  // magenta — p95 latency

  accent: '#00e5a0',  // New Relic signature green (chrome only)
  good: '#3fb37f',
  warning: '#fab219',
  critical: '#e5484d',

  grid: 'rgba(255,255,255,0.055)',
  axis: '#7a828c',
  ink: '#e6eaf0',
  muted: '#8b95a1',
  surface: '#16202a',
};
