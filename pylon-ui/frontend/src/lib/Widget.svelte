<script>
  import { onMount } from 'svelte'
  import { fly } from 'svelte/transition'
  import { Do } from '../../wailsjs/go/main/App.js'

  export let icon = ''
  export let title = ''
  export let action = ''
  export let accent = 'var(--accent)'

  let state = 'loading' // loading | ok | error
  let text = ''
  let spinning = false

  async function load() {
    state = 'loading'
    spinning = true
    try {
      text = await Do(action)
      state = 'ok'
    } catch (e) {
      text = (e && e.message) ? e.message : String(e)
      state = 'error'
    } finally {
      spinning = false
    }
  }
  onMount(load)
</script>

<div class="widget {state}" style="--wa: {accent}" in:fly={{ y: 12, duration: 320 }}>
  <span class="stripe"></span>
  <header>
    <span class="tile">{icon}</span>
    <span class="title">{title}</span>
    <button class="refresh" class:spinning on:click={load} title="yenile" aria-label="yenile">⟳</button>
  </header>

  <div class="body">
    {#if state === 'loading'}
      <span class="skeleton"></span>
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
    font-size: 15px;
    border-radius: 9px;
    background: color-mix(in srgb, var(--wa) 18%, transparent);
    border: 1px solid color-mix(in srgb, var(--wa) 30%, transparent);
  }
  .title { font-weight: 700; color: var(--text-1); font-size: 13px; flex: 1; }
  .refresh {
    border: none; background: transparent; color: var(--text-3);
    cursor: pointer; font-size: 15px; line-height: 1;
    transition: color var(--dur), transform var(--dur);
  }
  .refresh:hover { color: var(--text-1); }
  .refresh.spinning { animation: spin 0.7s linear infinite; color: var(--wa); }
  @keyframes spin { to { transform: rotate(360deg); } }

  .body { color: var(--text-0); font-size: 15px; line-height: 1.45; min-height: 22px; font-weight: 600; }
  .value { display: inline-block; }
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
