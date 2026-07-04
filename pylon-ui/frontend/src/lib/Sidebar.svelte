<script>
  // The single sidebar dock (PLANNED.md Phase 5 shell): P brand, the 3
  // character sockets, and settings share one strip, one hover trigger, one
  // 72→240px expand animation. Placeholder shapes stand in for character art;
  // socket styling is driven entirely by dispatchStore so real art swaps in
  // without touching the state logic.
  import { onMount, onDestroy } from 'svelte'
  import { DaemonRunning } from '../../wailsjs/go/main/App.js'
  import { DOMAINS, dispatchStore, Z } from './dispatchStore.js'
  import { musicPlaying } from './ambientStore.js'

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

  // Socket is visibly empty while the character is off on a dispatch.
  const isAway = (s) => s === 'travel' || s === 'absent' || s === 'return'

  const registerSocket = (el, id) => dispatchStore.registerSocket(id, el)

  function statusText(c) {
    if (c.state === 'idle' || c.state === 'preview') return 'boşta' // dummy — real service state in Phase 2
    if (c.state === 'absent') return `görevde · ~${Math.ceil(c.etaMs / 1000)}s`
    if (c.state === 'settle') return 'döndü'
    return 'yolda'
  }
</script>

<aside
  class="dock"
  class:expanded
  style="z-index: {Z.dock};"
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

  <div class="chars">
    {#each DOMAINS as d (d.id)}
      <div class="row" style="--c: {d.accent};">
        <div class="icon-col">
          <div class="socket" class:away={isAway($dispatchStore[d.id].state)} use:registerSocket={d.id}>
            {#if isAway($dispatchStore[d.id].state)}
              <!-- Absent (LOCKED): dashed outline + hollow ring + ETA; the
                   socket slot itself never collapses or disappears. -->
              <span class="ring"></span>
              {#if $dispatchStore[d.id].state === 'absent'}
                <span class="eta">~{Math.ceil($dispatchStore[d.id].etaMs / 1000)}s</span>
              {/if}
            {:else}
              <div
                class="avatar {$dispatchStore[d.id].state}"
                class:dance={$musicPlaying && $dispatchStore[d.id].state === 'idle'}
              ></div>
            {/if}
          </div>
        </div>
        <div class="info">
          <span class="name">{d.label}</span>
          <span class="status">{statusText($dispatchStore[d.id])}</span>
        </div>
      </div>
    {/each}
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

  .chars {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 14px;
    flex: 1;
  }

  /* Socket: fixed footprint whether the character is seated or away. */
  .socket {
    position: relative;
    width: 48px; height: 48px;
    display: grid;
    place-items: center;
    border-radius: 14px;
    border: 1px solid transparent;
    transition: border-color var(--dur);
  }
  .socket.away {
    border: 1px dashed color-mix(in srgb, var(--c) 55%, transparent);
  }
  .ring {
    width: 22px; height: 22px;
    border-radius: 50%;
    border: 2px solid color-mix(in srgb, var(--c) 45%, transparent);
    opacity: 0.8;
  }
  .eta {
    position: absolute;
    bottom: -2px; right: 0;
    font-size: 9px; font-weight: 700;
    color: var(--text-2);
    background: var(--bg-2);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0 4px;
    line-height: 1.5;
  }

  /* Placeholder character: tinted rounded square (art lands in Phase 5b). */
  .avatar {
    width: 36px; height: 36px;
    border-radius: 12px;
    background:
      radial-gradient(circle at 32% 28%, rgba(255, 255, 255, 0.35), transparent 45%),
      color-mix(in srgb, var(--c) 80%, var(--bg-0));
    box-shadow: 0 0 12px color-mix(in srgb, var(--c) 35%, transparent);
    transition: transform 150ms var(--ease), box-shadow 150ms var(--ease);
  }

  /* Preview (LOCKED): seated lean-in + highlight intensity increase. */
  .avatar.preview {
    transform: translateY(-1px) scale(1.06) rotate(-3deg);
    box-shadow: 0 0 18px color-mix(in srgb, var(--c) 65%, transparent);
  }

  /* Exit (LOCKED): 120ms, lift 4px + tilt 15° toward exit edge, ease-out. */
  .avatar.exit { animation: exit-lift 120ms ease-out forwards; }
  @keyframes exit-lift {
    to { transform: translateY(-4px) rotate(15deg); opacity: 0.9; }
  }

  /* Return settle (LOCKED): brief seat-flash. */
  .avatar.settle { animation: seat-flash 260ms var(--ease); }
  @keyframes seat-flash {
    0%   { box-shadow: 0 0 22px color-mix(in srgb, var(--c) 90%, transparent); transform: scale(1.1); }
    100% { box-shadow: 0 0 12px color-mix(in srgb, var(--c) 35%, transparent); transform: scale(1); }
  }

  /* Ambient music reaction — driven only by the single global ambientStore. */
  .avatar.dance { animation: bob 640ms ease-in-out infinite; }
  @keyframes bob {
    0%, 100% { transform: translateY(0) rotate(-4deg); }
    50%      { transform: translateY(-3px) rotate(4deg); }
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

  .bottom { display: flex; flex-direction: column; gap: 8px; }
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
