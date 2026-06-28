<script>
  import { onMount, onDestroy } from 'svelte'
  import { DaemonRunning } from '../../wailsjs/go/main/App.js'

  export let onSettings = () => {}

  // Secondary characters live here later (Phase C); empty slots for now.
  const characterSlots = [0, 1, 2]

  let running = false
  let timer

  async function refresh() {
    try { running = await DaemonRunning() } catch { running = false }
  }
  onMount(() => { refresh(); timer = setInterval(refresh, 5000) })
  onDestroy(() => clearInterval(timer))
</script>

<aside class="sidebar">
  <div class="brand" title="Pylon">P</div>

  <div class="chars">
    {#each characterSlots as _}
      <div class="char slot" title="karakter (yakında)"></div>
    {/each}
  </div>

  <div class="bottom">
    <span class="dot {running ? 'on' : 'off'}" title={running ? 'daemon çalışıyor' : 'daemon kapalı'}></span>
    <button class="gear" on:click={onSettings} title="Ayarlar">⚙</button>
  </div>
</aside>

<style>
  .sidebar {
    width: 64px;
    flex: 0 0 64px;
    background: #11161f;
    border-right: 1px solid #1d2633;
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 12px 0;
    gap: 14px;
  }
  .brand {
    width: 38px; height: 38px;
    border-radius: 12px;
    display: grid; place-items: center;
    font-weight: 700; color: #0b0f16;
    background: linear-gradient(160deg, #6ee7ff, #3b82f6);
    box-shadow: 0 0 14px rgba(59,130,246,.45);
  }
  .chars { display: flex; flex-direction: column; gap: 12px; margin-top: 6px; }
  .char.slot {
    width: 38px; height: 38px;
    border-radius: 12px;
    border: 1px dashed #2a3647;
    opacity: .5;
  }
  .bottom { margin-top: auto; display: flex; flex-direction: column; align-items: center; gap: 12px; }
  .dot { width: 9px; height: 9px; border-radius: 50%; }
  .dot.on { background: #34d399; box-shadow: 0 0 8px #34d399; }
  .dot.off { background: #ef4444; box-shadow: 0 0 8px #ef4444; }
  .gear {
    width: 38px; height: 38px;
    border: none; border-radius: 12px;
    background: #1a2230; color: #9fb3c8;
    font-size: 18px; cursor: pointer;
  }
  .gear:hover { background: #222d3d; color: #e6eef7; }
</style>
