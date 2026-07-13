<script>
  // The sidebar dock: P brand + user-pinned pages + settings share one strip,
  // one hover trigger, one 72→240px expand animation.
  import { sidebarPages, pageCatalogEntry } from './sidebarPages.js'
  import { daemonOnline } from './daemon.js'

  export let onSettings = () => {}
  export let onNavigate = () => {} // ('home' | page-id)
  export let active = 'home'       // 'home' | 'settings' | <page-id>
  // Parent is told when the dock hover-expands so the workspace can dim.
  export let onExpand = () => {}

  let expanded = false
  const setExpanded = (v) => { expanded = v; onExpand(v) }

  // Shared daemon probe (see daemon.js). null = still connecting on cold launch.
  $: running = $daemonOnline === true
  $: dotState = running ? 'on' : ($daemonOnline === null ? 'connecting' : 'off')
  $: statusText = running ? 'çevrimiçi' : ($daemonOnline === null ? 'bağlanıyor…' : 'çevrimdışı')
</script>

<aside
  class="dock"
  class:expanded
  on:mouseenter={() => setExpanded(true)}
  on:mouseleave={() => setExpanded(false)}
>
  <div class="row">
    <div class="icon-col">
      <button class="brand" title="Pylon" on:click={() => onNavigate('home')} aria-label="Ana sayfa">P</button>
    </div>
    <div class="info">
      <span class="name">Pylon</span>
    </div>
  </div>

  {#if $sidebarPages.length}
    <div class="pages">
      {#each $sidebarPages as p (p.id)}
        {@const entry = pageCatalogEntry(p.type)}
        {#if entry}
          <div class="row">
            <div class="icon-col">
              <button
                class="page-btn" class:active={active === p.id}
                style="--wa: {entry.accent}"
                on:click={() => onNavigate(p.id)}
                title={entry.title} aria-label={entry.title}
              >{@html entry.icon}</button>
            </div>
            <div class="info">
              <span class="name">{entry.title}</span>
            </div>
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  <div class="bottom">
    <div class="row">
      <div class="icon-col">
        <span class="dot {dotState}" title={running ? 'Pylon çalışıyor' : ($daemonOnline === null ? 'Pylon başlatılıyor' : 'Pylon çalışmıyor')}></span>
      </div>
      <div class="info">
        <span class="status">{statusText}</span>
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
    border: none; border-radius: 13px; cursor: pointer;
    display: grid; place-items: center;
    font-weight: 800; font-size: 16px; color: #0a0d14;
    background: linear-gradient(150deg, var(--accent-2), var(--accent));
    box-shadow: 0 0 16px var(--accent-glow);
  }

  /* Pinned page buttons — brand-tinted icon tiles. */
  .pages { display: flex; flex-direction: column; gap: 8px; }
  .page-btn {
    width: 40px; height: 40px;
    border: 1px solid var(--border); border-radius: 13px;
    background: var(--surface); color: var(--wa);
    cursor: pointer; display: grid; place-items: center;
    transition: background var(--dur), border-color var(--dur), transform var(--dur);
  }
  .page-btn :global(svg) { width: 20px; height: 20px; }
  .page-btn:hover { background: var(--surface-2); border-color: var(--border-2); }
  .page-btn.active {
    border-color: color-mix(in srgb, var(--wa) 55%, transparent);
    background: color-mix(in srgb, var(--wa) 14%, transparent);
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
  .dot.connecting { background: var(--warn); box-shadow: 0 0 9px var(--warn); animation: dotpulse 1.1s ease-in-out infinite; }
  @keyframes dotpulse { 0%, 100% { opacity: 0.45; } 50% { opacity: 1; } }
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
