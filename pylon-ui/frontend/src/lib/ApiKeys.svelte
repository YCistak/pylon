<script>
  // API key entry. Pylon's language model (Gemini by default) powers the voice
  // assistant and anything the local matcher can't resolve. The key is saved to
  // the daemon's encrypted vault (never to config in plaintext); the daemon is
  // then bounced so it picks the key up.
  import { onMount } from 'svelte'
  import { SetSecret, HasSecret, RestartDaemon } from '../../wailsjs/go/main/App.js'

  const NAME = 'gemini' // vault name buildIntentChain reads as secret:gemini

  let value = ''
  let saved = false // a key is already stored
  let busy = false
  let status = '' // '', 'ok', or an error message

  onMount(async () => {
    try { saved = await HasSecret(NAME) } catch {}
  })

  async function save() {
    const v = value.trim()
    if (!v || busy) return
    busy = true
    status = ''
    try {
      await SetSecret(NAME, v)
      await RestartDaemon() // apply it now
      saved = true
      value = ''
      status = 'ok'
    } catch (e) {
      status = String(e?.message || e)
    } finally {
      busy = false
    }
  }
</script>

<section class="card">
  <div class="card-head">
    <div class="card-title"><h3>Yapay Zekâ API Anahtarı</h3></div>
    {#if saved}<span class="badge">kayıtlı ✓</span>{/if}
  </div>
  <p class="hint">
    Sesli asistanı ve akıllı komutları çalıştıran dil modelinin (Gemini) anahtarı.
    Şifreli kasaya kaydedilir. Ücretsiz anahtar: <code>aistudio.google.com/apikey</code>
  </p>

  <div class="row">
    <input
      class="key"
      type="password"
      placeholder={saved ? 'yeni anahtarla değiştir…' : 'API anahtarını yapıştır'}
      bind:value
      on:keydown={(e) => e.key === 'Enter' && save()}
      autocomplete="off"
      spellcheck="false"
    />
    <button class="save" on:click={save} disabled={busy || !value.trim()}>
      {busy ? 'Kaydediliyor…' : 'Kaydet'}
    </button>
  </div>

  {#if status === 'ok'}
    <p class="note ok">Kaydedildi — Pylon yenilendi.</p>
  {:else if status}
    <p class="note err">{status}</p>
  {/if}
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
  .badge {
    font-size: 11px; font-weight: 700; color: var(--accent-2);
    background: rgba(52, 224, 216, 0.12); border: 1px solid rgba(52, 224, 216, 0.3);
    padding: 2px 8px; border-radius: 999px;
  }
  .hint { margin: 0 0 14px; font-size: 13px; line-height: 1.5; color: var(--text-2); }
  .hint code { font-size: 12px; background: var(--surface-2); padding: 1px 5px; border-radius: 5px; }

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

  .note { margin: 10px 2px 0; font-size: 12px; }
  .note.ok { color: var(--accent-2); }
  .note.err { color: #f98080; }
</style>
