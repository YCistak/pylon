<script>
  import { fade } from 'svelte/transition'
  import Sidebar from './lib/Sidebar.svelte'
  import Widget from './lib/Widget.svelte'
  import PylonStage from './lib/PylonStage.svelte'
  import Settings from './lib/Settings.svelte'
  import { AVAILABLE, layout } from './lib/widgets.js'

  let view = 'home' // home | settings
  const toggleSettings = () => (view = view === 'settings' ? 'home' : 'settings')

  // Home renders ONLY what Settings enabled (persisted store). Starts empty.
  $: left  = AVAILABLE.filter((w) => $layout[w.id] === 'left')
  $: right = AVAILABLE.filter((w) => $layout[w.id] === 'right')
  $: empty = left.length === 0 && right.length === 0
</script>

<div class="app">
  <Sidebar onSettings={toggleSettings} active={view} />

  {#if view === 'home'}
    <main class="home" in:fade={{ duration: 200 }}>
      <section class="col left">
        {#each left as w (w.id)}<Widget {...w} />{/each}
      </section>

      <section class="center">
        <PylonStage />
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
    <main class="settings-wrap" in:fade={{ duration: 200 }}>
      <Settings />
    </main>
  {/if}
</div>

<style>
  .app { display: flex; height: 100vh; }

  .home {
    flex: 1;
    display: grid;
    grid-template-columns: 300px 1fr 300px;
    align-items: center;
    gap: 28px;
    padding: 32px;
  }
  .col {
    display: flex; flex-direction: column; gap: 18px;
    align-self: center;
  }
  .center { display: flex; flex-direction: column; align-items: center; gap: 20px; }

  .hint { color: var(--text-2); font-size: 13px; }
  .link {
    border: none; background: none; padding: 0; cursor: pointer;
    color: var(--accent); font: inherit; font-weight: 700;
  }
  .link:hover { text-decoration: underline; }

  .settings-wrap { flex: 1; display: flex; }
</style>
