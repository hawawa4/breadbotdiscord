<script>
  import { api, imageURL } from '../lib/api.js'
  import { pct, displayName } from '../lib/format.js'
  import RoundnessChart from '../components/RoundnessChart.svelte'

  let { id } = $props()

  let user = $state(null)
  let roundness = $state(null)
  let error = $state(null)
  let loading = $state(true)

  async function load(uid) {
    loading = true
    error = null
    user = null
    roundness = null
    try {
      // User info is best-effort: a user may have scored breads without a
      // cached discordusers row. Don't fail the whole page if it 404s.
      const [u, r] = await Promise.allSettled([api.user(uid), api.userRoundness(uid)])
      if (u.status === 'fulfilled') user = u.value
      if (r.status === 'fulfilled') {
        roundness = r.value
      } else {
        throw r.reason
      }
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  // Reload whenever the route id changes.
  $effect(() => {
    load(id)
  })

  let name = $derived(displayName(user) || `baker ${id}`)
  let history = $derived(roundness?.history ?? [])
</script>

<h1>🧑‍🍳 {name}</h1>
<p class="muted" style="margin-top:-0.5rem">ID {id}</p>

{#if loading}
  <div class="state">Loading…</div>
{:else if error}
  <div class="state error">Failed to load: {error}</div>
{:else}
  <div class="grid two">
    {#each [{ key: 'max', title: 'Roundest', emoji: '🥇' }, { key: 'min', title: 'Least round', emoji: '🫓' }] as best}
      {@const m = roundness?.[best.key]}
      <div class="card">
        <div class="controls" style="justify-content:space-between">
          <h2 style="margin:0">{best.emoji} {best.title}</h2>
          {#if m}<span class="big">{pct(m.roundness)}</span>{/if}
        </div>
        {#if m && m.annotated_image}
          <a
            href={imageURL('predictions', m.annotated_image)}
            target="_blank"
            rel="noreferrer"
          >
            <img class="hero" src={imageURL('predictions', m.annotated_image)} alt="bread" />
          </a>
        {:else if m}
          <p class="muted">No image saved for this bread.</p>
        {:else}
          <p class="muted">No scored breads yet.</p>
        {/if}
        {#if m && m.replymessage_jump_url}
          <a href={m.replymessage_jump_url} target="_blank" rel="noreferrer">jump to message ↗</a>
        {/if}
      </div>
    {/each}
  </div>

  <div class="card" style="margin-top:1rem">
    <h2 style="margin-top:0">Roundness over time</h2>
    {#if history.length === 0}
      <p class="muted">No history yet.</p>
    {:else}
      <RoundnessChart {history} />
    {/if}
  </div>
{/if}

<style>
  .big {
    font-weight: 700;
    font-size: 1.4rem;
    color: var(--accent);
    font-variant-numeric: tabular-nums;
  }
  .hero {
    width: 100%;
    border-radius: 10px;
    border: 1px solid var(--border);
    margin: 0.75rem 0;
    display: block;
  }
</style>
