<script>
  import { fade, fly } from 'svelte/transition'
  import { Listen, Briefing } from '../../wailsjs/go/main/App.js'
  import { daemonOnline } from './daemon.js'

  // One shared busy flag: the mic and the briefing both talk to the daemon and
  // shouldn't overlap (the mic holds the recorder; a second call would queue).
  let busy = null // null | 'listen' | 'briefing'
  let heard = '' // what the mic transcribed (shown dimmer, above the reply)
  let reply = '' // Pylon's answer / the briefing text
  let error = ''

  $: offline = $daemonOnline !== true

  async function talk() {
    if (busy || offline) return
    busy = 'listen'
    heard = ''
    reply = ''
    error = ''
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
      busy = null
    }
  }

  async function brief() {
    if (busy || offline) return
    busy = 'briefing'
    heard = ''
    reply = ''
    error = ''
    try {
      reply = await Briefing()
    } catch (e) {
      error = String(e?.message || e)
    } finally {
      busy = null
    }
  }
</script>

<div class="voicebar">
  <div class="buttons">
    <button
      class="btn talk"
      class:listening={busy === 'listen'}
      on:click={talk}
      disabled={!!busy || offline}
      title="Konuşarak sor"
    >
      <span class="ic" aria-hidden="true">🎤</span>
      {busy === 'listen' ? 'Dinliyorum…' : 'Konuş'}
    </button>

    <button
      class="btn brief"
      on:click={brief}
      disabled={!!busy || offline}
      title="Günün brifingini ver"
    >
      <span class="ic" aria-hidden="true">📰</span>
      {busy === 'briefing' ? 'Hazırlıyorum…' : 'Brifing Ver'}
    </button>
  </div>

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
    width: 100%;
    max-width: 460px;
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
