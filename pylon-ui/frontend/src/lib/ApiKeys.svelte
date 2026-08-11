<script>
  import { t } from './i18n.js'
  // Credential entry. Everything here goes to the daemon's encrypted vault
  // (AES-256-GCM) under the name the shipped pylon.yaml already references as
  // `secret:<name>`, so saving one is all it takes — no config editing, and the
  // value is never written to disk in plaintext.
  import { onMount } from 'svelte'
  import { SetSecret, HasSecret, DeleteSecret, RestartDaemon } from '../../wailsjs/go/main/App.js'

  // name must match the `secret:<name>` reference in pylon.yaml.
  const KEYS = [
    {
      name: 'gemini',
      title: 'ui.keys.gemini',
      hint: 'ui.keys.gemini_hint',
    },
    {
      name: 'github',
      title: 'GitHub',
      hint: 'ui.keys.github_hint',
    },
    {
      name: 'freshrss',
      title: 'FreshRSS',
      hint: 'ui.keys.freshrss_hint',
    },
  ]

  let value = {} // name → what is typed right now
  let saved = {} // name → a credential is stored
  let busy = {}
  let confirming = '' // name whose deletion is awaiting confirmation
  let note = {} // name → 'ok' | 'silindi' | an error message

  onMount(refresh)

  async function refresh() {
    for (const k of KEYS) {
      try {
        saved[k.name] = await HasSecret(k.name)
      } catch {
        saved[k.name] = false
      }
    }
    saved = saved // Svelte 3 needs the assignment to see the mutation
  }

  async function save(name) {
    const v = (value[name] || '').trim()
    if (!v || busy[name]) return
    busy = { ...busy, [name]: true }
    note = { ...note, [name]: '' }
    try {
      await SetSecret(name, v)
      RestartDaemon() // apply it now
      saved = { ...saved, [name]: true }
      value = { ...value, [name]: '' }
      note = { ...note, [name]: 'ok' }
    } catch (e) {
      note = { ...note, [name]: String(e?.message || e) }
    } finally {
      busy = { ...busy, [name]: false }
    }
  }

  // Deleting is the one vault operation with no undo — the plaintext is gone and
  // only the user can produce it again — so it asks first. confirm() is not
  // reliably available inside the webview, hence the inline two-step.
  async function remove(name) {
    if (busy[name]) return
    confirming = ''
    busy = { ...busy, [name]: true }
    note = { ...note, [name]: '' }
    try {
      await DeleteSecret(name)
      RestartDaemon() // drop the service that was using it
      saved = { ...saved, [name]: false }
      note = { ...note, [name]: 'silindi' }
    } catch (e) {
      note = { ...note, [name]: String(e?.message || e) }
    } finally {
      busy = { ...busy, [name]: false }
    }
  }
</script>

<section class="card">
  <div class="card-head">
    <div class="card-title"><h3>{$t('ui.keys.title')}</h3></div>
  </div>
  <p class="lead">
    {$t('ui.keys.note')}
  </p>

  {#each KEYS as k (k.name)}
    <div class="key-row">
      <div class="key-head">
        <span class="key-title">{$t(k.title)}</span>
        {#if saved[k.name]}<span class="badge">{$t('ui.keys.saved')}</span>{/if}
      </div>
      <p class="hint">{$t(k.hint)}</p>

      <div class="row">
        <input
          class="key"
          type="password"
          placeholder={saved[k.name] ? $t('ui.keys.replace') : $t('ui.keys.paste')}
          bind:value={value[k.name]}
          on:keydown={(e) => e.key === 'Enter' && save(k.name)}
          autocomplete="off"
          spellcheck="false"
        />
        <button class="save" on:click={() => save(k.name)} disabled={busy[k.name] || !(value[k.name] || '').trim()}>
          {busy[k.name] ? $t('ui.saving') : $t('ui.save')}
        </button>
      </div>

      {#if confirming === k.name}
        <p class="note confirm">
          {$t('ui.keys.delete_ask')}
          <button class="danger" on:click={() => remove(k.name)} disabled={busy[k.name]}>{$t('ui.delete')}</button>
          <button class="link" on:click={() => (confirming = '')}>{$t('ui.cancel')}</button>
        </p>
      {:else if saved[k.name]}
        <p class="note">
          <button class="link" on:click={() => (confirming = k.name)} disabled={busy[k.name]}>{$t('ui.keys.delete')}</button>
        </p>
      {/if}

      {#if note[k.name] === 'ok'}
        <p class="note ok">{$t('ui.keys.saved_reloaded')}</p>
      {:else if note[k.name] === 'silindi'}
        <p class="note ok">{$t('ui.keys.deleted_reloaded')}</p>
      {:else if note[k.name]}
        <p class="note err">{note[k.name]}</p>
      {/if}
    </div>
  {/each}
</section>

<style>
  .card {
    background: var(--surface);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.08));
    border-radius: 16px;
    padding: 18px 20px;
  }
  .card-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; }
  .card-title h3 { margin: 0; font-size: 15px; color: var(--text-1); }
  .lead { margin: 0 0 4px; font-size: 13px; line-height: 1.5; color: var(--text-2); }

  .key-row { padding: 14px 0 2px; }
  .key-row + .key-row { border-top: 1px solid var(--border, rgba(255, 255, 255, 0.07)); }
  .key-head { display: flex; align-items: center; gap: 10px; margin-bottom: 4px; }
  .key-title { font-size: 14px; font-weight: 600; color: var(--text-1); }

  .badge {
    font-size: 11px; font-weight: 700; color: var(--accent-2);
    background: rgba(52, 224, 216, 0.12); border: 1px solid rgba(52, 224, 216, 0.3);
    padding: 2px 8px; border-radius: 999px;
  }
  .hint { margin: 0 0 12px; font-size: 12px; line-height: 1.5; color: var(--text-2); }

  .row { display: flex; gap: 10px; }
  .key {
    flex: 1; min-width: 0;
    padding: 9px 12px; border-radius: 10px;
    font: inherit; font-size: 13px;
    color: var(--text-1);
    background: var(--bg-0, rgba(0, 0, 0, 0.35));
    border: 1px solid var(--border, rgba(255, 255, 255, 0.12));
  }
  .key:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }
  .save {
    flex: none;
    padding: 9px 16px; border-radius: 10px;
    font: inherit; font-size: 13px; font-weight: 600; cursor: pointer;
    color: #0b0f17;
    background: linear-gradient(135deg, var(--accent), var(--accent-2));
    border: none;
  }
  .save:disabled { opacity: 0.5; cursor: default; }

  .note { margin: 10px 2px 0; font-size: 12px; display: flex; align-items: center; gap: 10px; }
  .note.ok { color: var(--accent-2); }
  .note.err { color: #f98080; }
  .note.confirm { color: var(--text-2); }

  .link {
    padding: 0;
    font: inherit; font-size: 12px; cursor: pointer;
    color: var(--text-2);
    background: none; border: none;
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  .link:hover:not(:disabled) { color: #f98080; }
  .link:disabled { opacity: 0.5; cursor: default; }

  .danger {
    padding: 5px 12px; border-radius: 9px;
    font: inherit; font-size: 12px; font-weight: 600; cursor: pointer;
    color: #f98080;
    background: rgba(249, 128, 128, 0.1);
    border: 1px solid rgba(249, 128, 128, 0.35);
  }
  .danger:disabled { opacity: 0.5; cursor: default; }
</style>
