<script>
  import { api, imageURL } from '../lib/api.js'
  import { href } from '../lib/router.js'
  import RoundnessCell from '../components/RoundnessCell.svelte'

  let order = $state('max')
  let n = $state(10)
  let rows = $state([])
  let error = $state(null)
  let loading = $state(true)

  async function load() {
    loading = true
    error = null
    try {
      const lb = await api.leaderboard(order, n)
      rows = lb.rows
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  // Reload whenever the controls change.
  $effect(() => {
    order
    n
    load()
  })
</script>

<h1>Leaderboard</h1>

<div class="controls" style="margin-bottom:1rem">
  <button class:active={order === 'max'} onclick={() => (order = 'max')}>Roundest</button>
  <button class:active={order === 'min'} onclick={() => (order = 'min')}>Least round</button>
  <span class="muted">·</span>
  <select bind:value={n}>
    {#each [10, 25, 50, 100] as opt}
      <option value={opt}>Top {opt}</option>
    {/each}
  </select>
</div>

{#if loading}
  <div class="state">Loading…</div>
{:else if error}
  <div class="state error">Failed to load: {error}</div>
{:else if rows.length === 0}
  <div class="state">No scored breads yet.</div>
{:else}
  <div class="card">
    <table>
      <thead>
        <tr><th>#</th><th>Image</th><th>Baker</th><th class="num">Roundness</th><th></th></tr>
      </thead>
      <tbody>
        {#each rows as row, i}
          <tr>
            <td class="num">{i + 1}</td>
            <td>
              {#if row.annotated_image}
                <img
                  class="mini"
                  src={imageURL('predictions', row.annotated_image)}
                  alt="bread"
                  loading="lazy"
                />
              {:else}
                <span class="muted">—</span>
              {/if}
            </td>
            <td><a href={href(`/users/${row.author_id}`)}>baker {row.author_id}</a></td>
            <td class="num"><RoundnessCell value={row.roundness} /></td>
            <td class="num">
              {#if row.replymessage_jump_url}
                <a href={row.replymessage_jump_url} target="_blank" rel="noreferrer">jump ↗</a>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

<style>
  .mini {
    width: 48px;
    height: 48px;
    object-fit: cover;
    border-radius: 8px;
    border: 1px solid var(--border);
    display: block;
  }
</style>
