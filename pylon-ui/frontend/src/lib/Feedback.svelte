<script>
  import { FeedbackEnv, OpenURL, SendFeedback } from '../../wailsjs/go/main/App.js'
  import { daemonOnline } from './daemon.js'
  import { t } from './i18n.js'

  // The ids are what GitHub labels the issue with, so they stay English and
  // untranslated; only what is on the button changes language.
  const CATEGORIES = [
    { id: 'bug', label: 'ui.feedback.bug' },
    { id: 'idea', label: 'ui.feedback.idea' },
    { id: 'question', label: 'ui.feedback.question' },
    { id: 'other', label: 'ui.feedback.other' },
  ]

  let category = CATEGORIES[0].id
  let body = ''
  let env = ''       // the diagnostics line, shown before anything is sent
  let sending = false
  let error = ''
  let issueURL = ''  // set once it is filed, so there is somewhere to go and look
  let opened = false // the fallback ran: the browser has the form instead

  $: offline = $daemonOnline !== true
  $: canSend = !sending && !offline && body.trim().length > 0

  // Fetched from the daemon rather than assembled here, so the line on screen
  // and the line in the issue cannot drift apart.
  $: if (!offline && !env) loadEnv()

  async function loadEnv() {
    try {
      env = await FeedbackEnv()
    } catch {
      // Not worth an error message: the box still works, and the report simply
      // arrives without the line.
      env = ''
    }
  }

  async function send() {
    if (!canSend) return
    sending = true
    error = ''
    issueURL = ''
    opened = false
    try {
      // "<how>\t<url>" — filed, or a prefilled page for the browser.
      const [how, url] = (await SendFeedback(category, body)).split('\t')
      if (how === 'sent') {
        issueURL = url
      } else {
        opened = true
        OpenURL(url)
      }
      body = ''
    } catch (e) {
      error = String(e?.message || e)
    } finally {
      sending = false
    }
  }
</script>

<section class="card">
  <h3>{$t('ui.feedback.title')}</h3>
  <p class="hint">{$t('ui.feedback.hint')}</p>

  <div class="cats" role="group" aria-label={$t('ui.feedback.category')}>
    {#each CATEGORIES as c}
      <button
        class="cat"
        class:active={category === c.id}
        aria-pressed={category === c.id}
        on:click={() => (category = c.id)}
        disabled={sending}
      >{$t(c.label)}</button>
    {/each}
  </div>

  <textarea
    class="box"
    bind:value={body}
    rows="5"
    disabled={sending || offline}
    placeholder={$t('ui.feedback.placeholder')}
    aria-label={$t('ui.feedback.title')}
  ></textarea>

  <!-- Shown, not hidden: this is the whole of what Pylon attaches, and the user
       reads it before pressing Send rather than finding it in the issue after. -->
  {#if env}
    <p class="env">{$t('ui.feedback.attached')} <span class="mono">{env}</span></p>
  {/if}

  <div class="row">
    <button class="btn primary" on:click={send} disabled={!canSend}>
      {sending ? $t('ui.feedback.sending') : $t('ui.feedback.send')}
    </button>
  </div>

  {#if error}
    <p class="err">{error}</p>
  {:else if issueURL}
    <p class="ok">
      {$t('ui.feedback.sent')}
      <button class="link" on:click={() => OpenURL(issueURL)}>{$t('ui.feedback.open_issue')}</button>
    </p>
  {:else if opened}
    <p class="ok">{$t('ui.feedback.opened_browser')}</p>
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

  /* Four choices shown at once rather than a <select>: the list is short and
     fixed, and picking a category should not cost a popup. */
  .cats { display: flex; flex-wrap: wrap; gap: 8px; margin: 14px 0 10px; }
  .cat {
    padding: 7px 14px;
    border-radius: 999px;
    font: inherit;
    font-size: 13px;
    cursor: pointer;
    color: var(--text-2);
    background: var(--surface-2);
    border: 1px solid var(--border-2);
  }
  .cat.active {
    color: var(--text-0);
    background: var(--accent-soft);
    border-color: var(--accent);
  }
  .cat:disabled { opacity: 0.5; cursor: default; }

  .box {
    width: 100%;
    padding: 11px 14px;
    border-radius: var(--r-sm);
    font: inherit;
    font-size: 14px;
    line-height: 1.5;
    resize: vertical;
    color: var(--text-1);
    background: var(--bg-2);
    border: 1px solid var(--border-2);
    /* The one place in the app where typing prose is the point. */
    -webkit-user-select: text;
    user-select: text;
  }
  .box::placeholder { color: var(--text-3); }
  .box:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-glow);
  }
  .box:disabled { opacity: 0.5; }

  .env { margin: 10px 0 0; font-size: 12px; color: var(--text-3); line-height: 1.5; }
  .mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; color: var(--text-2); }

  .row { display: flex; gap: 10px; margin-top: 14px; }
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

  .ok { margin: 12px 0 0; font-size: 13px; color: var(--ok); }
  .err { margin: 12px 0 0; font-size: 13px; color: var(--err); }
  .link {
    padding: 0;
    font: inherit;
    font-size: 13px;
    color: var(--accent-2);
    background: none;
    border: none;
    cursor: pointer;
    text-decoration: underline;
  }
</style>
