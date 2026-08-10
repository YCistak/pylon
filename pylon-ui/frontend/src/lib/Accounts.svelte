<script>
  import { t } from './i18n.js'
  // Connected accounts. Signing in lets Pylon reach the service on your behalf:
  // Google for calendar and Drive, Spotify for playback control. The OAuth
  // consent runs in the daemon (the same flow as `pylon auth <service>`); when
  // this build has no OAuth client embedded for a service its row shows as
  // not-yet-available rather than a dead button.
  import { AuthStatus, AuthLogin, AuthLogout, RestartDaemon } from '../../wailsjs/go/main/App.js'
  import { daemonOnline } from './daemon.js'

  // One entry per signable service. The daemon side is service-agnostic, so a
  // new integration only needs a row here.
  const ACCOUNTS = [
    { id: 'google', name: 'Google', sub: 'Takvim · Mail · Drive', logo: 'G' },
    { id: 'spotify', name: 'Spotify', sub: 'ui.accounts.spotify_sub', logo: '♪' },
  ]

  // Per-service state, keyed by id: 'connected' | 'ready' | 'unavailable' | 'offline'
  let status = Object.fromEntries(ACCOUNTS.map((a) => [a.id, 'offline']))
  let busy = {} // id → true while a browser flow or a sign-out is in flight
  let confirming = '' // id whose sign-out is awaiting confirmation
  let error = {} // id → message

  async function refresh() {
    for (const a of ACCOUNTS) {
      try {
        status[a.id] = await AuthStatus(a.id)
      } catch {
        status[a.id] = 'offline'
      }
    }
    status = status // Svelte 3 needs the assignment to see the mutation
  }

  // Follow the shared daemon probe rather than checking once on mount: the GUI
  // spawns the daemon in the background, so a cold launch would otherwise leave
  // this card stuck on whatever it saw before the socket answered. Same
  // change-only guard as Widget.svelte — the store re-publishes every 1.5s, and
  // asking the daemon that often would be pure noise.
  let lastOnline = undefined
  function onDaemonChange(online) {
    if (online === lastOnline) return
    lastOnline = online
    if (online === true) refresh()
    else if (online === false) {
      status = Object.fromEntries(ACCOUNTS.map((a) => [a.id, 'offline']))
    }
  }
  $: onDaemonChange($daemonOnline)

  async function login(id) {
    if (busy[id] || status[id] !== 'ready') return
    busy = { ...busy, [id]: true }
    error = { ...error, [id]: '' }
    try {
      await AuthLogin(id)
      // The service registry is built once at daemon startup, so without this
      // bounce the badge flips to "Bağlı" while the service's commands keep
      // failing until the user restarts Pylon themselves.
      RestartDaemon()
      await refresh()
    } catch (e) {
      error = { ...error, [id]: String(e?.message || e) }
    } finally {
      busy = { ...busy, [id]: false }
    }
  }

  // Signing out throws away a token that costs a browser round-trip to replace,
  // so it asks first — inline rather than via confirm(), which is not reliably
  // available inside the webview.
  async function logout(id) {
    if (busy[id]) return
    confirming = ''
    busy = { ...busy, [id]: true }
    error = { ...error, [id]: '' }
    try {
      await AuthLogout(id)
      RestartDaemon() // drop the service from the registry
      await refresh()
    } catch (e) {
      error = { ...error, [id]: String(e?.message || e) }
    } finally {
      busy = { ...busy, [id]: false }
    }
  }
</script>

<section class="card">
  <div class="card-head">
    <div class="card-title"><h3>{$t('ui.accounts.title')}</h3></div>
  </div>

  {#each ACCOUNTS as acct (acct.id)}
    <div class="acct">
      <span class="logo" aria-hidden="true">{acct.logo}</span>
      <div class="info">
        <span class="name">{acct.name}</span>
        <span class="sub">{acct.sub}</span>
      </div>

      {#if confirming === acct.id}
        <div class="confirm">
          <span class="ask">{$t('ui.accounts.signout_ask')}</span>
          <button class="danger" on:click={() => logout(acct.id)} disabled={busy[acct.id]}>Evet</button>
          <button class="signin" on:click={() => (confirming = '')}>{$t('ui.cancel')}</button>
        </div>
      {:else if status[acct.id] === 'connected'}
        <span class="badge">{$t('ui.accounts.connected')}</span>
        <button
          class="link"
          on:click={() => (confirming = acct.id)}
          disabled={busy[acct.id]}
          title={$t('ui.accounts.disconnect_title', acct.name)}
        >
          {busy[acct.id] ? $t('ui.accounts.signing_out') : $t('ui.accounts.sign_out')}
        </button>
      {:else if status[acct.id] === 'ready'}
        <button class="signin" on:click={() => login(acct.id)} disabled={busy[acct.id]}>
          {busy[acct.id] ? $t('ui.accounts.browser_opened') : $t('ui.accounts.connect')}
        </button>
      {:else if status[acct.id] === 'offline'}
        <button class="signin" disabled title={$t('ui.accounts.waiting')}>{$t('ui.status.connecting')}</button>
      {:else}
        <button class="signin" disabled title={$t('ui.accounts.unavailable_title')}>{$t('ui.accounts.connect')}</button>
      {/if}
    </div>

    {#if error[acct.id]}
      <p class="note err">{error[acct.id]}</p>
    {:else if status[acct.id] === 'unavailable'}
      <p class="note">{$t('ui.accounts.unavailable', acct.name)}</p>
    {/if}
  {/each}

  {#if $daemonOnline === false}
    <p class="note">{$t('ui.accounts.starting')}</p>
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
  .acct + .acct { margin-top: 14px; }
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

  /* Sign-out is secondary to the "Bağlı" badge it sits next to, so it reads as
     a link rather than competing with it as a second button. */
  .link {
    flex: none;
    padding: 4px 2px;
    font: inherit; font-size: 12px; cursor: pointer;
    color: var(--text-2);
    background: none; border: none;
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  .link:hover:not(:disabled) { color: #f98080; }
  .link:disabled { opacity: 0.5; cursor: default; }

  .confirm { display: flex; align-items: center; gap: 8px; }
  .ask { font-size: 12px; color: var(--text-2); }
  .danger {
    flex: none;
    padding: 7px 14px; border-radius: 10px;
    font: inherit; font-size: 13px; font-weight: 600; cursor: pointer;
    color: #f98080;
    background: rgba(249, 128, 128, 0.1);
    border: 1px solid rgba(249, 128, 128, 0.35);
  }
  .danger:disabled { opacity: 0.5; cursor: default; }

  .note { margin: 10px 2px 0; font-size: 12px; line-height: 1.45; color: var(--text-2); }
  .note.err { color: #f98080; }
</style>
