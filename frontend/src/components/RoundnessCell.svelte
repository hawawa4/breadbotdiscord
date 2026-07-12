<script>
  import { pct } from '../lib/format.js'
  let { value } = $props()
</script>

<span class="round" title="roundness">{pct(value)}</span>

<style>
  .round {
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
</style>
