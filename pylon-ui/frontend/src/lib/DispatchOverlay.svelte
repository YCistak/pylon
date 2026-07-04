<script>
  // Dispatch overlay (PLANNED.md Phase 5, LOCKED). Full-frame, click-through,
  // rendered as a sibling of the workspace — never a child of the core avatar,
  // so the core's stacking context can never clip it. A placeholder dot rides
  // the arc until real character art exists; the flight logic below is what
  // the art will reuse unchanged.
  import { onDestroy } from 'svelte'
  import { DOMAINS, dispatchStore, TIMING, easeInCubic, Z, domainById } from './dispatchStore.js'

  let characters = {}
  // id -> { x, y, phase: 'out'|'work'|'back', path: {from, ctrl, to} }
  let flights = {}
  let raf = {}

  const prev = {}
  const unsub = dispatchStore.subscribe((chars) => {
    characters = chars
    for (const d of DOMAINS) {
      const s = chars[d.id].state
      if (prev[d.id] !== s) {
        if (s === 'travel') startFlight(d.id, 'out')
        else if (s === 'return') startFlight(d.id, 'back')
        else if (s === 'settle' || s === 'idle') endFlight(d.id)
        prev[d.id] = s
      }
    }
  })
  onDestroy(() => { unsub(); Object.values(raf).forEach(cancelAnimationFrame) })

  function arcPath(id) {
    const { from, to } = dispatchStore.flightEndpoints(id)
    // Control point lifted above the midpoint → shallow arc over the core.
    const ctrl = { x: (from.x + to.x) / 2, y: Math.min(from.y, to.y) - 120 }
    return { from, ctrl, to }
  }

  const bez = (a, c, b, t) =>
    (1 - t) * (1 - t) * a + 2 * (1 - t) * t * c + t * t * b

  function startFlight(id, phase) {
    // Reuse the outbound path on the way back so the return visibly reverses it.
    const path = phase === 'out' ? arcPath(id) : (flights[id]?.path ?? arcPath(id))
    const start = performance.now()
    cancelAnimationFrame(raf[id])

    const step = (now) => {
      let t = Math.min(1, (now - start) / TIMING.travelMs)
      t = easeInCubic(t) // LOCKED travel easing
      const u = phase === 'out' ? t : 1 - t
      flights = {
        ...flights,
        [id]: {
          phase,
          path,
          x: bez(path.from.x, path.ctrl.x, path.to.x, u),
          y: bez(path.from.y, path.ctrl.y, path.to.y, u),
        },
      }
      if (t < 1) raf[id] = requestAnimationFrame(step)
      else if (phase === 'out') flights = { ...flights, [id]: { ...flights[id], phase: 'work' } }
    }
    raf[id] = requestAnimationFrame(step)
  }

  function endFlight(id) {
    cancelAnimationFrame(raf[id])
    const { [id]: _, ...rest } = flights
    flights = rest
  }

  const chipText = (f) => (f.phase === 'work' ? 'çalışıyor…' : 'yolda')

  $: flightIds = Object.keys(flights)
</script>

<!-- z:30 — arc + traveling placeholder character -->
<div class="overlay" style="z-index: {Z.overlay};" aria-hidden="true">
  <svg class="frame">
    {#each flightIds as id (id)}
      <path
        d="M {flights[id].path.from.x} {flights[id].path.from.y}
           Q {flights[id].path.ctrl.x} {flights[id].path.ctrl.y}
             {flights[id].path.to.x} {flights[id].path.to.y}"
        class="trail"
        style="stroke: {domainById(id).accent};"
      />
      <circle
        cx={flights[id].x}
        cy={flights[id].y}
        r="10"
        class="traveler"
        class:working={flights[id].phase === 'work'}
        style="fill: {domainById(id).accent};"
      />
    {/each}
  </svg>
</div>

<!-- z:40 — in-transit callout chips, pinned to the traveler -->
<div class="overlay" style="z-index: {Z.callout};" aria-hidden="true">
  {#each flightIds as id (id)}
    <span
      class="chip"
      style="left: {flights[id].x}px; top: {flights[id].y - 26}px; --c: {domainById(id).accent};"
    >
      {domainById(id).label} · {chipText(flights[id])}
    </span>
  {/each}
</div>

<!-- z:50 — toast / ETA notifications -->
<div class="overlay toasts" style="z-index: {Z.toast};">
  {#each flightIds as id (id)}
    <div class="toast" style="--c: {domainById(id).accent};">
      <span class="pip"></span>
      {domainById(id).label} görevde
      {#if characters[id]?.state === 'absent'}
        · ~{Math.ceil(characters[id].etaMs / 1000)}s
      {/if}
    </div>
  {/each}
</div>

<style>
  /* LOCKED: absolutely positioned, full-frame, click-through. */
  .overlay {
    position: fixed;
    inset: 0;
    pointer-events: none;
  }

  .frame { width: 100%; height: 100%; overflow: visible; }

  .trail {
    fill: none;
    stroke-width: 1.5;
    stroke-dasharray: 3 9;
    stroke-linecap: round;
    opacity: 0.35;
  }
  .traveler {
    filter: drop-shadow(0 0 8px currentColor);
  }
  .traveler.working { animation: work-pulse 900ms ease-in-out infinite; transform-origin: center; transform-box: fill-box; }
  @keyframes work-pulse {
    0%, 100% { opacity: 1; }
    50%      { opacity: 0.55; }
  }

  .chip {
    position: absolute;
    transform: translateX(-50%);
    font-size: 10px;
    font-weight: 700;
    color: var(--text-0);
    background: var(--bg-2);
    border: 1px solid color-mix(in srgb, var(--c) 45%, transparent);
    border-radius: 999px;
    padding: 2px 9px;
    white-space: nowrap;
    box-shadow: var(--shadow-soft);
  }

  .toasts {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    justify-content: flex-start;
    gap: 8px;
    padding: 18px;
  }
  .toast {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--text-1);
    background: var(--bg-2);
    border: 1px solid var(--border-2);
    border-radius: var(--r-sm);
    padding: 7px 12px;
    box-shadow: var(--shadow);
  }
  .pip {
    width: 7px; height: 7px;
    border-radius: 50%;
    background: var(--c);
    box-shadow: 0 0 8px var(--c);
  }
</style>
