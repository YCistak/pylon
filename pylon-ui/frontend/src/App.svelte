<script>
  import Sidebar from './lib/Sidebar.svelte'
  import Widget from './lib/Widget.svelte'
  import PylonStage from './lib/PylonStage.svelte'

  // Widgets flanking Pylon. Static for now; Settings (step 4) will toggle these.
  const leftWidgets = [
    { icon: '📅', title: 'Takvim',   action: 'calendar.list_today' },
    { icon: '📰', title: 'FreshRSS', action: 'freshrss.unread_count' },
  ]
  const rightWidgets = [
    { icon: '🐙', title: 'GitHub',   action: 'github.list_prs' },
  ]

  let view = 'home' // home | settings
  const toggleSettings = () => (view = view === 'settings' ? 'home' : 'settings')
</script>

<div class="app">
  <Sidebar onSettings={toggleSettings} />

  {#if view === 'home'}
    <main class="home">
      <section class="col left">
        {#each leftWidgets as w}<Widget {...w} />{/each}
      </section>

      <section class="center">
        <PylonStage />
      </section>

      <section class="col right">
        {#each rightWidgets as w}<Widget {...w} />{/each}
      </section>
    </main>
  {:else}
    <main class="settings">
      <h2>Ayarlar</h2>
      <p class="muted">Ayarlar 4. adımda detaylı tasarlanacak — parola ekleme,
        servis aç/kapa, widget seçimi buraya gelecek.</p>
    </main>
  {/if}
</div>

<style>
  .app { display: flex; height: 100vh; }

  .home {
    flex: 1;
    display: grid;
    grid-template-columns: 280px 1fr 280px;
    align-items: center;
    gap: 24px;
    padding: 28px;
  }
  .col { display: flex; flex-direction: column; gap: 18px; align-self: stretch; justify-content: center; }
  .center { display: grid; place-items: center; }

  .settings { flex: 1; padding: 40px; }
  .settings h2 { margin: 0 0 10px; color: #eaf2fb; }
  .muted { color: #7d93a8; max-width: 480px; line-height: 1.5; }
</style>
