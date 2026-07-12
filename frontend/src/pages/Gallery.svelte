<script>
  import { api, imageURL } from '../lib/api.js'
  import { href } from '../lib/router.js'
  import { pct } from '../lib/format.js'

  // The gallery draws from the leaderboard rows (which carry annotated_image,
  // author, and roundness) — there is no dedicated "all images" endpoint, and
  // the leaderboard already surfaces the scored breads we have images for.
  let items = $state([])
  let error = $state(null)
  let loading = $state(true)
  let selected = $state(null) // index into items, or null

  async function load() {
    loading = true
    error = null
    try {
      const lb = await api.leaderboard('max', 100)
      items = lb.rows.filter((r) => r.annotated_image)
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  load()

  function open(i) {
    selected = i
  }
  function close() {
    selected = null
  }
  function step(delta) {
    if (selected == null) return
    selected = (selected + delta + items.length) % items.length
  }

  function onKey(e) {
    if (selected == null) return
    if (e.key === 'Escape') close()
    else if (e.key === 'ArrowRight') step(1)
    else if (e.key === 'ArrowLeft') step(-1)
  }
</script>

<svelte:window on:keydown={onKey} />

<h1>🖼️ Gallery</h1>

{#if loading}
  <div class="state">Loading…</div>
{:else if error}
  <div class="state error">Failed to load: {error}</div>
{:else if items.length === 0}
  <div class="state">No saved images yet.</div>
{:else}
  <div class="gallery">
    {#each items as item, i}
      <button class="thumb" onclick={() => open(i)} title={pct(item.roundness)}>
        <img src={imageURL('predictions', item.annotated_image)} alt="bread" loading="lazy" />
        <span class="badge">{pct(item.roundness)}</span>
      </button>
    {/each}
  </div>
{/if}

{#if selected != null}
  {@const item = items[selected]}
  <div
    class="lightbox"
    role="button"
    tabindex="-1"
    onclick={close}
    onkeydown={(e) => e.key === 'Enter' && close()}
  >
    <button class="nav prev" onclick={(e) => { e.stopPropagation(); step(-1) }} aria-label="Previous">‹</button>
    <div class="frame" role="presentation" onclick={(e) => e.stopPropagation()}>
      <img src={imageURL('predictions', item.annotated_image)} alt="bread" />
      <div class="meta">
        <span class="round">{pct(item.roundness)} round</span>
        <a href={href(`/users/${item.author_id}`)} onclick={close}>baker {item.author_id}</a>
        {#if item.replymessage_jump_url}
          <a href={item.replymessage_jump_url} target="_blank" rel="noreferrer">jump ↗</a>
        {/if}
      </div>
    </div>
    <button class="nav next" onclick={(e) => { e.stopPropagation(); step(1) }} aria-label="Next">›</button>
  </div>
{/if}

<style>
  .gallery {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 0.75rem;
  }
  .thumb {
    position: relative;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: 10px;
    overflow: hidden;
    cursor: pointer;
    background: var(--surface-2);
    aspect-ratio: 1;
  }
  .thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .thumb:hover img {
    transform: scale(1.04);
  }
  .thumb img {
    transition: transform 0.15s ease;
  }
  .badge {
    position: absolute;
    bottom: 6px;
    right: 6px;
    background: rgba(0, 0, 0, 0.65);
    color: #fff;
    font-size: 0.72rem;
    font-weight: 600;
    padding: 2px 6px;
    border-radius: 6px;
    font-variant-numeric: tabular-nums;
  }

  .lightbox {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.8);
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    z-index: 100;
    padding: 1rem;
  }
  .frame {
    max-width: min(90vw, 900px);
    max-height: 90vh;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .frame img {
    max-width: 100%;
    max-height: 78vh;
    object-fit: contain;
    border-radius: 10px;
    background: #fff;
  }
  .meta {
    display: flex;
    gap: 1rem;
    align-items: center;
    color: #eee;
    flex-wrap: wrap;
    justify-content: center;
  }
  .meta .round {
    font-weight: 700;
    color: #fff;
  }
  .meta a {
    color: var(--accent);
  }
  .nav {
    background: rgba(255, 255, 255, 0.12);
    color: #fff;
    border: none;
    font-size: 2rem;
    line-height: 1;
    width: 48px;
    height: 64px;
    border-radius: 10px;
    cursor: pointer;
    flex: none;
  }
  .nav:hover {
    background: rgba(255, 255, 255, 0.25);
  }
</style>
