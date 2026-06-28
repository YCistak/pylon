<script>
  import { onMount } from 'svelte'
  import { Do } from '../../wailsjs/go/main/App.js'

  export let icon = ''
  export let title = ''
  export let action = ''

  let state = 'loading' // loading | ok | error
  let text = ''

  async function load() {
    state = 'loading'
    try {
      text = await Do(action)
      state = 'ok'
    } catch (e) {
      text = (e && e.message) ? e.message : String(e)
      state = 'error'
    }
  }
  onMount(load)
</script>

<div class="widget {state}">
  <header>
    <span class="icon">{icon}</span>
    <span class="title">{title}</span>
    <button class="refresh" on:click={load} title="yenile">⟳</button>
  </header>
  <div class="body">
    {#if state === 'loading'}<span class="muted">…</span>
    {:else}{text}{/if}
  </div>
</div>

<style>
  .widget {
    background: #151b26;
    border: 1px solid #1f2a3a;
    border-radius: 14px;
    padding: 14px;
    box-shadow: 0 6px 18px rgba(0,0,0,.25);
  }
  .widget.error { border-color: #5b2330; }
  header { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
  .icon { font-size: 16px; }
  .title { font-weight: 600; color: #cdd9e6; font-size: 13px; flex: 1; }
  .refresh {
    border: none; background: transparent; color: #5f7689;
    cursor: pointer; font-size: 14px; line-height: 1;
  }
  .refresh:hover { color: #9fc1e0; }
  .body { color: #e6eef7; font-size: 14px; line-height: 1.4; min-height: 20px; }
  .widget.error .body { color: #f0a0ab; font-size: 12px; }
  .muted { color: #5f7689; }
</style>
