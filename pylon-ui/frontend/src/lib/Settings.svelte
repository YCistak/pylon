<script>
  import { t } from './i18n.js'
  import { CATALOG, catalogEntry, modeOf, REFRESH_OPTIONS, widgets } from './widgets.js'
  import { PAGE_CATALOG, pageCatalogEntry, sidebarPages } from './sidebarPages.js'
  import Widget from './Widget.svelte'
  import DockerWidget from './DockerWidget.svelte'
  import Accounts from './Accounts.svelte'
  import ApiKeys from './ApiKeys.svelte'
  import Language from './Language.svelte'
  import VoiceSettings from './VoiceSettings.svelte'

  export let editing = null // widget instance handed in from App (pen icon)

  // Settings grew card by card into one long column. Grouping them keeps each
  // screen to a single question — which language, what's on the home, which
  // accounts, how voice behaves — instead of making you scroll past all of them
  // to reach one.
  //
  // Language comes first, and not because it is used most: it is the one
  // setting you go looking for when you cannot read any of the others.
  const TABS = [
    { id: 'genel', label: 'ui.settings.general', hint: 'ui.settings.general_hint' },
    { id: 'gorunum', label: 'ui.settings.appearance', hint: 'ui.settings.appearance_hint' },
    { id: 'hesaplar', label: 'ui.settings.accounts', hint: 'ui.settings.accounts_hint' },
    { id: 'ses', label: 'ui.settings.voice', hint: 'ui.settings.voice_hint' },
  ]
  let activeTab = TABS[0].id
  $: activeHint = TABS.find((tab) => tab.id === activeTab)?.hint ?? ''

  // Left/right arrows move between tabs, which is what a tablist is expected to
  // do once the buttons carry tab roles. The predicate reads its own parameter:
  // it used to close over `tab` from the markup's {#each}, so findIndex matched
  // on the last-rendered tab and both arrows always stepped from the same place.
  function onTabKeydown(e) {
    const i = TABS.findIndex((tab) => tab.id === activeTab)
    if (e.key === 'ArrowRight') activeTab = TABS[(i + 1) % TABS.length].id
    else if (e.key === 'ArrowLeft') activeTab = TABS[(i - 1 + TABS.length) % TABS.length].id
    else return
    e.preventDefault()
    // Roving tabindex: the newly selected tab is the only tab stop, so focus has
    // to move with the selection or the next arrow key goes to the old button.
    document.getElementById(`tab-${activeTab}`)?.focus()
  }

  let showPicker = false
  let draft = null   // widget instance being created/edited in the modal
  let isNew = false

  // A modal covers the page but does nothing to the keyboard on its own. Without
  // the two below, focus stayed on whatever opened the dialog and Tab walked a
  // page the user could no longer see. modalEl is focused when it appears (it
  // carries tabindex="-1" for exactly that), and returnFocus hands the caret
  // back on close so a keyboard user is not dumped at the top of the document.
  let modalEl = null
  let returnFocus = null
  $: if (modalEl) modalEl.focus()

  // Where focus goes when the trigger itself is gone by the time the dialog
  // closes. Two ways that happens, and neither is an edge case: deleting the
  // widget removes its row, and the pen icon on Home unmounts the whole page on
  // its way here — that one leaves a keyboard user at the top of the document
  // every single time, which is what `isConnected` was quietly falling back to.
  // Landing on the row for the widget just edited puts the caret where the user
  // is already looking.
  let penEls = {}
  let addEl = null

  function openCreate(type) {
    const entry = catalogEntry(type)
    const mode = entry.modes[0]
    returnFocus = document.activeElement
    draft = {
      // No title: an empty one means "call it whatever this type is called in
      // the current language", resolved wherever it is drawn. Putting the
      // resolved text here is what used to freeze a widget into the language it
      // was added in. Typing a name makes it text, permanently.
      id: null, type, title: '', column: 'left',
      mode: mode.id, params: {}, refresh: 0, accent: entry.accent,
    }
    isNew = true
    showPicker = false
  }

  function openEdit(instance) {
    returnFocus = document.activeElement
    draft = { ...instance, params: { ...instance.params } }
    isNew = false
  }

  // Pen icon on Home hands us an instance via bind:editing. Land on Görünüm too,
  // so closing the modal leaves you looking at the widget it belongs to.
  $: if (editing) { activeTab = 'gorunum'; openEdit(editing); editing = null }

  function closeModal() {
    const edited = draft?.id
    draft = null
    modalEl = null
    const back = returnFocus
    returnFocus = null
    // The trigger first, then the row for the widget that was being edited, then
    // the button that adds one. Something in that list is always on screen, so
    // Tab continues from where the user was rather than from the top.
    //
    // Svelte has not removed the dialog yet at this point, so the fallbacks are
    // queued for after the DOM catches up — focusing a node the framework is
    // about to touch does not survive.
    if (back && back.isConnected) {
      back.focus()
      return
    }
    requestAnimationFrame(() => {
      const row = edited != null ? penEls[edited] : null
      if (row && row.isConnected) row.focus()
      else if (addEl && addEl.isConnected) addEl.focus()
    })
  }

  // Tab must not leave a modal dialog — behind it is a page the user cannot see
  // and cannot act on. Cycling within the dialog is what a dialog is expected to
  // do; Escape (onKeydown) is still the way out.
  function onModalKeydown(e) {
    if (e.key !== 'Tab' || !modalEl) return
    const stops = modalEl.querySelectorAll('button:not([disabled]), input:not([disabled])')
    if (stops.length === 0) return

    const first = stops[0]
    const last = stops[stops.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      last.focus()
      e.preventDefault()
    } else if (!e.shiftKey && document.activeElement === last) {
      first.focus()
      e.preventDefault()
    }
  }

  function setMode(modeId) {
    draft = { ...draft, mode: modeId, params: {} }
  }

  function save() {
    const { id, type, title, column, mode, params, refresh } = draft
    // Trimmed, so that clearing the field — or leaving only spaces in it — goes
    // back to the default name instead of saving a blank-looking title.
    const patch = { title: (title || '').trim(), column, mode, params, refresh }
    if (isNew) widgets.add(type, patch)
    else widgets.update(id, patch)
    closeModal()
  }

  function remove() {
    widgets.remove(draft.id)
    closeModal()
  }

  function onKeydown(e) {
    if (e.key !== 'Escape') return
    if (draft) closeModal()
    else if (showPicker) showPicker = false
  }

  // Close the picker when clicking anywhere outside it.
  function onWindowClick(e) {
    if (showPicker && !e.target.closest('.add-wrap')) showPicker = false
  }

  $: draftEntry = draft && catalogEntry(draft.type)
  $: draftMode = draft && modeOf(draft.type, draft.mode)
