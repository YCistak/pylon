<script>
  import { CATALOG, catalogEntry, modeOf, REFRESH_OPTIONS, widgets } from './widgets.js'
  import Widget from './Widget.svelte'

  export let editing = null // widget instance handed in from App (pen icon)

  let showPicker = false
  let draft = null   // widget instance being created/edited in the modal
  let isNew = false

  function openCreate(type) {
    const entry = catalogEntry(type)
    const mode = entry.modes[0]
    draft = {
      id: null, type, title: entry.title, column: 'left',
      mode: mode.id, params: {}, refresh: 0, accent: entry.accent,
    }
    isNew = true
    showPicker = false
  }

  function openEdit(instance) {
    draft = { ...instance, params: { ...instance.params } }
    isNew = false
  }

  // Pen icon on Home hands us an instance via bind:editing.
  $: if (editing) { openEdit(editing); editing = null }

  function closeModal() { draft = null }

  function setMode(modeId) {
    draft = { ...draft, mode: modeId, params: {} }
  }

  function save() {
    const { id, type, title, column, mode, params, refresh } = draft
    const patch = { title, column, mode, params, refresh }
    if (isNew) widgets.add(type, patch)
    else widgets.update(id, patch)
    closeModal()
  }

  function remove() {
    widgets.remove(draft.id)
    closeModal()
  }

  function onKeydown(e) {
    if (e.key === 'Escape' && draft) closeModal()
  }

  $: draftEntry = draft && catalogEntry(draft.type)
  $: draftMode = draft && modeOf(draft.type, draft.mode)
</script>

<svelte:window on:keydown={onKeydown} />

