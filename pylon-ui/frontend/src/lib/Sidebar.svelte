<script>
  // The sidebar dock: P brand + settings share one strip, one hover trigger,
  // one 72→240px expand animation.
  import { onMount, onDestroy } from 'svelte'
  import { DaemonRunning } from '../../wailsjs/go/main/App.js'

  export let onSettings = () => {}
  export let active = 'home'
  // Parent is told when the dock hover-expands so the workspace can dim.
  export let onExpand = () => {}

  let expanded = false
  const setExpanded = (v) => { expanded = v; onExpand(v) }

  let running = false
  let timer

  async function refresh() {
    try { running = await DaemonRunning() } catch { running = false }
  }
  onMount(() => { refresh(); timer = setInterval(refresh, 5000) })
  onDestroy(() => clearInterval(timer))
</script>

<aside
  class="dock"
  class:expanded
  on:mouseenter={() => setExpanded(true)}
  on:mouseleave={() => setExpanded(false)}
>
  <div class="row">
    <div class="icon-col">
      <div class="brand" title="Pylon">P</div>
    </div>
    <div class="info">
      <span class="name">Pylon</span>
    </div>
  </div>

  <div class="bottom">
    <div class="row">
      <div class="icon-col">
        <span class="dot {running ? 'on' : 'off'}" title={running ? 'Pylon çalışıyor' : 'Pylon çalışmıyor'}></span>
      </div>
      <div class="info">
        <span class="status">{running ? 'çevrimiçi' : 'çevrimdışı'}</span>
      </div>
    </div>
    <div class="row">
      <div class="icon-col">
        <button class="gear" class:active={active === 'settings'} on:click={onSettings} title="Ayarlar" aria-label="Ayarlar">⚙</button>
      </div>
      <div class="info">
        <span class="name">Ayarlar</span>
      </div>
    </div>
  </div>
</aside>

<style>
  /* LOCKED: 72px collapsed → 240px expanded, 180ms cubic-bezier(0.2,0.9,0.25,1) */
  .dock {
    position: relative;
    width: 72px;
    flex: 0 0 auto;
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding: 14px 12px;
    background: linear-gradient(180deg, var(--bg-2), var(--bg-0));
    border-right: 1px solid var(--border);
    overflow: hidden;
    transition: width 180ms cubic-bezier(0.2, 0.9, 0.25, 1);
  }
  .dock.expanded { width: 240px; }

  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 40px;
  }

  /* Fixed icon column: keeps icons centered in the 72px collapsed strip. */
  .icon-col {
    width: 48px;
    flex: 0 0 auto;
    display: grid;
    place-items: center;
  }

  .brand {
    width: 40px; height: 40px;
    border-radius: 13px;
    display: grid; place-items: center;
    font-weight: 800; color: #0a0d14;
    background: linear-gradient(150deg, var(--accent-2), var(--accent));
    box-shadow: 0 0 16px var(--accent-glow);
  }

  /* Labels revealed by the single expand animation. */
  .info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    white-space: nowrap;
    opacity: 0;
    transform: translateX(-6px);
    transition: opacity 180ms cubic-bezier(0.2, 0.9, 0.25, 1),
                transform 180ms cubic-bezier(0.2, 0.9, 0.25, 1);
  }
  .dock.expanded .info { opacity: 1; transform: translateX(0); }

  .name { font-size: 13px; font-weight: 700; color: var(--text-0); }
  .status { font-size: 11px; color: var(--text-2); }

  .bottom { display: flex; flex-direction: column; gap: 8px; margin-top: auto; }
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
