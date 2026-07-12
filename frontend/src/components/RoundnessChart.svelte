<script>
  import uPlot from 'uplot'
  import 'uplot/dist/uPlot.min.css'

  // history: [{ index, roundness }], newest first (index 1 = most recent).
  let { history } = $props()

  let el
  let plot

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
    const ordered = [...history].sort((a, b) => b.index - a.index)
    const xs = ordered.map((_, i) => i + 1)
    const ys = ordered.map((p) => p.roundness * 100)

    const accent = cssVar('--accent-2', '#008080')
    const text = cssVar('--text-muted', '#7a6f5f')
    const grid = cssVar('--border', '#e6dccb')

    plot = new uPlot(
      {
        width,
        height: 320,
        cursor: { y: false },
        scales: { y: { range: [0, 100] } },
        axes: [
          { stroke: text, grid: { stroke: grid, width: 1 }, ticks: { stroke: grid } },
          {
            stroke: text,
            grid: { stroke: grid, width: 1 },
            ticks: { stroke: grid },
            values: (u, splits) => splits.map((v) => v + '%'),
          },
        ],
        series: [
          { label: 'n' },
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
