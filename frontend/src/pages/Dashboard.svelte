<script>
  import { api } from '../lib/api.js'
  import { href } from '../lib/router.js'
  import { pct } from '../lib/format.js'
  import RoundnessCell from '../components/RoundnessCell.svelte'

  let summary = $state(null)
  let top = $state([])
  let error = $state(null)
  let loading = $state(true)

  async function load() {
    loading = true
    error = null
    try {
      const [s, lb] = await Promise.all([api.summary(), api.leaderboard('max', 5)])
      summary = s
      top = lb.rows
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  load()
</script>

<h1>Dashboard</h1>

{#if loading}
  <div class="state">Loading…</div>
{:else if error}
  <div class="state error">Failed to load: {error}</div>
{:else}
  <div class="grid tiles">
    <div class="card tile">
      <div class="value">{summary.scored_count}</div>
      <div class="label">Breads scored</div>
    </div>
    <div class="card tile">
      <div class="value">{summary.distinct_users}</div>
      <div class="label">Bakers</div>
    </div>
    <div class="card tile">
      <div class="value">{pct(summary.avg_roundness)}</div>
      <div class="label">Avg roundness</div>
    </div>
    <div class="card tile">
      <div class="value">{pct(summary.max_roundness)}</div>
      <div class="label">Roundest ever</div>
    </div>
  </div>

  <div class="card" style="margin-top:1rem">
    <div class="controls" style="justify-content:space-between">
      <h2 style="margin:0">🏆 Roundest breads</h2>
      <a href={href('/leaderboard')}>Full leaderboard →</a>
    </div>
    {#if top.length === 0}
      <p class="muted">No scored breads yet.</p>
    {:else}
      <table>
        <thead>
          <tr><th>#</th><th>Message</th><th class="num">Roundness</th></tr>
        </thead>
        <tbody>
          {#each top as row, i}
            <tr>
              <td class="num">{i + 1}</td>
              <td>
                <a href={href(`/users/${row.author_id}`)}>baker {row.author_id}</a>
                {#if row.replymessage_jump_url}
                  · <a href={row.replymessage_jump_url} target="_blank" rel="noreferrer">jump</a>
                {/if}
              </td>
              <td class="num"><RoundnessCell value={row.roundness} /></td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
{/if}
