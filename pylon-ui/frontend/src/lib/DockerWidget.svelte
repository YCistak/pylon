<script>
  import { t } from './i18n.js'
  import { onDestroy } from 'svelte'
  import { fly, slide } from 'svelte/transition'
  import { Do } from '../../wailsjs/go/main/App.js'
  import { iconDocker } from './icons.js'

  export let title = 'Docker'
  export let params = {}
  export let refresh = 0 // minutes, 0 = off
  export let accent = '#2496ed'
  export let onEdit = null

  $: container = (params && params.container) || ''

  let state = 'loading' // loading | ok | error
  let running = null     // true | false | null
  let statusText = ''
  let statsText = ''
  let errText = ''
  let busy = ''          // '' | start | stop | restart while a control runs
  let timer = null

  // logs
  let showLogs = false
  let logsText = ''
  let logsLoading = false

  function friendlyError(msg) {
    if (/unknown or unregistered action|unknown action/i.test(msg)) return $t('ui.docker.not_connected')
    if (/no container named/i.test(msg)) return $t('ui.docker.not_found')
    return $t('ui.widget.unreachable')
  }

  async function load() {
    if (!container) { state = 'error'; errText = 'Konteyner belirtilmedi'; return }
    state = 'loading'
    try {
      statusText = await Do('docker.status', { container })
      // "X çalışıyor (...)" vs "X çalışmıyor (...)" — the two differ past "çalış".
      running = statusText.includes($t('ui.match.docker_up'))
      statsText = ''
      if (running) {
        try { statsText = await Do('docker.stats', { container }) } catch { /* stats optional */ }
      }
      state = 'ok'
    } catch (e) {
      state = 'error'
      errText = friendlyError((e && e.message) ? e.message : String(e))
    }
  }

  async function control(verb) {
    if (busy || !container) return
    busy = verb
    try {
      await Do('docker.' + verb, { container })
      await load()
      if (showLogs) await loadLogs()
    } catch (e) {
      state = 'error'
      errText = friendlyError((e && e.message) ? e.message : String(e))
    } finally {
      busy = ''
    }
  }

  async function toggleLogs() {
    showLogs = !showLogs
    if (showLogs && !logsText) await loadLogs()
  }
  async function loadLogs() {
    logsLoading = true
    try {
      logsText = await Do('docker.logs', { container, lines: '40' })
    } catch (e) {
      logsText = friendlyError((e && e.message) ? e.message : String(e))
    } finally {
      logsLoading = false
    }
  }

  function schedule(min) {
    if (timer) { clearInterval(timer); timer = null }
    if (min > 0) timer = setInterval(load, min * 60 * 1000)
  }
  onDestroy(() => { if (timer) clearInterval(timer) })
  $: schedule(refresh)
  // (re)load whenever the target container changes
  $: container, load()

  // strip the leading "<name>: " the stats/status replies carry, for a tidy card
  $: statsShort = statsText.replace(/^[^:]+:\s*/, '').replace(/\.$/, '')
  $: uptime = (statusText.match(/\(([^)]+)\)/) || [])[1] || ''
</script>

