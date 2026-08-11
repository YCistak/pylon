<script>
  import { fade, fly } from 'svelte/transition'
  import { Ask, Listen } from '../../wailsjs/go/main/App.js'
  import { daemonOnline } from './daemon.js'
  import { t } from './i18n.js'

  // Two ways into the same intent engine: push-to-talk, and typing. The mic is
  // the primary one, but it is useless in a quiet room or with the headset
  // unplugged, so the same questions can be typed — including "brifing ver".
  let listening = false
  let asking = false
  let typed = '' // what is in the box right now
  let heard = '' // what was transcribed or typed (shown dimmer, above the reply)
  let reply = '' // Pylon's answer
  let error = ''

  $: offline = $daemonOnline !== true
  $: busy = listening || asking

  // Clear the last exchange before starting a new one, so a stale answer never
  // sits under a fresh question.
  function reset() {
    heard = ''
    reply = ''
    error = ''
  }

  async function talk() {
    if (busy || offline) return
    listening = true
    reset()
    try {
      const out = await Listen()
      // The daemon returns "» <heard>\n<reply>"; split so we can style them.
      const nl = out.indexOf('\n')
      if (out.startsWith('» ') && nl !== -1) {
        heard = out.slice(2, nl).trim()
        reply = out.slice(nl + 1).trim()
      } else {
        reply = out
      }
    } catch (e) {
      error = String(e?.message || e)
    } finally {
      listening = false
    }
  }

  async function ask() {
    const question = typed.trim()
    if (busy || offline || !question) return
    asking = true
    reset()
    // Echoed straight away: the answer can take seconds, and the question
    // staying visible is what makes the wait read as progress.
    heard = question
    typed = ''
    try {
      reply = await Ask(question)
    } catch (e) {
      error = String(e?.message || e)
    } finally {
      asking = false
    }
  }
</script>

<div class="voicebar">
  <div class="buttons">
    <button
      class="btn talk"
      class:listening
      on:click={talk}
      disabled={busy || offline}
    >
      <span class="ic" aria-hidden="true">🎤</span>
      {listening ? $t('ui.voicebar.listening') : $t('ui.voicebar.talk')}
    </button>
  </div>

  <form class="askbar" on:submit|preventDefault={ask}>
    <input
      class="field"
      type="text"
      bind:value={typed}
      disabled={busy || offline}
      placeholder={offline ? $t('ui.voicebar.offline') : $t('ui.voicebar.placeholder')}
      aria-label={$t('ui.voicebar.write')}
    />
    <button
      class="btn send"
      type="submit"
      disabled={busy || offline || !typed.trim()}
    >
      <span class="ic" aria-hidden="true">↵</span>
      {asking ? $t('ui.voicebar.asking') : $t('ui.voicebar.send')}
    </button>
  </form>

  {#if error}
    <p class="line err" in:fade>{error}</p>
  {:else if reply}
    <div class="bubble" in:fly={{ y: 8, duration: 220 }}>
      {#if heard}<p class="heard">“{heard}”</p>{/if}
      <p class="reply">{reply}</p>
    </div>
  {/if}
</div>

<style>
  .voicebar {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 14px;
    /* A concrete width, not 100%: the stage centres its children, so it is
       shrink-to-fit and a percentage would resolve against the orb's 220px —
       leaving the ask box a sliver. */
    width: 420px;
    max-width: 100%;
  }
  .buttons { display: flex; gap: 12px; }

  .btn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 11px 18px;
    border-radius: 12px;
    font: inherit;
    font-weight: 600;
    font-size: 14px;
    cursor: pointer;
    border: 1px solid var(--border, rgba(255, 255, 255, 0.1));
    background: var(--surface-2);
    color: var(--text-1);
    transition: transform 120ms, background 160ms, border-color 160ms, box-shadow 160ms;
  }
  .btn:hover:not(:disabled) { transform: translateY(-1px); }
  .btn:disabled { opacity: 0.5; cursor: default; }
  .btn .ic { font-size: 15px; }

  .talk {
    background: linear-gradient(135deg, var(--accent), var(--accent-2));
    border-color: transparent;
    color: #0b0f17;
    box-shadow: 0 6px 20px var(--accent-glow);
  }
  .talk.listening {
    animation: pulse 1.1s ease-in-out infinite;
  }
  @keyframes pulse {
    0%, 100% { box-shadow: 0 0 0 0 var(--accent-glow); }
    50%      { box-shadow: 0 0 0 10px rgba(124, 140, 248, 0); }
  }

  .askbar {
    display: flex;
    gap: 8px;
    width: 100%;
  }
  .field {
    flex: 1;
    min-width: 0;
    padding: 10px 14px;
    border-radius: 12px;
    font: inherit;
    font-size: 14px;
    color: var(--text-1);
    background: var(--surface-2);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.1));
    transition: border-color 160ms, box-shadow 160ms;
  }
  .field::placeholder { color: var(--text-2); }
  .field:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-glow);
  }
  .field:disabled { opacity: 0.5; }
  .send { padding: 10px 16px; }

  .bubble {
    width: 100%;
    padding: 12px 16px;
    border-radius: 14px;
    background: var(--surface);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.08));
    text-align: left;
  }
  .heard { margin: 0 0 6px; font-size: 13px; color: var(--text-2); font-style: italic; }
  .reply { margin: 0; font-size: 15px; line-height: 1.45; color: var(--text-1); white-space: pre-line; }
  .line.err { margin: 0; font-size: 13px; color: #f98080; }
</style>
