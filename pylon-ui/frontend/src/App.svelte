<script>
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { t, syncLanguage } from './lib/i18n.js'
  import Sidebar from './lib/Sidebar.svelte'
  import Widget from './lib/Widget.svelte'
  import DockerWidget from './lib/DockerWidget.svelte'
  import PylonStage from './lib/PylonStage.svelte'
  import VoiceBar from './lib/VoiceBar.svelte'
  import Settings from './lib/Settings.svelte'
  import DockerPage from './lib/DockerPage.svelte'
  import { widgets, catalogEntry, modeOf } from './lib/widgets.js'
  import { sidebarPages } from './lib/sidebarPages.js'

  // The interface follows the daemon's language, asked for once at startup.
  onMount(syncLanguage)

  // A Docker "container" widget gets the rich interactive card; everything else
  // uses the generic text card.
  const isRichDocker = (w) => w.type === 'docker' && w.mode === 'container'

  let view = 'home' // 'home' | 'settings' | <page-id>
  let editing = null // widget instance being edited, or null
  const toggleSettings = () => (view = view === 'settings' ? 'home' : 'settings')
  const editWidget = (w) => { editing = w; view = 'settings' }
  const navigate = (v) => (view = v)

  // The pinned page whose id matches the current view (else undefined). If a
  // pinned page is removed while open, activePage becomes undefined and the
  // template falls through to home — no explicit reset needed.
  $: activePage = $sidebarPages.find((p) => p.id === view)

  // Home renders the ordered instance array, split by column. Starts empty.
  //
  // The title is resolved here, once, for every card: an empty one means the
  // widget still carries its type's default name, which has to follow the
  // language. `t` is passed in rather than read inside, because Svelte only
  // tracks the stores it can see in the reactive statements themselves — a $t
  // hidden inside this function would leave the titles in the old language
  // until something unrelated changed.
  function withAction(w, t) {
    const entry = catalogEntry(w.type)
    const mode = modeOf(w.type, w.mode)
    return { ...w, icon: entry.icon, action: mode.action, title: w.title || t(entry.title) }
  }
  $: left  = $widgets.filter((w) => w.column === 'left').map((w) => withAction(w, $t))
  $: right = $widgets.filter((w) => w.column === 'right').map((w) => withAction(w, $t))
  $: empty = $widgets.length === 0

  // Workspace dims while the sidebar dock is hover-expanded.
  let dockExpanded = false
</script>

<div class="app">
  <Sidebar onSettings={toggleSettings} onNavigate={navigate} active={view} onExpand={(v) => (dockExpanded = v)} />

  {#if view === 'settings'}
    <main class="settings-wrap" class:dimmed={dockExpanded} in:fade={{ duration: 200 }}>
      <Settings bind:editing />
    </main>
  {:else if activePage}
    <main class="page-wrap" class:dimmed={dockExpanded} in:fade={{ duration: 200 }}>
      {#if activePage.type === 'docker'}
        <DockerPage />
      {/if}
    </main>
  {:else}
    <main class="home" class:dimmed={dockExpanded} in:fade={{ duration: 200 }}>
      <section class="col left">
        {#each left as w (w.id)}
          {#if isRichDocker(w)}
            <DockerWidget title={w.title} params={w.params} refresh={w.refresh} accent={w.accent} onEdit={() => editWidget(w)} />
          {:else}
            <Widget {...w} onEdit={() => editWidget(w)} />
          {/if}
        {/each}
      </section>

      <section class="center">
        <PylonStage />
        <VoiceBar />
        {#if empty}
          <p class="hint" in:fade>
            {$t('ui.home.empty')} <button class="link" on:click={toggleSettings}>{$t('ui.settings.title')}</button>
          </p>
        {/if}
      </section>

      <section class="col right">
        {#each right as w (w.id)}
          {#if isRichDocker(w)}
            <DockerWidget title={w.title} params={w.params} refresh={w.refresh} accent={w.accent} onEdit={() => editWidget(w)} />
          {:else}
            <Widget {...w} onEdit={() => editWidget(w)} />
          {/if}
        {/each}
      </section>
    </main>
  {/if}
</div>

<style>
  .app { display: flex; height: 100vh; }

  .home {
    position: relative;
    z-index: 0; /* workspace background layer */
    flex: 1;
    min-width: 0;             /* let the grid shrink instead of overflowing the window */
    display: grid;
    grid-template-columns: minmax(0, 300px) minmax(0, 1fr) minmax(0, 300px);
    align-items: center;
    gap: 28px;
    padding: 32px;
    overflow-y: auto;         /* tall content scrolls instead of clipping */
    transition: opacity 180ms cubic-bezier(0.2, 0.9, 0.25, 1);
  }
  .home.dimmed, .settings-wrap.dimmed { opacity: 0.5; }

  .col {
    display: flex; flex-direction: column; gap: 18px;
    align-self: center;
    min-width: 0;
  }
  .center { display: flex; flex-direction: column; align-items: center; gap: 20px; }

  /* Narrow window: collapse to one column — Pylon on top, widgets stacked below,
     vertically scrollable — so nothing overflows or gets clipped when small. */
  @media (max-width: 820px) {
    .home {
      grid-template-columns: 1fr;
      align-items: start;
      justify-items: center;
      gap: 20px;
      padding: 20px;
    }
    .center { order: -1; }            /* Pylon first in the stack */
    .col { width: 100%; max-width: 440px; align-self: stretch; }
  }

  .hint { color: var(--text-2); font-size: 13px; }
  .link {
    border: none; background: none; padding: 0; cursor: pointer;
    color: var(--accent); font: inherit; font-weight: 700;
  }
  .link:hover { text-decoration: underline; }

  .settings-wrap, .page-wrap {
    flex: 1; display: flex;
    min-width: 0;
    transition: opacity 180ms cubic-bezier(0.2, 0.9, 0.25, 1);
  }
  .page-wrap.dimmed { opacity: 0.5; }
</style>