<div class="dw {state}" style="--wa: {accent}" in:fly={{ y: 12, duration: 320 }}>
  <span class="stripe"></span>
  <header>
    <span class="tile">{@html iconDocker}</span>
    <span class="title">{container || title}</span>
    <span class="dot" class:on={running === true} class:off={running === false}
          title={running === true ? $t('ui.docker.up') : running === false ? $t('ui.docker.down') : ''}></span>
    {#if onEdit}
      <button class="icon-btn" on:click={onEdit} title={$t('ui.edit')} aria-label={$t('ui.edit')}>✎</button>
    {/if}
    <button class="icon-btn" on:click={load} title={$t('ui.refresh_short')} aria-label={$t('ui.refresh_short')}>⟳</button>
  </header>

  <div class="body">
    {#if state === 'loading'}
      <span class="skeleton"></span>
    {:else if state === 'error'}
      <span class="err">{errText}</span>
    {:else}
      <div class="statline">
        <span class="badge" class:up={running} class:down={!running}>
          {running ? $t('ui.docker.up') : $t('ui.docker.down')}
        </span>
        {#if running && uptime}<span class="muted">{uptime}</span>{/if}
      </div>
      {#if running && statsShort}
        <div class="stats">{statsShort}</div>
      {/if}

      <div class="controls">
        {#if running}
          <button class="ctl" disabled={!!busy} on:click={() => control('stop')}>
            {busy === 'stop' ? '…' : '■ ' + $t('ui.docker.stop')}
          </button>
          <button class="ctl" disabled={!!busy} on:click={() => control('restart')}>
            {busy === 'restart' ? '…' : '⟲ ' + $t('ui.docker.restart_btn')}
          </button>
        {:else}
          <button class="ctl start" disabled={!!busy} on:click={() => control('start')}>
            {busy === 'start' ? '…' : '▶ ' + $t('ui.docker.start')}
          </button>
        {/if}
        <button class="ctl logs" class:active={showLogs} on:click={toggleLogs}>{$t('ui.docker.logs')}</button>
      </div>

      {#if showLogs}
        <div class="logwrap" transition:slide={{ duration: 200 }}>
          {#if logsLoading}
            <span class="muted">{$t('ui.docker.logs_loading')}</span>
          {:else}
            <pre class="logs">{logsText || $t('ui.docker.no_logs')}</pre>
            <button class="loglink" on:click={loadLogs}>⟳ {$t('ui.refresh_short')}</button>
          {/if}
        </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .dw {
    position: relative;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    padding: 14px 16px;
    box-shadow: var(--shadow-soft);
    overflow: hidden;
    backdrop-filter: blur(6px);
    transition: transform var(--dur) var(--ease), border-color var(--dur) var(--ease);
  }
  .dw:hover { transform: translateY(-2px); border-color: var(--border-2); }
  .stripe { position: absolute; left: 0; top: 0; bottom: 0; width: 3px; background: var(--wa); opacity: .9; }
  .dw.error { border-color: rgba(242, 104, 138, .35); }
  .dw.error .stripe { background: var(--err); }

  header { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
  .tile { width: 30px; height: 30px; flex: 0 0 auto; display: grid; place-items: center; color: var(--wa); }
  .tile :global(svg) { width: 16px; height: 16px; }
  .title { font-weight: 700; color: var(--text-1); font-size: 13px; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .dot { width: 8px; height: 8px; border-radius: 50%; flex: 0 0 auto; background: var(--text-3); }
  .dot.on  { background: var(--ok);  box-shadow: 0 0 0 3px rgba(53, 214, 164, .18); }
  .dot.off { background: var(--err); box-shadow: 0 0 0 3px rgba(242, 104, 138, .16); }

  .icon-btn {
    border: none; background: transparent; color: var(--text-3);
    cursor: pointer; font-size: 13px; line-height: 1;
    transition: color var(--dur);
  }
  .icon-btn:hover { color: var(--text-1); }

  .body { color: var(--text-0); font-size: 13px; min-height: 22px; }

  .statline { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
  .badge { font-size: 11px; font-weight: 800; padding: 2px 8px; border-radius: 6px; }
  .badge.up   { color: var(--ok);  background: rgba(53, 214, 164, .12); }
  .badge.down { color: var(--err); background: rgba(242, 104, 138, .12); }
  .muted { color: var(--text-3); font-size: 12px; }
  .stats { color: var(--text-1); font-weight: 600; font-size: 13px; margin-bottom: 10px; }

  .controls { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
  .ctl {
    border: 1px solid var(--border-2); background: var(--bg-2); color: var(--text-1);
    font-size: 11.5px; font-weight: 700; padding: 6px 10px; border-radius: 8px; cursor: pointer;
    transition: background var(--dur), border-color var(--dur), color var(--dur);
  }
  .ctl:hover:not(:disabled) { border-color: var(--wa); background: var(--surface-2); }
  .ctl:disabled { opacity: .5; cursor: default; }
  .ctl.start:hover:not(:disabled) { border-color: var(--ok); color: var(--ok); }
  .ctl.logs { margin-left: auto; color: var(--text-2); }
  .ctl.logs.active { color: var(--text-0); border-color: var(--wa); }

  .logwrap { margin-top: 10px; }
  .logs {
    margin: 0; max-height: 160px; overflow: auto;
    background: var(--bg-1); border: 1px solid var(--border); border-radius: 8px;
    padding: 8px 10px; font-size: 11px; line-height: 1.5;
    color: var(--text-2); white-space: pre-wrap; word-break: break-all;
    font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  }
  .loglink {
    border: none; background: transparent; color: var(--text-3);
    cursor: pointer; font-size: 11px; margin-top: 4px; padding: 0;
  }
  .loglink:hover { color: var(--text-1); }

  .err { color: var(--err); font-size: 12px; }
  .skeleton {
    display: block; height: 16px; width: 60%; border-radius: 6px;
    background: linear-gradient(90deg, var(--surface-2) 25%, var(--border-2) 37%, var(--surface-2) 63%);
    background-size: 400% 100%; animation: shimmer 1.3s ease-in-out infinite;
  }
  @keyframes shimmer { 0% { background-position: 100% 0; } 100% { background-position: 0 0; } }
</style>