<div class="settings">
  <header class="head">
    <h2>Ayarlar</h2>
    <p class="sub">Widget'ları buradan ekle, düzenle veya sil. Ana sayfa yalnızca burada eklediklerini gösterir.</p>
  </header>

  <section class="card">
    <div class="card-head">
      <h3>Widget'lar</h3>
      <div class="add-wrap">
        <button class="add-btn" on:click={() => (showPicker = !showPicker)}>+ Widget Ekle</button>
        {#if showPicker}
          <div class="picker">
            <p class="picker-title">Hangi widget?</p>
            {#each CATALOG as c}
              <button class="picker-item" on:click={() => openCreate(c.type)}>
                <span class="tile" style="--wa: {c.accent}">{@html c.icon}</span>
                <span>{c.title}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    {#if $widgets.length === 0}
      <p class="empty">Henüz widget yok — yukarıdan ekle.</p>
    {:else}
      <ul class="list">
        {#each $widgets as w (w.id)}
          {@const entry = catalogEntry(w.type)}
          <li>
            <span class="tile" style="--wa: {entry.accent}">{@html entry.icon}</span>
            <span class="name">{w.title}</span>
            <span class="meta">{w.column === 'left' ? 'Sol' : 'Sağ'} · {modeOf(w.type, w.mode)?.label}</span>
            <button class="pen" on:click={() => openEdit(w)} title="düzenle" aria-label="düzenle">✎</button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <p class="note">Parola ekleme, servis aç/kapa ve ses ayarları — 4. adımda gelecek.</p>
</div>

{#if draft}
  <div class="backdrop" on:click={closeModal}>
    <div class="modal" on:click|stopPropagation>
      <header class="modal-head">
        <span class="tile" style="--wa: {draftEntry.accent}">{@html draftEntry.icon}</span>
        <h3>{isNew ? `${draftEntry.title} ekle` : `${draftEntry.title} düzenle`}</h3>
        <button class="x" on:click={closeModal} aria-label="kapat">×</button>
      </header>

      <div class="preview">
        <Widget
          icon={draftEntry.icon}
          title={draft.title}
          action={draftMode.action}
          params={draft.params}
          accent={draftEntry.accent}
        />
      </div>

      <label class="field">
        <span>Başlık</span>
        <input type="text" bind:value={draft.title} />
      </label>

      {#if draftEntry.modes.length > 1}
        <div class="field">
          <span>Ne göster?</span>
          <div class="radios">
            {#each draftEntry.modes as m}
              <button class="radio" class:active={draft.mode === m.id} on:click={() => setMode(m.id)}>
                {m.label}
              </button>
            {/each}
          </div>
        </div>
      {/if}

      {#each draftMode.params as p (p.key)}
        <label class="field">
          <span>{p.label}</span>
          <input
            type="text"
            placeholder={p.placeholder || ''}
            bind:value={draft.params[p.key]}
          />
        </label>
      {/each}

      <div class="row">
        <label class="field">
          <span>Sütun</span>
          <div class="radios">
            <button class="radio" class:active={draft.column === 'left'}  on:click={() => (draft.column = 'left')}>Sol</button>
            <button class="radio" class:active={draft.column === 'right'} on:click={() => (draft.column = 'right')}>Sağ</button>
          </div>
        </label>

        <label class="field">
          <span>Otomatik yenile</span>
          <div class="radios">
            {#each REFRESH_OPTIONS as r}
              <button class="radio" class:active={draft.refresh === r.value} on:click={() => (draft.refresh = r.value)}>
                {r.label}
              </button>
            {/each}
          </div>
        </label>
      </div>

      <footer class="modal-foot">
        {#if !isNew}
          <button class="danger" on:click={remove}>Sil</button>
        {/if}
        <span class="spacer"></span>
        <button class="ghost" on:click={closeModal}>İptal</button>
        <button class="primary" on:click={save}>{isNew ? 'Ekle' : 'Kaydet'}</button>
      </footer>
    </div>
  </div>
{/if}

<style>
  .settings { flex: 1; padding: 40px 44px; overflow: auto; max-width: 720px; }
  .head h2 { margin: 0 0 6px; color: var(--text-0); font-size: 24px; }
  .sub { color: var(--text-2); line-height: 1.55; margin: 0 0 26px; max-width: 520px; }

  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    padding: 8px 20px 16px;
  }
  .card-head { display: flex; align-items: center; justify-content: space-between; margin: 16px 4px 6px; }
  .card-head h3 { color: var(--text-1); font-size: 14px; margin: 0; }

  .add-wrap { position: relative; }
  .add-btn {
    border: 1px solid var(--border-2); background: var(--bg-2); color: var(--text-1);
    font-size: 12px; font-weight: 700; padding: 7px 12px; border-radius: 8px; cursor: pointer;
    transition: background var(--dur), border-color var(--dur);
  }
  .add-btn:hover { border-color: var(--accent); }

  .picker {
    position: absolute; right: 0; top: calc(100% + 8px); z-index: 20;
    background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-md);
    box-shadow: var(--shadow-soft); padding: 8px; width: 200px;
  }
  .picker-title { margin: 4px 8px 8px; font-size: 11px; color: var(--text-3); font-weight: 700; text-transform: uppercase; }
  .picker-item {
    display: flex; align-items: center; gap: 10px; width: 100%;
    border: none; background: transparent; color: var(--text-1);
    font-size: 13px; font-weight: 600; padding: 8px; border-radius: 7px; cursor: pointer;
    transition: background var(--dur);
  }
  .picker-item:hover { background: var(--bg-2); }

  .empty { color: var(--text-3); font-size: 13px; padding: 14px 4px; }

  .list { list-style: none; margin: 0; padding: 0; }
  .list li {
    display: flex; align-items: center; gap: 14px;
    padding: 14px 4px;
    border-top: 1px solid var(--border);
  }
  .list li:first-child { border-top: none; }

  .tile {
    width: 34px; height: 34px; flex: 0 0 auto;
    display: grid; place-items: center;
    color: var(--wa);
  }
  .tile :global(svg) { width: 18px; height: 18px; }
  .tile :global(img) { width: 22px; height: 22px; object-fit: contain; }
  .name { flex: 1; color: var(--text-1); font-weight: 700; }
  .meta { color: var(--text-3); font-size: 12px; }

  .pen {
    border: none; background: transparent; color: var(--text-3); cursor: pointer;
    font-size: 13px; transition: color var(--dur);
  }
  .pen:hover { color: var(--text-1); }

  .note { color: var(--text-3); font-size: 12px; margin-top: 22px; }

  /* Modal */
  .backdrop {
    position: fixed; inset: 0; z-index: 50;
    background: rgba(0, 0, 0, 0.45);
    display: grid; place-items: center;
    backdrop-filter: blur(2px);
  }
  .modal {
    width: 380px; max-width: calc(100vw - 40px);
    max-height: calc(100vh - 60px); overflow: auto;
    background: var(--surface); border: 1px solid var(--border-2);
    border-radius: var(--r-lg); box-shadow: var(--shadow-soft);
    padding: 18px 20px 16px;
  }
  .modal-head { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
  .modal-head h3 { flex: 1; margin: 0; font-size: 15px; color: var(--text-0); }
  .modal-head .tile { width: 28px; height: 28px; }
  .x {
    border: none; background: transparent; color: var(--text-3); cursor: pointer;
    font-size: 20px; line-height: 1; padding: 0 4px;
  }
  .x:hover { color: var(--text-1); }

  .preview { margin-bottom: 14px; pointer-events: none; }

  .field { display: block; margin-bottom: 12px; }
  .field > span { display: block; font-size: 11px; font-weight: 700; color: var(--text-3); text-transform: uppercase; margin-bottom: 6px; }
  .field input {
    width: 100%; box-sizing: border-box;
    background: var(--bg-2); border: 1px solid var(--border-2); border-radius: 8px;
    padding: 8px 10px; color: var(--text-0); font-size: 13px;
  }
  .field input:focus { outline: none; border-color: var(--accent); }

  .row { display: flex; gap: 16px; }
  .row .field { flex: 1; }

  .radios { display: flex; gap: 4px; flex-wrap: wrap; background: var(--bg-2); padding: 3px; border-radius: 9px; }
  .radio {
    border: none; background: transparent; color: var(--text-2);
    font-size: 12px; font-weight: 600; padding: 5px 10px;
    border-radius: 7px; cursor: pointer;
    transition: background var(--dur), color var(--dur);
  }
  .radio.active { background: var(--surface-2); color: var(--text-0); }

  .modal-foot { display: flex; align-items: center; gap: 8px; margin-top: 6px; }
  .spacer { flex: 1; }
  .modal-foot button {
    border: none; border-radius: 8px; padding: 8px 14px;
    font-size: 12px; font-weight: 700; cursor: pointer;
  }
  .danger { background: transparent; color: var(--err); }
  .danger:hover { text-decoration: underline; }
  .ghost { background: var(--bg-2); color: var(--text-2); }
  .ghost:hover { color: var(--text-1); }
  .primary { background: linear-gradient(120deg, var(--accent), var(--accent-2)); color: #fff; }
</style>
