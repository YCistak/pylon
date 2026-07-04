<script>
  import { onMount, onDestroy } from 'svelte'
  import { fade } from 'svelte/transition'
  import Sidebar from './lib/Sidebar.svelte'
  import Widget from './lib/Widget.svelte'
  import PylonStage from './lib/PylonStage.svelte'
  import Settings from './lib/Settings.svelte'
  import DispatchOverlay from './lib/DispatchOverlay.svelte'
  import { AVAILABLE, layout } from './lib/widgets.js'
  import { dispatchStore, coreDimmed, Z } from './lib/dispatchStore.js'
  import { toggleMusic } from './lib/ambientStore.js'

  let view = 'home' // home | settings
  const toggleSettings = () => (view = view === 'settings' ? 'home' : 'settings')

  // Home renders ONLY what Settings enabled (persisted store). Starts empty.
  $: left  = AVAILABLE.filter((w) => $layout[w.id] === 'left')
  $: right = AVAILABLE.filter((w) => $layout[w.id] === 'right')
  $: empty = left.length === 0 && right.length === 0

  // Workspace dims while the character dock is hover-expanded.
  let dockExpanded = false

  // Core dim is a BOOLEAN GATE on activeDispatchCount (LOCKED): 0.32 while
  // any dispatch is active, 1.0 only at zero — never multiplied per dispatch.
  $: coreOpacity = $coreDimmed ? 0.32 : 1.0

  const registerStage = (el) => dispatchStore.registerStage(el)

  // ── Dev-only triggers (placeholder until Phase 2 wiring) ──
  let devOpen = false
  const onKey = (e) => {
    if (e.ctrlKey && e.shiftKey && e.code === 'KeyD') { devOpen = !devOpen; e.preventDefault() }
  }
  onMount(() => {
    window.addEventListener('keydown', onKey)
    window.pylonDev = {
      dispatch: dispatchStore.dispatch,
      preview: dispatchStore.preview,
      toggleMusic,
    }
    console.info('[pylon] dev: Ctrl+Shift+D panel · window.pylonDev.{dispatch(domain), preview(domain, on), toggleMusic()}')
  })
  onDestroy(() => window.removeEventListener('keydown', onKey))
</script>

<div class="app">
  <Sidebar onSettings={toggleSettings} active={view} onExpand={(v) => (dockExpanded = v)} />

  {#if view === 'home'}
    <main class="home" class:dimmed={dockExpanded} in:fade={{ duration: 200 }}>
      <section class="col left">
        {#each left as w (w.id)}<Widget {...w} />{/each}
      </section>

      <section class="center">
        <!-- z:10 dim wrapper — PylonStage itself is untouched -->
        <div class="core-layer" style="opacity: {coreOpacity}; z-index: {Z.core};" use:registerStage>
          <PylonStage />
        </div>
        {#if empty}
          <p class="hint" in:fade>
            Henüz widget yok — <button class="link" on:click={toggleSettings}>Ayarlar</button>'dan ekle.
          </p>
        {/if}
      </section>

      <section class="col right">
        {#each right as w (w.id)}<Widget {...w} />{/each}
      </section>
    </main>
  {:else}
    <main class="settings-wrap" class:dimmed={dockExpanded} in:fade={{ duration: 200 }}>
      <Settings />
    </main>
  {/if}

  <!-- Dispatch overlay: sibling of the workspace (LOCKED), never inside it. -->
  <DispatchOverlay />

  {#if devOpen}
    <div class="dev" transition:fade={{ duration: 120 }}>
      <span class="dev-title">dev</span>
      <button on:click={() => dispatchStore.dispatch('organizer')}>organizer ➜</button>
      <button on:click={() => dispatchStore.dispatch('devops')}>devops ➜</button>
      <button on:click={() => dispatchStore.dispatch('system_media')}>sys/media ➜</button>
      <button on:click={toggleMusic}>♪ music</button>
    </div>
  {/if}
</div>

<style>
  .app { display: flex; height: 100vh; }

  .home {
    position: relative;
    z-index: 0; /* workspace background layer */
    flex: 1;
    display: grid;
    grid-template-columns: 300px 1fr 300px;
    align-items: center;
    gap: 28px;
    padding: 32px;
    transition: opacity 180ms cubic-bezier(0.2, 0.9, 0.25, 1);
  }
  .home.dimmed, .settings-wrap.dimmed { opacity: 0.5; }

  .col {
    display: flex; flex-direction: column; gap: 18px;
    align-self: center;
  }
  .center { display: flex; flex-direction: column; align-items: center; gap: 20px; }

  .core-layer {
    position: relative;
    transition: opacity 240ms var(--ease);
  }

  .hint { color: var(--text-2); font-size: 13px; }
  .link {
    border: none; background: none; padding: 0; cursor: pointer;
    color: var(--accent); font: inherit; font-weight: 700;
  }
  .link:hover { text-decoration: underline; }

  .settings-wrap {
    flex: 1; display: flex;
    transition: opacity 180ms cubic-bezier(0.2, 0.9, 0.25, 1);
  }

  .dev {
    position: fixed;
    bottom: 14px; right: 14px;
    z-index: 60; /* dev-only, above the locked stack */
    display: flex; align-items: center; gap: 6px;
    padding: 6px 8px;
    background: var(--bg-2);
    border: 1px solid var(--border-2);
    border-radius: var(--r-sm);
    box-shadow: var(--shadow);
  }
  .dev-title { font-size: 10px; font-weight: 800; color: var(--text-3); text-transform: uppercase; }
  .dev button {
    font-size: 11px; color: var(--text-1);
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 8px; padding: 4px 8px; cursor: pointer;
  }
  .dev button:hover { background: var(--surface-2); color: var(--text-0); }
</style>
