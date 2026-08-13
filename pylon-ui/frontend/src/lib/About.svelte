<script>
  import { GUIVersion, RestartDaemon, UpdateApply, UpdateCheck, Version } from '../../wailsjs/go/main/App.js'
  import { daemonOnline } from './daemon.js'
  import { t } from './i18n.js'

  // Two binaries built from one tag, shown side by side because they come apart
  // in the one case this screen exists for: an update replaces the daemon on
  // disk, and this window keeps running the old code until it is closed and
  // opened again. A single version number here would be a lie for as long as
  // that lasts, and the user would have no way to see it.
  let daemonVersion = ''
  let guiVersion = ''

  let busy = false
  let message = ''   // whatever the daemon last said, already translated
  let error = ''
  let available = false
  let newVersion = ''
  let installed = false

  $: offline = $daemonOnline !== true
  $: mismatched = daemonVersion && guiVersion && daemonVersion !== guiVersion

  // Versions are read when the daemon answers rather than once at mount: the
  // window often opens before a cold-started daemon is listening, and a dash
  // that never fills in reads as a broken screen.
  $: if (!offline && !daemonVersion) load()

  async function load() {
    guiVersion = await GUIVersion()
    try {
      daemonVersion = await Version()
    } catch (e) {
      error = String(e?.message || e)
    }
  }

  async function check() {
    if (busy || offline) return
    busy = true
    error = ''
    message = ''
    installed = false
    try {
      // "<available>\t<version>\t<message>" — the daemon decides, because
      // deciding it here would mean matching on translated prose.
      const [flag, version, text] = (await UpdateCheck()).split('\t')
      available = flag === 'true'
      newVersion = version || ''
      message = text || ''
    } catch (e) {
      error = String(e?.message || e)
    } finally {
      busy = false
    }
  }

  async function install() {
    if (busy || offline || !available) return
    busy = true
    error = ''
    try {
      message = await UpdateApply()
      available = false
      installed = true
    } catch (e) {
      error = String(e?.message || e)
    } finally {
      busy = false
    }
  }

  function restart() {
    RestartDaemon()
    daemonVersion = '' // re-read once it answers again
  }
</script>

<section class="card">
  <h3>{$t('ui.about.versions')}</h3>

  <dl class="versions">
    <!-- i18n-raw: product names and version strings are the same in every
         language, and the value is whatever the build stamped. -->
    <dt>Pylon</dt>
    <dd>{daemonVersion || '—'}</dd>
    <dt>{$t('ui.about.interface')}</dt>
    <dd>{guiVersion || '—'}</dd>
  </dl>

  {#if mismatched}
    <p class="warn">{$t('ui.about.mismatch')}</p>
  {/if}
</section>

<section class="card">
  <h3>{$t('ui.about.update')}</h3>
  <p class="hint">{$t('ui.about.update_hint')}</p>

  <div class="row">
    <button class="btn" on:click={check} disabled={busy || offline}>
      {busy && !available ? $t('ui.about.checking') : $t('ui.about.check')}
    </button>

    {#if available}
      <button class="btn primary" on:click={install} disabled={busy}>
        {busy ? $t('ui.about.installing') : $t('ui.about.install', newVersion)}
      </button>
    {/if}

    {#if installed}
      <button class="btn primary" on:click={restart} disabled={busy}>
        {$t('ui.about.restart')}
      </button>
    {/if}
  </div>

  {#if error}
    <p class="err">{error}</p>
  {:else if message}
    <p class="msg">{message}</p>
  {/if}

  {#if installed}
    <p class="hint">{$t('ui.about.reopen')}</p>
  {/if}
</section>

<style>
  .card {
    padding: 18px 20px;
    border-radius: var(--r-md);
    background: var(--surface);
    border: 1px solid var(--border);
  }
  h3 { margin: 0 0 4px; font-size: 15px; color: var(--text-0); }
  .hint { margin: 6px 0 0; font-size: 13px; color: var(--text-2); line-height: 1.45; }

  /* A definition list, because that is what this is: two labelled values. The
     grid keeps the numbers in one column so a mismatch is visible as a
     difference rather than something to read for. */
  .versions {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 4px 16px;
    margin: 12px 0 0;
    font-size: 14px;
  }
  .versions dt { color: var(--text-2); }
  .versions dd { margin: 0; color: var(--text-1); font-variant-numeric: tabular-nums; }

  .warn { margin: 12px 0 0; font-size: 13px; color: var(--warn); line-height: 1.45; }

  .row { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 14px; }
  .btn {
    padding: 9px 16px;
    border-radius: 10px;
    font: inherit;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    color: var(--text-1);
    background: var(--surface-2);
    border: 1px solid var(--border-2);
  }
  .btn:disabled { opacity: 0.5; cursor: default; }
  .btn.primary {
    background: linear-gradient(135deg, var(--accent), var(--accent-2));
    border-color: transparent;
    color: #0b0f17;
  }

  .msg { margin: 12px 0 0; font-size: 13px; color: var(--text-1); white-space: pre-line; }
  .err { margin: 12px 0 0; font-size: 13px; color: var(--err); }
</style>
