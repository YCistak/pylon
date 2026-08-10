<script>
  import { t } from './i18n.js'
  import { onMount, onDestroy } from 'svelte'
  import { slide } from 'svelte/transition'
  import { Do } from '../../wailsjs/go/main/App.js'
  import { iconDocker } from './icons.js'

  const accent = '#2496ed'

  // Page preferences persist so the view/filter/refresh stick between visits.
  const PREF_KEY = 'pylon.dockerpage.v1'
  const REFRESH_OPTIONS = [
    { sec: 0,  label: 'ui.off' },
    { sec: 5,  label: '5sn' },
    { sec: 15, label: '15sn' },
    { sec: 30, label: '30sn' },
    { sec: 60, label: '1dk' },
  ]

  let view = 'list'    // list | grid
  let filter = 'all'   // all | running
  let refreshSec = 15
  let prefsLoaded = false

  function loadPrefs() {
    try {
      const p = JSON.parse(localStorage.getItem(PREF_KEY) || '{}')
      if (p.view === 'grid' || p.view === 'list') view = p.view
      if (p.filter === 'running' || p.filter === 'all') filter = p.filter
      if (typeof p.refreshSec === 'number') refreshSec = p.refreshSec
    } catch { /* defaults */ }
    prefsLoaded = true
  }
  function savePrefs() {
    if (!prefsLoaded) return
    try { localStorage.setItem(PREF_KEY, JSON.stringify({ view, filter, refreshSec })) } catch {}
  }

  let state = 'loading'   // loading | ok | error
  let containers = []
  let errText = ''
  let busy = {}           // { [name]: verb } while a control runs
  let expanded = null     // name of the open row/card
  let detail = {}         // { [name]: { stats, logs, loading } }
  let timer = null

  function friendly(e) {
    const m = (e && e.message) ? e.message : String(e)
    if (/unknown or unregistered action|unknown action/i.test(m)) return $t('ui.docker.not_connected')
    return $t('ui.widget.unreachable')
  }

  async function load() {
    if (!containers.length) state = 'loading'
    try {
      const raw = await Do('docker.list', {})
      containers = JSON.parse(raw || '[]')
      state = 'ok'
    } catch (e) {
      state = 'error'
      errText = friendly(e)
    }
  }

  async function control(name, verb) {
    if (busy[name]) return
    busy = { ...busy, [name]: verb }
    try {
      await Do('docker.' + verb, { container: name })
      await load()
      if (expanded === name) await loadDetail(name)
    } catch (e) {
      errText = friendly(e)
    } finally {
      const b = { ...busy }; delete b[name]; busy = b
    }
  }

  async function toggle(name) {
    if (expanded === name) { expanded = null; return }
    expanded = name
    if (!detail[name]) await loadDetail(name)
  }

  async function loadDetail(name) {
    detail = { ...detail, [name]: { ...(detail[name] || {}), loading: true } }
    const c = containers.find((x) => x.name === name)
    let stats = '', logs = ''
    try { if (c && c.state === 'running') stats = await Do('docker.stats', { container: name }) } catch { /* optional */ }
    try { logs = await Do('docker.logs', { container: name, lines: '40' }) } catch (e) { logs = friendly(e) }
    detail = { ...detail, [name]: { stats: stats.replace(/^[^:]+:\s*/, '').replace(/\.$/, ''), logs, loading: false } }
  }

  const isRunning = (c) => c.state === 'running'
  $: shown = filter === 'running' ? containers.filter(isRunning) : containers
  $: runningCount = containers.filter(isRunning).length

  // Auto-refresh honours the chosen interval (0 = off). Re-armed whenever it changes.
  function schedule(sec) {
    if (timer) { clearInterval(timer); timer = null }
    if (sec > 0) timer = setInterval(load, sec * 1000)
  }
  $: schedule(refreshSec)
  // persist whenever a pref changes (after initial load)
  $: view, filter, refreshSec, savePrefs()

  onMount(() => { loadPrefs(); load() })
  onDestroy(() => { if (timer) clearInterval(timer) })
</script>

