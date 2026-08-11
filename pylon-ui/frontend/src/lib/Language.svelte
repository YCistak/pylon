<script>
  // The language Pylon speaks. One setting, held by the daemon: picking here
  // changes what the assistant says, what this window is labelled, and what the
  // `pylon` command prints, all at once and without a restart.
  //
  // Every option is written in its own language and script. Someone who has
  // ended up in a language they cannot read has to be able to find their way
  // back out, and a list of names in the *current* language would not let them.
  import { LanguagePref } from '../../wailsjs/go/main/App.js'
  import { availableLanguages, lang, setLanguage, t } from './i18n.js'
  import { daemonOnline } from './daemon.js'

  let options = []   // [{ code, name }] from the daemon
  let chosen = ''    // the explicit choice; '' means "follow the system"
  let busy = ''      // code being applied, so only that button shows the wait
  let error = ''

  async function refresh() {
    options = await availableLanguages()
    try {
      chosen = await LanguagePref()
    } catch {
      chosen = ''
    }
  }

  // Same shape as Accounts.svelte: follow the shared probe rather than checking
  // once on mount, because the GUI may still be starting the daemon. The
  // change-only guard keeps this from re-asking every 1.5s.
  let lastOnline = undefined
  function onDaemonChange(online) {
    if (online === lastOnline) return
    lastOnline = online
    if (online === true) refresh()
    else if (online === false) options = []
  }
  $: onDaemonChange($daemonOnline)

  async function pick(code) {
    if (busy || code === chosen) return
    busy = code || 'auto'
    error = ''
    try {
      await setLanguage(code)
      chosen = code
    } catch (e) {
      // Say which language failed rather than only that something did: the
      // window is still in the old language, so without this the click just
      // looks ignored.
      error = String(e?.message || e)
    } finally {
      busy = ''
    }
  }
</script>

<section class="card">
  <div class="card-head">
    <div class="card-title"><h3>{$t('ui.language')}</h3></div>
  </div>
  <p class="hint">{$t('ui.language.hint')}</p>

  {#if options.length === 0}
    <p class="note">{$t('ui.language.offline')}</p>
  {:else}
    <div class="langs">
      <button
        class="pill"
        class:on={chosen === ''}
        disabled={!!busy}
        on:click={() => pick('')}
      >
        {$t('ui.language.system')}
        {#if chosen === ''}<span class="current">· {$lang}</span>{/if}
      </button>

      {#each options as o (o.code)}
        <button
          class="pill"
          class:on={chosen === o.code}
          disabled={!!busy}
          on:click={() => pick(o.code)}
          lang={o.code}
        >{o.name}</button>
      {/each}
    </div>
  {/if}

  {#if error}
    <p class="note err">{$t('ui.language.failed')} {error}</p>
  {/if}
</section>

<style>
  .card {
    background: var(--surface);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.08));
    border-radius: 16px;
    padding: 18px 20px;
  }
  .card-head { margin-bottom: 6px; }
  .card-title h3 { margin: 0; font-size: 15px; color: var(--text-1); }
  .hint { margin: 0 0 14px; font-size: 12.5px; line-height: 1.5; color: var(--text-2); }

  .langs { display: flex; flex-wrap: wrap; gap: 8px; }
  .pill {
    border: 1px solid var(--border-2); background: var(--bg-2); color: var(--text-1);
    font: inherit; font-size: 13px; font-weight: 600;
    padding: 9px 15px; border-radius: 10px; cursor: pointer;
    transition: background 160ms, border-color 160ms, color 160ms;
  }
  .pill:hover:not(:disabled) { border-color: var(--accent); background: var(--panel-2); }
  .pill:disabled { opacity: 0.55; cursor: default; }
  .pill.on {
    color: var(--accent-2);
    border-color: color-mix(in srgb, var(--accent-2) 45%, transparent);
    background: rgba(52, 224, 216, 0.1);
  }
  /* Which language "the system" actually resolved to — otherwise the option is
     a promise the card never keeps. */
  .current { opacity: 0.7; font-weight: 500; text-transform: uppercase; font-size: 11px; }

  .note { margin: 12px 2px 0; font-size: 12px; line-height: 1.45; color: var(--text-2); }
  .note.err { color: #f98080; }
</style>