</script>

<svelte:window on:keydown={onKeydown} on:click={onWindowClick} />

<div class="settings">
  <header class="head">
    <h2>{$t('ui.settings.title')}</h2>
  </header>

  <!-- tabindex="-1": the tablist itself is never a tab stop — focus lands on the
       selected tab, which carries the roving tabindex below. -->
  <div class="tabs" role="tablist" tabindex="-1" on:keydown={onTabKeydown}>
    {#each TABS as tab (tab.id)}
      <button
        class="tab"
        class:active={activeTab === tab.id}
        id="tab-{tab.id}"
        role="tab"
        aria-selected={activeTab === tab.id}
        aria-controls={activeTab === tab.id ? 'settings-panel' : undefined}
        tabindex={activeTab === tab.id ? 0 : -1}
        on:click={() => (activeTab = tab.id)}
      >{$t(tab.label)}</button>
    {/each}
  </div>
  <!-- Translated here, not in the reactive statement: TABS holds catalog keys,
       and this printed the key itself ("ui.settings.appearance_hint") in every
       language. -->
  <p class="sub">{activeHint ? $t(activeHint) : ''}</p>

  <!-- One flex column so every card is spaced the same, whether it lives in
       this file or comes from a child component. A `.card + .card` rule cannot
       do that: Svelte scopes it to this component's own elements, so Accounts,
       ApiKeys and VoiceSettings used to sit flush against each other.

       There is one panel element, not one per tab — only the selected tab's
       aria-controls points at it, and aria-labelledby names whichever tab is
       currently showing, so the pairing stays valid as the content swaps. -->
  <div class="panel" id="settings-panel" role="tabpanel" aria-labelledby="tab-{activeTab}">
  {#if activeTab === 'genel'}
  <Language />
  {:else if activeTab === 'gorunum'}
  <section class="card">
    <div class="card-head">
      <div class="card-title">
        <h3>{$t('ui.settings.widgets')}</h3>
        <span class="count">{$widgets.length}</span>
      </div>
      <div class="add-wrap">
        <button class="add-btn" class:open={showPicker} bind:this={addEl} on:click={() => (showPicker = !showPicker)}>
          <span class="plus">+</span> {$t('ui.settings.add_widget_btn')}
        </button>
        {#if showPicker}
          <div class="picker">
            <p class="picker-title">{$t('ui.settings.which_widget')}</p>
            {#each CATALOG as c}
              <button class="picker-item" on:click={() => openCreate(c.type)}>
                <span class="tile" style="--wa: {c.accent}">{@html c.icon}</span>
                <span class="picker-name">{$t(c.title)}</span>
                <span class="chevron">›</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    {#if $widgets.length === 0}
      <div class="empty">
        <p>{$t('ui.settings.no_widgets')}</p>
        <span>{$t('ui.settings.no_widgets_hint')}</span>
      </div>
    {:else}
      <ul class="list">
        {#each $widgets as w (w.id)}
          {@const entry = catalogEntry(w.type)}
          <li>
            <span class="tile" style="--wa: {entry.accent}">{@html entry.icon}</span>
            <div class="info">
              <!-- i18n-raw: what the user typed, if anything; an untouched
                   widget has no title of its own and takes its type's. -->
              <span class="name">{w.title || $t(entry.title)}</span>
              <span class="meta">
                <span class="chip">{w.column === 'left' ? $t('ui.left') : $t('ui.right')}</span>
                <span class="mode">{$t(modeOf(w.type, w.mode)?.label ?? '')}</span>
                {#if w.refresh > 0}<span class="chip refresh">⟳ {$t('ui.settings.minutes_short', w.refresh)}</span>{/if}
              </span>
            </div>
            <button class="pen" bind:this={penEls[w.id]} on:click={() => openEdit(w)} title={$t('ui.edit')} aria-label={$t('ui.edit')}>✎</button>
            <button class="del" on:click={() => widgets.remove(w.id)} title={$t('ui.delete_short')} aria-label={$t('ui.delete_short')}>✕</button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <div class="card-head">
      <div class="card-title">
        <h3>{$t('ui.settings.sidebar')}</h3>
        <span class="count">{$sidebarPages.length}</span>
      </div>
    </div>
    <p class="section-hint">{$t('ui.settings.sidebar_hint')}</p>
    <ul class="list">
      {#each PAGE_CATALOG as pc (pc.type)}
        {@const pinned = $sidebarPages.find((p) => p.type === pc.type)}
        <li>
          <span class="tile" style="--wa: {pc.accent}">{@html pc.icon}</span>
          <div class="info">
            <span class="name">{$t(pc.title)}</span>
            <span class="meta"><span class="mode">{$t('ui.settings.full_page')}</span></span>
          </div>
          {#if pinned}
            <button class="pill on" on:click={() => sidebarPages.remove(pinned.id)}>{$t('ui.settings.pinned')}</button>
          {:else}
            <button class="pill" on:click={() => sidebarPages.add(pc.type)}>{$t('ui.settings.pin')}</button>
          {/if}
        </li>
      {/each}
    </ul>
  </section>
  {:else if activeTab === 'hesaplar'}
  <Accounts />
  <ApiKeys />
  {:else if activeTab === 'ses'}
  <VoiceSettings />
  {/if}
  </div>

  <p class="note">{$t('ui.settings.more_soon')}</p>
</div>

{#if draft}
  <div class="backdrop">
    <!-- The dim area behind the dialog is a click target, so it is a button
         rather than a div with on:click — a plain div has no keyboard
         equivalent, which is what the a11y warning was about. It is a mouse
         affordance only: aria-hidden and out of the tab order, because Escape
         and the × button are the keyboard ways out and a screen reader has no
         use for a third, unlabelled one. Being a sibling of .modal rather than
         its parent also means the dialog no longer needs to stop propagation
         to avoid closing itself. -->
    <button class="scrim" on:click={closeModal} tabindex="-1" aria-hidden="true"></button>

    <div
      class="modal"
      bind:this={modalEl}
      on:keydown={onModalKeydown}
      role="dialog"
      aria-modal="true"
      aria-labelledby="widget-modal-title"
      tabindex="-1"
    >
      <header class="modal-head">
        <span class="tile" style="--wa: {draftEntry.accent}">{@html draftEntry.icon}</span>
        <h3 id="widget-modal-title">{isNew ? $t('ui.settings.add_widget', $t(draftEntry.title)) : $t('ui.settings.edit_widget', $t(draftEntry.title))}</h3>
        <button class="x" on:click={closeModal} aria-label={$t('ui.close')}>×</button>
      </header>

      <div class="preview">
        {#if draft.type === 'docker' && draft.mode === 'container'}
          <!-- i18n-raw: free text once typed; empty falls back to the default -->
          <DockerWidget
            title={draft.title || $t(draftEntry.title)}
            params={draft.params}
            accent={draftEntry.accent}
          />
        {:else}
          <!-- i18n-raw: same, in the non-Docker preview -->
          <Widget
            icon={draftEntry.icon}
            title={draft.title || $t(draftEntry.title)}
            action={draftMode.action}
            params={draft.params}
            accent={draftEntry.accent}
          />
        {/if}
      </div>

      <label class="field">
        <span>{$t('ui.settings.title_field')}</span>
        <!-- Empty is a real choice, not a missing one: the placeholder shows
             what the widget will be called, and it follows the language. -->
        <input type="text" bind:value={draft.title} placeholder={$t(draftEntry.title)} />
      </label>

      {#if draftEntry.modes.length > 1}
        <div class="field">
          <span>{$t('ui.settings.what_to_show')}</span>
          <div class="radios">
            {#each draftEntry.modes as m}
              <button class="radio" class:active={draft.mode === m.id} on:click={() => setMode(m.id)}>
                {$t(m.label)}
              </button>
            {/each}
          </div>
        </div>
      {/if}

      {#each draftMode.params as p (p.key)}
        <label class="field">
          <span>{$t(p.label)}</span>
          <input
            type="text"
            placeholder={p.placeholder || ''}
            bind:value={draft.params[p.key]}
          />
        </label>
      {/each}

      <div class="row">
        <label class="field">
          <span>{$t('ui.settings.column')}</span>
          <div class="radios">
            <button class="radio" class:active={draft.column === 'left'}  on:click={() => (draft.column = 'left')}>{$t('ui.left')}</button>
            <button class="radio" class:active={draft.column === 'right'} on:click={() => (draft.column = 'right')}>{$t('ui.right')}</button>
          </div>
        </label>

        <label class="field">
          <span>{$t('ui.settings.auto_refresh')}</span>
          <div class="radios">
            {#each REFRESH_OPTIONS as r}
              <button class="radio" class:active={draft.refresh === r.value} on:click={() => (draft.refresh = r.value)}>
                {$t(r.label)}
              </button>
            {/each}
          </div>
        </label>
      </div>

      <footer class="modal-foot">
        {#if !isNew}
          <button class="danger" on:click={remove}>{$t('ui.delete')}</button>
        {/if}
        <span class="spacer"></span>
        <button class="ghost" on:click={closeModal}>{$t('ui.cancel')}</button>
        <button class="primary" on:click={save}>{isNew ? $t('ui.add') : $t('ui.save')}</button>
      </footer>
    </div>
  </div>
{/if}

<style>
  .settings { flex: 1; padding: 40px 44px; overflow: auto; max-width: 720px; }
  .head h2 { margin: 0 0 16px; color: var(--text-0); font-size: 24px; }
  .sub { color: var(--text-2); line-height: 1.55; margin: 0 0 22px; max-width: 520px; }

  /* Underline-style tabs: the row reads as navigation rather than as three more
     buttons competing with the ones inside the cards. */
  .tabs {
    display: flex; gap: 4px;
    border-bottom: 1px solid var(--border);
    margin-bottom: 16px;
  }
  .tab {
    position: relative;
    border: 0; background: none; cursor: pointer;
    font: inherit; font-size: 13px; font-weight: 700;
    color: var(--text-2);
    padding: 9px 14px 11px;
    border-radius: 9px 9px 0 0;
    transition: color var(--dur), background var(--dur);
  }
  .tab::after {
    content: ''; position: absolute; left: 10px; right: 10px; bottom: -1px;
    height: 2px; border-radius: 2px;
    background: var(--accent);
    transform: scaleX(0);
    transition: transform var(--dur) var(--ease);
  }
  .tab:hover { color: var(--text-1); background: var(--surface); }
  .tab.active { color: var(--text-0); }
  .tab.active::after { transform: scaleX(1); }
  .tab:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  /* The dialog takes focus when it opens; without this it would draw the same
     ring a clicked control does, which reads as "you pressed this". */
  .modal:focus-visible { outline: none; }

  .panel { display: flex; flex-direction: column; gap: 18px; }
  .section-hint { color: var(--text-3); font-size: 12.5px; margin: 0 4px 6px; line-height: 1.5; }
  .pill {
    border: 1px solid var(--border-2); background: var(--bg-2); color: var(--text-1);
    font-size: 12px; font-weight: 700; padding: 7px 13px; border-radius: 9px; cursor: pointer;
    transition: background var(--dur), border-color var(--dur), color var(--dur);
  }
  .pill:hover { border-color: var(--accent); background: var(--panel-2); }
  .pill.on { color: var(--accent-2); border-color: color-mix(in srgb, var(--accent-2) 45%, transparent); background: rgba(52, 224, 216, 0.1); }

  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    padding: 8px 16px 14px;
  }
  .card-head { display: flex; align-items: center; justify-content: space-between; margin: 14px 4px 10px; }
  .card-title { display: flex; align-items: center; gap: 9px; }
  .card-title h3 { color: var(--text-1); font-size: 14px; margin: 0; }
  .count {
    min-width: 20px; height: 20px; padding: 0 6px; box-sizing: border-box;
    display: inline-grid; place-items: center;
    background: var(--surface-2); color: var(--text-2);
    border-radius: 999px; font-size: 11px; font-weight: 800;
  }

  .add-wrap { position: relative; }
  .add-btn {
    display: inline-flex; align-items: center; gap: 6px;
    border: 1px solid var(--border-2); background: var(--bg-2); color: var(--text-1);
    font-size: 12px; font-weight: 700; padding: 8px 13px; border-radius: 9px; cursor: pointer;
    transition: background var(--dur), border-color var(--dur), box-shadow var(--dur);
  }
  .add-btn .plus { font-size: 14px; line-height: 1; color: var(--accent); font-weight: 800; }
  .add-btn:hover { border-color: var(--accent); background: var(--panel-2); }
  .add-btn.open { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }

  /* Opaque popover — sits above everything, nothing bleeds through. */
  .picker {
    position: absolute; right: 0; top: calc(100% + 10px); z-index: 60;
    background: var(--panel); border: 1px solid var(--border-2); border-radius: var(--r-md);
    box-shadow: 0 18px 44px rgba(0, 0, 0, 0.55), 0 0 0 1px rgba(0,0,0,0.3);
    padding: 7px; width: 224px;
    animation: pop 160ms var(--ease);
  }
  /* little caret pointing at the button */
  .picker::before {
    content: ''; position: absolute; top: -6px; right: 26px;
    width: 11px; height: 11px; background: var(--panel);
    border-left: 1px solid var(--border-2); border-top: 1px solid var(--border-2);
    transform: rotate(45deg);
  }
  @keyframes pop { from { opacity: 0; transform: translateY(-6px) scale(0.98); } to { opacity: 1; transform: none; } }

  .picker-title { margin: 5px 9px 7px; font-size: 10px; letter-spacing: 0.06em; color: var(--text-3); font-weight: 800; text-transform: uppercase; }
  .picker-item {
    display: flex; align-items: center; gap: 11px; width: 100%;
    border: none; background: transparent; color: var(--text-1);
    font-size: 13.5px; font-weight: 700; padding: 9px; border-radius: 9px; cursor: pointer;
    transition: background var(--dur), transform var(--dur);
  }
  .picker-name { flex: 1; text-align: left; }
  .picker-item .chevron { color: var(--text-3); font-size: 16px; opacity: 0; transform: translateX(-4px); transition: opacity var(--dur), transform var(--dur); }
  .picker-item:hover { background: var(--panel-2); }
  .picker-item:hover .chevron { opacity: 1; transform: none; }

  .empty {
    text-align: center; padding: 30px 4px 26px;
    display: flex; flex-direction: column; gap: 5px;
  }
  .empty p { margin: 0; color: var(--text-1); font-weight: 700; font-size: 14px; }
  .empty span { color: var(--text-3); font-size: 12.5px; }
  .empty strong { color: var(--text-2); }

  .list { list-style: none; margin: 0; padding: 0; }
  .list li {
    display: flex; align-items: center; gap: 13px;
    padding: 11px 12px;
    border-radius: var(--r-md);
    border: 1px solid transparent;
    transition: background var(--dur), border-color var(--dur);
  }
  .list li + li { margin-top: 4px; }
  .list li:hover { background: var(--surface); border-color: var(--border); }

  .tile {
    width: 36px; height: 36px; flex: 0 0 auto;
    display: grid; place-items: center;
    background: color-mix(in srgb, var(--wa) 12%, transparent);
    border-radius: 10px;
    color: var(--wa);
  }
  .tile :global(svg) { width: 18px; height: 18px; }
  .tile :global(img) { width: 22px; height: 22px; object-fit: contain; }

  .info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .name { color: var(--text-0); font-weight: 700; font-size: 14px; }
  .meta { display: flex; align-items: center; gap: 7px; flex-wrap: wrap; }
  .chip {
    font-size: 10.5px; font-weight: 700; color: var(--text-2);
    background: var(--surface-2); padding: 2px 7px; border-radius: 6px;
  }
  .chip.refresh { color: var(--accent-2); background: rgba(52, 224, 216, 0.1); }
  .mode { color: var(--text-3); font-size: 12px; }

  .pen, .del {
    border: none; background: transparent; cursor: pointer;
    font-size: 13px; width: 28px; height: 28px; border-radius: 8px;
    display: grid; place-items: center;
    color: var(--text-3);
    opacity: 0; transition: opacity var(--dur), color var(--dur), background var(--dur);
  }
  .list li:hover .pen, .list li:hover .del { opacity: 1; }
  .pen:hover { color: var(--text-0); background: var(--surface-2); }
  .del:hover { color: var(--err); background: rgba(242, 104, 138, 0.12); }

  .note { color: var(--text-3); font-size: 12px; margin-top: 22px; }

  /* Modal */
  .backdrop {
    position: fixed; inset: 0; z-index: 80;
    background: rgba(4, 6, 10, 0.6);
    display: grid; place-items: center;
    backdrop-filter: blur(4px);
    animation: fadein 160ms var(--ease);
  }
  @keyframes fadein { from { opacity: 0; } to { opacity: 1; } }
  /* Fills the backdrop and sits behind the dialog, so a click anywhere outside
     the dialog lands on it. Transparent: .backdrop already draws the dim. */
  .scrim {
    position: absolute; inset: 0;
    border: none; padding: 0; margin: 0;
    background: transparent; cursor: default;
  }
  .modal {
    position: relative; /* above .scrim, which is absolutely positioned behind it */
    width: 400px; max-width: calc(100vw - 40px);
    max-height: calc(100vh - 60px); overflow: auto;
    background: var(--panel); border: 1px solid var(--border-2);
    border-radius: var(--r-lg);
    box-shadow: 0 24px 60px rgba(0, 0, 0, 0.6);
    padding: 20px 22px 18px;
    animation: pop 200ms var(--ease);
  }
  .modal-head { display: flex; align-items: center; gap: 11px; margin-bottom: 16px; }
  .modal-head h3 { flex: 1; margin: 0; font-size: 15px; color: var(--text-0); }
  .modal-head .tile { width: 30px; height: 30px; }
  .x {
    border: none; background: transparent; color: var(--text-3); cursor: pointer;
    font-size: 20px; line-height: 1; width: 28px; height: 28px; border-radius: 8px;
    transition: color var(--dur), background var(--dur);
  }
  .x:hover { color: var(--text-0); background: var(--surface-2); }

  .preview { margin-bottom: 16px; pointer-events: none; }

  .field { display: block; margin-bottom: 14px; }
  .field > span { display: block; font-size: 10.5px; letter-spacing: 0.05em; font-weight: 800; color: var(--text-3); text-transform: uppercase; margin-bottom: 7px; }
  .field input {
    width: 100%; box-sizing: border-box;
    background: var(--bg-1); border: 1px solid var(--border-2); border-radius: 9px;
    padding: 9px 11px; color: var(--text-0); font-size: 13px;
    transition: border-color var(--dur), box-shadow var(--dur);
  }
  .field input:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }

  .row { display: flex; gap: 16px; }
  .row .field { flex: 1; }

  .radios { display: flex; gap: 3px; flex-wrap: wrap; background: var(--bg-1); padding: 3px; border-radius: 10px; border: 1px solid var(--border); }
  .radio {
    border: none; background: transparent; color: var(--text-2);
    font-size: 12px; font-weight: 700; padding: 6px 11px;
    border-radius: 7px; cursor: pointer;
    transition: background var(--dur), color var(--dur);
  }
  .radio:hover { color: var(--text-0); }
  .radio.active { background: var(--panel-3); color: var(--text-0); box-shadow: var(--shadow-soft); }

  .modal-foot { display: flex; align-items: center; gap: 8px; margin-top: 8px; }
  .spacer { flex: 1; }
  .modal-foot button {
    border: none; border-radius: 9px; padding: 9px 16px;
    font-size: 12.5px; font-weight: 800; cursor: pointer;
    transition: filter var(--dur), background var(--dur), color var(--dur);
  }
  .danger { background: transparent; color: var(--err); padding-left: 4px; }
  .danger:hover { text-decoration: underline; }
  .ghost { background: var(--surface-2); color: var(--text-2); }
  .ghost:hover { color: var(--text-0); }
  .primary { background: linear-gradient(120deg, var(--accent), var(--accent-2)); color: #0b0f17; }
  .primary:hover { filter: brightness(1.08); }
</style>