<div class="page">
  <header class="head">
    <span class="tile" style="--wa: {accent}">{@html iconDocker}</span>
    <div class="titles">
      <h2>Docker</h2>
      <p class="sub">{$t('ui.docker.running_count', runningCount, containers.length)}</p>
    </div>
  </header>

  <div class="toolbar">
    <div class="seg">
      <button class:active={view === 'list'} on:click={() => (view = 'list')} title="Liste">☰ Liste</button>
      <button class:active={view === 'grid'} on:click={() => (view = 'grid')} title="Kare">▦ Kare</button>
    </div>
    <div class="seg">
      <button class:active={filter === 'all'}     on:click={() => (filter = 'all')}>Hepsi</button>
      <button class:active={filter === 'running'} on:click={() => (filter = 'running')}>{$t('ui.docker.filter_running')}</button>
    </div>

    <div class="spacer"></div>

    <label class="refresh-sel">
      <span>Yenile</span>
      <div class="select-wrap">
        <select bind:value={refreshSec}>
          {#each REFRESH_OPTIONS as o}<option value={o.sec}>{o.label}</option>{/each}
        </select>
        <span class="caret">▾</span>
      </div>
    </label>
    <button class="refresh-btn" on:click={load} title={$t('ui.refresh')} aria-label={$t('ui.refresh')}>⟳</button>
  </div>

  {#if state === 'loading'}
    <div class="msg">{$t('ui.loading')}</div>
  {:else if state === 'error'}
    <div class="msg err">{errText}</div>
  {:else if shown.length === 0}
    <div class="msg">{filter === 'running' ? $t('ui.docker.none_running') : $t('ui.docker.none')}</div>
  {:else}
    <div class="items {view}">
      {#each shown as c (c.name)}
        <div class="item" class:open={expanded === c.name}>
          <div class="item-head">
            <span class="dot" class:on={isRunning(c)} class:off={!isRunning(c)}></span>
            <button class="name" on:click={() => toggle(c.name)}>
              <span class="cname">{c.name}</span>
              <span class="cimage">{c.image}</span>
            </button>
            <span class="cstatus">{c.status}</span>
          </div>

          <div class="controls">
            {#if isRunning(c)}
              <button class="ctl" disabled={!!busy[c.name]} on:click={() => control(c.name, 'stop')}>
                {busy[c.name] === 'stop' ? '…' : '■ Durdur'}
              </button>
              <button class="ctl" disabled={!!busy[c.name]} on:click={() => control(c.name, 'restart')} title={$t('ui.docker.restart')}>
                {busy[c.name] === 'restart' ? '…' : '⟲'}
              </button>
            {:else}
              <button class="ctl start" disabled={!!busy[c.name]} on:click={() => control(c.name, 'start')}>
                {busy[c.name] === 'start' ? '…' : '▶ ' + $t('ui.docker.start')}
              </button>
            {/if}
            <button class="chev" class:up={expanded === c.name} on:click={() => toggle(c.name)} aria-label="detay">›</button>
          </div>

          {#if expanded === c.name}
            <div class="detail" transition:slide={{ duration: 200 }}>
              {#if detail[c.name]?.loading}
                <span class="muted">{$t('ui.loading')}</span>
              {:else}
                {#if detail[c.name]?.stats}
                  <div class="stats">{detail[c.name].stats}</div>
                {/if}
                <div class="logrow">
                  <span class="logtitle">Loglar</span>
                  <button class="loglink" on:click={() => loadDetail(c.name)}>⟳ yenile</button>
                </div>
                <pre class="logs">{detail[c.name]?.logs || 'Log yok.'}</pre>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .page { flex: 1; padding: 32px 40px; overflow: auto; max-width: 980px; }

  .head { display: flex; align-items: center; gap: 14px; margin-bottom: 16px; }
  .tile { width: 40px; height: 40px; flex: 0 0 auto; display: grid; place-items: center; color: var(--wa); }
  .tile :global(svg) { width: 22px; height: 22px; }
  .titles { flex: 1; min-width: 0; }
  .titles h2 { margin: 0; color: var(--text-0); font-size: 22px; }
  .sub { margin: 2px 0 0; color: var(--text-2); font-size: 12px; }

  .toolbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-bottom: 22px; }
  .spacer { flex: 1; }
  .seg { display: flex; gap: 3px; background: var(--bg-1); padding: 3px; border-radius: 10px; border: 1px solid var(--border); height: 34px; box-sizing: border-box; }
  .seg button {
    border: none; background: transparent; color: var(--text-2);
    font-size: 12px; font-weight: 700; padding: 0 12px; border-radius: 7px; cursor: pointer;
    transition: background var(--dur), color var(--dur);
  }
  .seg button.active { background: var(--panel-3); color: var(--text-0); box-shadow: var(--shadow-soft); }

  .refresh-sel { display: flex; align-items: center; gap: 8px; }
  .refresh-sel > span { color: var(--text-3); font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: .05em; }
  .select-wrap { position: relative; display: inline-flex; }
  .select-wrap select {
    appearance: none; -webkit-appearance: none;
    height: 34px; box-sizing: border-box;
    background: var(--bg-1); border: 1px solid var(--border); color: var(--text-1);
    font: inherit; font-size: 12px; font-weight: 700;
    padding: 0 28px 0 11px; border-radius: 9px; cursor: pointer;
    transition: border-color var(--dur);
  }
  .select-wrap select:hover { border-color: var(--border-2); }
  .select-wrap select:focus { outline: none; border-color: var(--accent); }
  .select-wrap .caret { position: absolute; right: 10px; top: 50%; transform: translateY(-50%); color: var(--text-3); font-size: 10px; pointer-events: none; }

  .refresh-btn {
    width: 34px; height: 34px; box-sizing: border-box; flex: 0 0 auto;
    border: 1px solid var(--border); border-radius: 9px;
    background: var(--surface); color: var(--text-2);
    font-size: 16px; cursor: pointer; display: grid; place-items: center;
    transition: background var(--dur), color var(--dur), border-color var(--dur);
  }
  .refresh-btn:hover { color: var(--text-0); background: var(--surface-2); border-color: var(--border-2); }

  .msg { color: var(--text-2); font-size: 14px; padding: 30px 4px; text-align: center; }
  .msg.err { color: var(--err); }

  /* item container: list = stacked rows, grid = responsive tiles */
  .items.list { display: flex; flex-direction: column; gap: 8px; }
  .items.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 12px; align-items: start; }

  .item {
    background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-md);
    transition: border-color var(--dur);
  }
  .item:hover, .item.open { border-color: var(--border-2); }

  .dot { width: 9px; height: 9px; border-radius: 50%; flex: 0 0 auto; background: var(--text-3); }
  .dot.on  { background: var(--ok);  box-shadow: 0 0 0 3px rgba(53, 214, 164, .16); }
  .dot.off { background: var(--err); box-shadow: 0 0 0 3px rgba(242, 104, 138, .14); }

  .name {
    border: none; background: transparent; cursor: pointer; text-align: left;
    display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1;
  }
  .cname { color: var(--text-0); font-weight: 700; font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .cimage { color: var(--text-3); font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .cstatus { color: var(--text-2); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .controls { display: flex; align-items: center; gap: 6px; }
  .ctl {
    border: 1px solid var(--border-2); background: var(--bg-2); color: var(--text-1);
    font-size: 11.5px; font-weight: 700; padding: 6px 10px; border-radius: 8px; cursor: pointer;
    transition: background var(--dur), border-color var(--dur), color var(--dur);
  }
  .ctl:hover:not(:disabled) { border-color: var(--accent); background: var(--surface-2); }
  .ctl:disabled { opacity: .5; cursor: default; }
  .ctl.start:hover:not(:disabled) { border-color: var(--ok); color: var(--ok); }
  .chev {
    border: none; background: transparent; color: var(--text-3); cursor: pointer;
    font-size: 18px; line-height: 1; padding: 2px 4px; transition: transform var(--dur), color var(--dur);
  }
  .chev:hover { color: var(--text-1); }
  .chev.up { transform: rotate(90deg); color: var(--text-1); }

  /* LIST layout: head + controls on one row; detail wraps full-width below. */
  .list .item { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding: 12px 14px; }
  .list .item-head { display: flex; align-items: center; gap: 12px; flex: 1; min-width: 0; }
  .list .cstatus { flex: 0 1 auto; max-width: 220px; }
  .list .controls { flex: 0 0 auto; }
  .list .detail { flex-basis: 100%; }

  /* GRID layout: card — head, status, controls stacked. */
  .grid .item { display: flex; flex-direction: column; gap: 10px; padding: 14px; }
  .grid .item-head { display: flex; align-items: flex-start; gap: 10px; }
  .grid .cstatus { flex-basis: 100%; order: 3; }
  .grid .item-head { flex-wrap: wrap; }
  .grid .controls { margin-top: 2px; }

  .detail { padding-top: 4px; }
  .list .detail { padding: 0 2px 4px 33px; }
  .stats { color: var(--text-1); font-weight: 600; font-size: 13px; margin-bottom: 10px; }
  .logrow { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
  .logtitle { font-size: 11px; font-weight: 800; color: var(--text-3); text-transform: uppercase; letter-spacing: .05em; }
  .loglink { border: none; background: transparent; color: var(--text-3); cursor: pointer; font-size: 11px; }
  .loglink:hover { color: var(--text-1); }
  .logs {
    margin: 0; max-height: 200px; overflow: auto;
    background: var(--bg-1); border: 1px solid var(--border); border-radius: 8px;
    padding: 8px 10px; font-size: 11px; line-height: 1.5; color: var(--text-2);
    white-space: pre-wrap; word-break: break-all;
    font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  }
  .muted { color: var(--text-3); font-size: 12px; }
</style>
