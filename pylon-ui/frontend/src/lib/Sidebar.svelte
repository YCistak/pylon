<script>
  import { onMount, onDestroy } from 'svelte'
  import { DaemonRunning } from '../../wailsjs/go/main/App.js'

  export let onSettings = () => {}
  export let active = 'home'

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
    <button class="gear" class:active={active === 'settings'} on:click={onSettings} title="Ayarlar" aria-label="Ayarlar">⚙</button>
  </div>
</aside>

<style>
  .sidebar {
    width: 68px;
    flex: 0 0 68px;
    background: linear-gradient(180deg, var(--bg-2), var(--bg-0));
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 14px 0;
    gap: 16px;
  }
  .brand {
    width: 40px; height: 40px;
    border-radius: 13px;
    display: grid; place-items: center;
    font-weight: 800; color: #0a0d14;
    background: linear-gradient(150deg, var(--accent-2), var(--accent));
    box-shadow: 0 0 16px var(--accent-glow);
  }
  .chars { display: flex; flex-direction: column; gap: 12px; margin-top: 8px; }
  .char.slot {
    width: 40px; height: 40px;
    border-radius: 13px;
    border: 1px dashed var(--border-2);
    opacity: 0.45;
    transition: opacity var(--dur);
  }
  .char.slot:hover { opacity: 0.7; }

  .bottom { margin-top: auto; display: flex; flex-direction: column; align-items: center; gap: 14px; }
  .dot { width: 9px; height: 9px; border-radius: 50%; }
  .dot.on { background: var(--ok); box-shadow: 0 0 9px var(--ok); }
  .dot.off { background: var(--err); box-shadow: 0 0 9px var(--err); }
  .gear {
    width: 40px; height: 40px;
    border: 1px solid var(--border); border-radius: 13px;
    background: var(--surface); color: var(--text-2);
    font-size: 18px; cursor: pointer;
    transition: background var(--dur), color var(--dur), border-color var(--dur);
  }
  .gear:hover { background: var(--surface-2); color: var(--text-0); }
  .gear.active { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 45%, transparent); background: var(--accent-soft); }
</style>
