<script>
  import { api, imageURL } from '../lib/api.js'
  import { pct, displayName } from '../lib/format.js'
  import RoundnessChart from '../components/RoundnessChart.svelte'
  import MessagePreview from '../components/MessagePreview.svelte'

  let { id } = $props()
  // selected: the message (leaderboard-shaped: replymessage_id / channel_id /
  // replymessage_jump_url) whose preview is shown inline. Set by the best/worst
  // card buttons or by clicking a point on the roundness chart.
  let selected = $state(null)

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
        {:else if !m}
          <p class="muted">No scored breads yet.</p>
        {/if}
        {#if m}
          <div class="controls" style="gap:0.75rem">
            <button class="link" onclick={() => (selected = m)}>preview message</button>
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <div class="card" style="margin-top:1rem">
    <h2 style="margin-top:0">Roundness over time</h2>
    {#if history.length === 0}
      <p class="muted">No history yet.</p>
    {:else}
      <RoundnessChart {history} onSelect={(p) => (selected = p)} />
      <p class="muted hint">Click a point to preview that bread's message.</p>
    {/if}
  </div>
{/if}

{#if selected}
  {#key selected}
    <MessagePreview
      messageId={selected.replymessage_id}
      channelId={selected.channel_id}
      jumpUrl={selected.replymessage_jump_url}
      onClose={() => (selected = null)}
    />
  {/key}
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
  .hint {
    margin: 0.5rem 0 0;
    font-size: 0.82rem;
  }
</style>
