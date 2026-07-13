<script>
  import { api } from '../lib/api.js'
  import { formatDateTime } from '../lib/format.js'

  // messageId + channelId identify the message to fetch from the bot (we point
  // this at the bot's reply, which carries the verdict text + annotated image).
  // jumpUrl (optional) is shown as a link to open the message in Discord.
  //
  // inline (default false): when true, render just the faked message card with
  // no modal backdrop / close button / Escape handler — for embedding directly
  // in a page. onClose is then optional.
  let { messageId, channelId, jumpUrl = null, onClose = null, inline = false } = $props()

  let preview = $state(null)
  let error = $state(null)
  let loading = $state(true)

  async function load() {
    loading = true
    error = null
    try {
      preview = await api.messagePreview(messageId, channelId)
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  load()

  // Human-readable timestamp from unix ms, or "" if absent. Day-first (EU).
  let when = $derived(preview?.timestamp_ms ? formatDateTime(preview.timestamp_ms) : '')

  function onKey(e) {
    if (!inline && e.key === 'Escape' && onClose) onClose()
  }
</script>

<svelte:window on:keydown={onKey} />

{#snippet card()}
  {#if loading}
    <div class="state">Loading message…</div>
  {:else if error}
    <div class="state error">
      Couldn't load the message: {error}
      {#if jumpUrl}
        <p><a href={jumpUrl} target="_blank" rel="noreferrer">Open in Discord ↗</a></p>
      {/if}
    </div>
  {:else if preview}
    <article class="msg">
      <div class="head">
        {#if preview.author_avatar}
          <img class="avatar" src={preview.author_avatar} alt="" />
        {:else}
          <div class="avatar placeholder">🍞</div>
        {/if}
        <div class="who">
          <span class="name">{preview.author_name || 'Unknown baker'}</span>
          {#if when}<span class="ts">{when}</span>{/if}
        </div>
      </div>

      {#if preview.content}
        <p class="content">{preview.content}</p>
      {/if}

      {#if preview.image_urls && preview.image_urls.length}
        <div class="images">
          {#each preview.image_urls as url}
            <img class="attachment" src={url} alt="attachment" />
          {/each}
        </div>
      {:else}
        <p class="muted">No images in this message.</p>
      {/if}

      {#if jumpUrl}
        <div class="foot">
          <a href={jumpUrl} target="_blank" rel="noreferrer">Open in Discord ↗</a>
        </div>
      {/if}
    </article>
  {/if}
{/snippet}

{#if inline}
  <div class="inline">
    {@render card()}
  </div>
{:else}
  <div
    class="backdrop"
    role="button"
    tabindex="-1"
    onclick={onClose}
    onkeydown={(e) => e.key === 'Enter' && onClose()}
  >
    <div class="modal" role="presentation" onclick={(e) => e.stopPropagation()}>
      <button class="close" onclick={onClose} aria-label="Close">×</button>
      {@render card()}
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.65);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
    padding: 1rem;
  }
  .modal {
    position: relative;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 12px;
    max-width: min(92vw, 560px);
    max-height: 90vh;
    overflow: auto;
    padding: 1.25rem;
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.35);
  }
  .close {
    position: absolute;
    top: 0.5rem;
    right: 0.6rem;
    background: none;
    border: none;
    font-size: 1.6rem;
    line-height: 1;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.1rem 0.4rem;
  }
  .close:hover {
    color: var(--text);
  }
  .inline {
    /* Embedded card: no backdrop/chrome, just the faked message. */
    display: block;
  }
  .msg {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .avatar {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    object-fit: cover;
    flex: none;
  }
  .avatar.placeholder {
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--surface-2);
    font-size: 1.1rem;
  }
  .who {
    display: flex;
    flex-direction: column;
  }
  .name {
    font-weight: 700;
    color: var(--text);
  }
  .ts {
    font-size: 0.78rem;
    color: var(--text-muted);
  }
  .content {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .images {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .attachment {
    width: 100%;
    border-radius: 10px;
    border: 1px solid var(--border);
    display: block;
  }
  .foot {
    margin-top: 0.25rem;
  }
</style>
