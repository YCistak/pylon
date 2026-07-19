<script>
  // Connected accounts. Sign in with Google to let Pylon reach your calendar,
  // mail and Drive. The OAuth consent runs in the daemon (same flow as
  // `pylon auth google`); when this build has no OAuth client embedded yet the
  // row shows as not-yet-available rather than a dead button.
  import { onMount } from 'svelte'
  import { GoogleStatus, GoogleLogin } from '../../wailsjs/go/main/App.js'

  let status = 'unavailable' // 'connected' | 'ready' | 'unavailable'
  let busy = false
  let error = ''

  async function refresh() {
    try { status = await GoogleStatus() } catch { status = 'unavailable' }
  }
  onMount(refresh)

  async function login() {
    if (busy || status !== 'ready') return
    busy = true
    error = ''
    try {
      await GoogleLogin()
      await refresh()
    } catch (e) {
      error = String(e?.message || e)
    } finally {
      busy = false
    }
  }
</script>

<section class="card">
  <div class="card-head">
    <div class="card-title"><h3>Bağlı Hesaplar</h3></div>
  </div>

  <div class="acct">
    <span class="logo" aria-hidden="true">G</span>
    <div class="info">
      <span class="name">Google</span>
      <span class="sub">Takvim · Mail · Drive</span>
    </div>

    {#if status === 'connected'}
      <span class="badge">Bağlı ✓</span>
    {:else if status === 'ready'}
      <button class="signin" on:click={login} disabled={busy}>
        {busy ? 'Tarayıcı açıldı…' : 'Google ile Giriş Yap'}
      </button>
    {:else}
      <button class="signin" disabled title="Bu sürümde henüz aktif değil">
        Google ile Giriş Yap
      </button>
    {/if}
  </div>

  {#if status === 'unavailable'}
    <p class="note">Google girişi bu sürümde henüz aktif değil — yakında.</p>
  {:else if error}
    <p class="note err">{error}</p>
  {/if}
</section>

<style>
  .card {
    background: var(--surface);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.08));
    border-radius: 16px;
    padding: 18px 20px;
  }
  .card-head { margin-bottom: 12px; }
  .card-title h3 { margin: 0; font-size: 15px; color: var(--text-1); }

  .acct { display: flex; align-items: center; gap: 12px; }
  .logo {
    flex: none;
    width: 34px; height: 34px; border-radius: 50%;
    display: grid; place-items: center;
    font-weight: 800; font-size: 16px;
    color: var(--text-1);
    background: var(--surface-2);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.12));
  }
  .info { display: flex; flex-direction: column; flex: 1; min-width: 0; }
  .name { font-size: 14px; font-weight: 600; color: var(--text-1); }
  .sub { font-size: 12px; color: var(--text-2); }

  .badge {
    font-size: 11px; font-weight: 700; color: var(--accent-2);
    background: rgba(52, 224, 216, 0.12); border: 1px solid rgba(52, 224, 216, 0.3);
    padding: 4px 10px; border-radius: 999px;
  }
  .signin {
    flex: none;
    padding: 9px 16px; border-radius: 10px;
    font: inherit; font-size: 13px; font-weight: 600; cursor: pointer;
    color: var(--text-1);
    background: var(--surface-2);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.14));
    transition: border-color 160ms, background 160ms;
  }
  .signin:hover:not(:disabled) { border-color: var(--accent); }
  .signin:disabled { opacity: 0.5; cursor: default; }

  .note { margin: 12px 2px 0; font-size: 12px; line-height: 1.45; color: var(--text-2); }
  .note.err { color: #f98080; }
</style>
