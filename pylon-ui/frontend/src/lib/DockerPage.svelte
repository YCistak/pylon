<script>
  import { onMount, onDestroy } from 'svelte'
  import { slide, fade } from 'svelte/transition'
  import { Do } from '../../wailsjs/go/main/App.js'
  import { iconDocker } from './icons.js'

  const accent = '#2496ed'

  let state = 'loading'   // loading | ok | error
  let containers = []
  let errText = ''
  let filter = 'all'      // all | running
  let busy = {}           // { [name]: verb } while a control runs
  let expanded = null     // name of the open row
  let detail = {}         // { [name]: { stats, logs, loading } }
  let timer = null

  function friendly(e) {
    const m = (e && e.message) ? e.message : String(e)
    if (/servissiz aksiyon|bilinmeyen aksiyon/i.test(m)) return 'Docker bağlı değil'
    return 'Şu an ulaşılamıyor'
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

  onMount(() => { load(); timer = setInterval(load, 15000) })
  onDestroy(() => { if (timer) clearInterval(timer) })
</script>

<div class="page">
  <header class="head">
    <span class="tile" style="--wa: {accent}">{@html iconDocker}</span>
    <div class="titles">
      <h2>Docker</h2>
      <p class="sub">{runningCount} / {containers.length} konteyner çalışıyor</p>
    </div>
    <div class="seg">
      <button class:active={filter === 'all'}      on:click={() => (filter = 'all')}>Hepsi</button>
      <button class:active={filter === 'running'}  on:click={() => (filter = 'running')}>Çalışanlar</button>
    </div>
    <button class="refresh" on:click={load} title="yenile" aria-label="yenile">⟳</button>
  </header>

  {#if state === 'loading'}
    <div class="msg">Yükleniyor…</div>
  {:else if state === 'error'}
    <div class="msg err">{errText}</div>
  {:else if shown.length === 0}
    <div class="msg">{filter === 'running' ? 'Çalışan konteyner yok.' : 'Konteyner yok.'}</div>
  {:else}
    <ul class="list">
      {#each shown as c (c.name)}
        <li class:open={expanded === c.name}>
          <div class="row">
            <span class="dot" class:on={isRunning(c)} class:off={!isRunning(c)}></span>
            <button class="name" on:click={() => toggle(c.name)}>
              <span class="cname">{c.name}</span>
              <span class="cimage">{c.image}</span>
            </button>
            <span class="cstatus">{c.status}</span>
            <div class="controls">
              {#if isRunning(c)}
                <button class="ctl" disabled={!!busy[c.name]} on:click={() => control(c.name, 'stop')}>
                  {busy[c.name] === 'stop' ? '…' : '■ Durdur'}
                </button>
                <button class="ctl" disabled={!!busy[c.name]} on:click={() => control(c.name, 'restart')}>
                  {busy[c.name] === 'restart' ? '…' : '⟲'}
                </button>
              {:else}
                <button class="ctl start" disabled={!!busy[c.name]} on:click={() => control(c.name, 'start')}>
                  {busy[c.name] === 'start' ? '…' : '▶ Başlat'}
                </button>
              {/if}
              <button class="chev" class:up={expanded === c.name} on:click={() => toggle(c.name)} aria-label="detay">›</button>
            </div>
          </div>

          {#if expanded === c.name}
            <div class="detail" transition:slide={{ duration: 200 }}>
              {#if detail[c.name]?.loading}
                <span class="muted">Yükleniyor…</span>
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
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .page { flex: 1; padding: 32px 40px; overflow: auto; max-width: 860px; }

  .head { display: flex; align-items: center; gap: 14px; margin-bottom: 22px; }
  .tile { width: 40px; height: 40px; flex: 0 0 auto; display: grid; place-items: center; color: var(--wa); }
  .tile :global(svg) { width: 22px; height: 22px; }
  .titles { flex: 1; min-width: 0; }
  .titles h2 { margin: 0; color: var(--text-0); font-size: 22px; }
  .sub { margin: 2px 0 0; color: var(--text-2); font-size: 12px; }

  .seg { display: flex; gap: 3px; background: var(--bg-1); padding: 3px; border-radius: 10px; border: 1px solid var(--border); }
  .seg button {
    border: none; background: transparent; color: var(--text-2);
    font-size: 12px; font-weight: 700; padding: 6px 12px; border-radius: 7px; cursor: pointer;
    transition: background var(--dur), color var(--dur);
  }
  .seg button.active { background: var(--panel-3); color: var(--text-0); box-shadow: var(--shadow-soft); }

  .refresh { border: none; background: transparent; color: var(--text-3); cursor: pointer; font-size: 18px; transition: color var(--dur); }
  .refresh:hover { color: var(--text-0); }

  .msg { color: var(--text-2); font-size: 14px; padding: 30px 4px; text-align: center; }
  .msg.err { color: var(--err); }

  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  li {
    background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-md);
    overflow: hidden; transition: border-color var(--dur);
  }
  li:hover { border-color: var(--border-2); }
  li.open { border-color: var(--border-2); }

  .row { display: flex; align-items: center; gap: 12px; padding: 12px 14px; }
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: 0 0 auto; background: var(--text-3); }
  .dot.on  { background: var(--ok);  box-shadow: 0 0 0 3px rgba(53, 214, 164, .16); }
  .dot.off { background: var(--err); box-shadow: 0 0 0 3px rgba(242, 104, 138, .14); }

  .name {
    border: none; background: transparent; cursor: pointer; text-align: left;
    display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1;
  }
  .cname { color: var(--text-0); font-weight: 700; font-size: 14px; }
  .cimage { color: var(--text-3); font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .cstatus { color: var(--text-2); font-size: 12px; flex: 0 1 auto; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 220px; }

  .controls { display: flex; align-items: center; gap: 6px; flex: 0 0 auto; }
  .ctl {
    border: 1px solid var(--border-2); background: var(--bg-2); color: var(--text-1);
    font-size: 11.5px; font-weight: 700; padding: 6px 10px; border-radius: 8px; cursor: pointer;
    transition: background var(--dur), border-color var(--dur), color var(--dur);
  }
  .ctl:hover:not(:disabled) { border-color: var(--wa, var(--accent)); background: var(--surface-2); }
  .ctl:disabled { opacity: .5; cursor: default; }
  .ctl.start:hover:not(:disabled) { border-color: var(--ok); color: var(--ok); }
  .chev {
    border: none; background: transparent; color: var(--text-3); cursor: pointer;
    font-size: 18px; line-height: 1; padding: 2px 4px; transition: transform var(--dur), color var(--dur);
  }
  .chev:hover { color: var(--text-1); }
  .chev.up { transform: rotate(90deg); color: var(--text-1); }

  .detail { padding: 0 14px 14px 33px; }
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
