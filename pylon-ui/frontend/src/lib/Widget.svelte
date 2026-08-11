<script>
  import { t } from './i18n.js'
  import { onDestroy } from 'svelte'
  import { fly } from 'svelte/transition'
  import { Do } from '../../wailsjs/go/main/App.js'
  import { daemonOnline } from './daemon.js'

  export let icon = ''
  export let title = ''
  export let action = ''
  export let params = {}
  export let refresh = 0 // minutes, 0 = off
  export let accent = 'var(--accent)'
  export let onEdit = null

  let state = 'connecting' // connecting | loading | ok | error
  let text = ''
  let spinning = false
  let timer = null

  // Never surface daemon internals to the user. An unconfigured/unauthorized
  // service reports "servissiz/bilinmeyen aksiyon"; show plain language instead.
  function friendlyError(msg) {
    if (/unknown or unregistered action|unknown action/i.test(msg)) return $t('ui.widget.not_connected')
    return $t('ui.widget.unreachable')
  }

  async function load() {
    // On a cold launch the GUI is still bringing the daemon up; don't flash a
    // hard error — sit in "connecting" and let the daemonOnline flip retry us.
    if ($daemonOnline !== true) { state = 'connecting'; return }
    state = 'loading'
    spinning = true
    try {
      text = await Do(action, params)
      state = 'ok'
    } catch (e) {
      const msg = (e && e.message) ? e.message : String(e)
      // A dial failure means the daemon went away — treat as connecting, not a
      // service error, so it recovers on its own when the socket comes back.
      if (/daemon is not running/i.test(msg)) { state = 'connecting'; return }
      text = friendlyError(msg)
      state = 'error'
    } finally {
      spinning = false
    }
  }

  function scheduleRefresh(minutes) {
    if (timer) { clearInterval(timer); timer = null }
    if (minutes > 0) timer = setInterval(load, minutes * 60 * 1000)
  }

  // Reload on any of: action/params edited in Settings, or the daemon flipping
  // online (which changes the key and re-fires load once the socket answers).
  let lastKey = null
  function maybeLoad(key) {
    if (key === lastKey) return
    lastKey = key
    load()
  }

  onDestroy(() => { if (timer) clearInterval(timer) })
  $: scheduleRefresh(refresh)
  $: maybeLoad(`${action}|${JSON.stringify(params)}|${$daemonOnline}`)
</script>

<div class="widget {state}" style="--wa: {accent}" in:fly={{ y: 12, duration: 320 }}>
  <span class="stripe"></span>
  <header>
    <span class="tile">{@html icon}</span>
    <span class="title">{title}</span>
    {#if onEdit}
      <button class="edit" on:click={onEdit} title={$t('ui.edit')} aria-label={$t('ui.edit')}>✎</button>
    {/if}
    <button class="refresh" class:spinning on:click={load} title={$t('ui.refresh_short')} aria-label={$t('ui.refresh_short')}>⟳</button>
  </header>

  <div class="body">
    {#if state === 'loading'}
      <span class="skeleton"></span>
    {:else if state === 'connecting'}
      <span class="connecting">{$t('ui.status.connecting')}</span>
    {:else}
      <span class="value">{text}</span>
    {/if}
  </div>
</div>

<style>
  .widget {
    position: relative;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    padding: 14px 16px;
    box-shadow: var(--shadow-soft);
    overflow: hidden;
    backdrop-filter: blur(6px);
    transition: transform var(--dur) var(--ease), border-color var(--dur) var(--ease);
  }
  .widget:hover {
    transform: translateY(-2px);
    border-color: var(--border-2);
  }
  .stripe {
    position: absolute; left: 0; top: 0; bottom: 0;
    width: 3px;
    background: var(--wa);
    opacity: 0.9;
  }
  .widget.error { border-color: rgba(242, 104, 138, 0.35); }
  .widget.error .stripe { background: var(--err); }

  header { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
  .tile {
    width: 30px; height: 30px; flex: 0 0 auto;
    display: grid; place-items: center;
    color: var(--wa);
  }
  .tile :global(svg) { width: 16px; height: 16px; }
  .tile :global(img) { width: 20px; height: 20px; object-fit: contain; }
  .title { font-weight: 700; color: var(--text-1); font-size: 13px; flex: 1; }
  .edit, .refresh {
    border: none; background: transparent; color: var(--text-3);
    cursor: pointer; font-size: 13px; line-height: 1;
    transition: color var(--dur), transform var(--dur);
  }
  .refresh { font-size: 15px; }
  .edit:hover, .refresh:hover { color: var(--text-1); }
  .refresh.spinning { animation: spin 0.7s linear infinite; color: var(--wa); }
  @keyframes spin { to { transform: rotate(360deg); } }

  .body { color: var(--text-0); font-size: 15px; line-height: 1.45; min-height: 22px; font-weight: 600; }
  .value { display: inline-block; }
  .connecting { font-size: 12px; font-weight: 500; color: var(--text-3); }
  .widget.error .body { color: var(--err); font-size: 12px; font-weight: 500; }

  .skeleton {
    display: block; height: 16px; width: 70%;
    border-radius: 6px;
    background: linear-gradient(90deg, var(--surface-2) 25%, var(--border-2) 37%, var(--surface-2) 63%);
    background-size: 400% 100%;
    animation: shimmer 1.3s ease-in-out infinite;
  }
  @keyframes shimmer { 0% { background-position: 100% 0; } 100% { background-position: 0 0; } }
</style>
