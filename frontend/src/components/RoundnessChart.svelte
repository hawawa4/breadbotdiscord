<script>
  import uPlot from 'uplot'
  import 'uplot/dist/uPlot.min.css'
  import { formatDate } from '../lib/format.js'

  // history: [{ index, roundness, ... }], newest first (index 1 = most recent).
  // onSelect (optional): called with the clicked point's history item.
  let { history, onSelect = null } = $props()

  let el
  let plot
  // ordered = history sorted oldest → newest, so uPlot data index i maps back
  // to ordered[i] for click handling.
  let ordered = []

  function cssVar(name, fallback) {
    const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
    return v || fallback
  }

  function build(width) {
    if (plot) {
      plot.destroy()
      plot = null
    }
    if (!history || history.length === 0) return

    // Plot oldest → newest so the line reads left-to-right over time.
    ordered = [...history].sort((a, b) => b.index - a.index)

    // Use real timestamps for the x-axis when every point has one (created_at_ms
    // is unix ms; uPlot's time scale wants seconds). Fall back to a synthetic
    // 1..n index for older rows not yet backfilled with a timestamp.
    const haveTime = ordered.length > 0 && ordered.every((p) => p.created_at_ms != null)
    const xs = haveTime
      ? ordered.map((p) => Number(p.created_at_ms) / 1000)
      : ordered.map((_, i) => i + 1)
    const ys = ordered.map((p) => p.roundness * 100)

    const accent = cssVar('--accent-2', '#008080')
    const text = cssVar('--text-muted', '#7a6f5f')
    const grid = cssVar('--border', '#e6dccb')

    plot = new uPlot(
      {
        width,
        height: 320,
        // Snap the cursor to the nearest data point (no free-floating x line);
        // hovering "locks" onto discrete points rather than continuous space.
        cursor: {
          y: false,
          points: { size: 11 },
          // dataIdx always returns the series' own nearest index, so the
          // highlighted cursor point sits exactly on a plotted point.
          dataIdx: (u, seriesIdx, hoveredIdx) => hoveredIdx,
        },
        // x is a real time scale only when we have timestamps; otherwise it's a
        // plain numeric index (time: false) so uPlot doesn't format 1..n as dates.
        scales: { x: { time: haveTime }, y: { range: [0, 100] } },
        hooks: {
          // Only register hover/click when the pointer is actually near a
          // plotted point (within HIT_RADIUS px), not anywhere on the plot.
          ready: [
            (u) => {
              if (!onSelect) return
              const HIT_RADIUS = 14

              // nearestHit returns the data index within HIT_RADIUS of the
              // pointer, or null. Uses the snapped cursor idx + pixel distance.
              const nearestHit = () => {
                const i = u.cursor.idx
                if (i == null || !ordered[i]) return null
                const px = u.valToPos(u.data[0][i], 'x')
                const py = u.valToPos(u.data[1][i], 'y')
                const dx = u.cursor.left - px
                const dy = u.cursor.top - py
                return dx * dx + dy * dy <= HIT_RADIUS * HIT_RADIUS ? i : null
              }

              u.over.addEventListener('mousemove', () => {
                u.over.style.cursor = nearestHit() != null ? 'pointer' : 'default'
              })
              u.over.addEventListener('click', () => {
                const i = nearestHit()
                if (i != null) onSelect(ordered[i])
              })
            },
          ],
        },
        axes: [
          {
            stroke: text,
            grid: { stroke: grid, width: 1 },
            ticks: { stroke: grid },
            // Force day-first tick labels on the time axis (splits are in
            // seconds); uPlot's built-in time formatter is month-first (US).
            // Left undefined on the index axis so uPlot renders plain numbers.
            values: haveTime ? (u, splits) => splits.map((v) => formatDate(v * 1000)) : undefined,
          },
          {
            stroke: text,
            grid: { stroke: grid, width: 1 },
            ticks: { stroke: grid },
            values: (u, splits) => splits.map((v) => v + '%'),
          },
        ],
        series: [
          // x series: a date (time-based) or the synthetic sample index.
          haveTime
            ? {
                label: 'date',
                value: (u, v) => (v == null ? '—' : formatDate(v * 1000)),
              }
            : { label: 'n' },
          {
            label: 'roundness',
            stroke: accent,
            width: 2.5,
            points: { show: true, size: 7, stroke: accent, fill: accent },
            value: (u, v) => (v == null ? '—' : v.toFixed(1) + '%'),
          },
        ],
      },
      [xs, ys],
      el,
    )
  }

  $effect(() => {
    // Rebuild on history change or container resize.
    const ro = new ResizeObserver((entries) => {
      const w = entries[0].contentRect.width
      if (w > 0) build(w)
    })
    ro.observe(el)
    return () => {
      ro.disconnect()
      if (plot) plot.destroy()
    }
  })
</script>

<div class="chart" bind:this={el}></div>

<style>
  .chart {
    width: 100%;
  }
</style>
