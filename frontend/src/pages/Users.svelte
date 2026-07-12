<script>
  import { api } from '../lib/api.js'
  import { href } from '../lib/router.js'
  import { displayName } from '../lib/format.js'

  const limit = 50
  let offset = $state(0)
  let rows = $state([])
  let total = $state(0)
  let error = $state(null)
  let loading = $state(true)

  async function load() {
    loading = true
    error = null
    try {
      const res = await api.users(limit, offset)
      rows = res.rows
      total = res.total
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => {
    offset
    load()
  })

  let hasPrev = $derived(offset > 0)
  let hasNext = $derived(offset + limit < total)
</script>

<h1>Bakers</h1>

{#if loading}
  <div class="state">Loading…</div>
{:else if error}
  <div class="state error">Failed to load: {error}</div>
{:else if rows.length === 0}
  <div class="state">No users yet.</div>
{:else}
  <div class="card">
    <table>
      <thead>
        <tr><th>Name</th></tr>
      </thead>
      <tbody>
        {#each rows as u}
          <tr>
            <td><a href={href(`/users/${u.author_id}`)}>{displayName(u)}</a></td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  <div class="controls" style="margin-top:1rem; justify-content:space-between">
    <button disabled={!hasPrev} onclick={() => (offset = Math.max(0, offset - limit))}>
      ← Prev
    </button>
    <span class="muted">{offset + 1}–{Math.min(offset + limit, total)} of {total}</span>
    <button disabled={!hasNext} onclick={() => (offset = offset + limit)}>Next →</button>
  </div>
{/if}
