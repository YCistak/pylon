import { writable, derived, get } from 'svelte/store'

// ── Phase 5 dispatch state machine (PLANNED.md, LOCKED) ─────────────────────
// States per character: idle → (preview) → exit → travel → absent → return →
// settle → idle. All timing values come from the locked spec; the state logic
// is placeholder-shape-agnostic so real character art swaps in with zero
// changes here.

// Timings (LOCKED — PLANNED.md Phase 5)
export const TIMING = {
  exitMs: 120,     // lift 4px + tilt 15° toward exit edge, ease-out
  travelMs: 500,   // arc across workspace, ease-in-cubic
  workMs: 2000,    // placeholder "service call" duration (real latency later)
  settleMs: 260,   // seat-flash on return
}

// ease-in-cubic (LOCKED travel easing)
export const easeInCubic = (t) => t * t * t

// z-index stack (LOCKED). Workspace bg is z:0.
export const Z = {
  core: 10,      // Pylon core avatar (dims to 0.32 during dispatch)
  dock: 20,      // sidebar dock
  overlay: 30,   // dispatch overlay (arc + traveling character)
  callout: 40,   // in-transit status chip
  toast: 50,     // toast / ETA notification
}

// Character-to-service grouping (LOCKED — max 3, do not extend)
export const DOMAINS = [
  { id: 'organizer',    label: 'Organizer',      services: 'takvim · drive',   accent: 'var(--accent)' },
  { id: 'devops',       label: 'DevOps',         services: 'github · freshrss', accent: 'var(--accent-2)' },
  { id: 'system_media', label: 'System / Media', services: 'spotify · sistem',  accent: 'var(--warn)' },
]

const initial = {}
for (const d of DOMAINS) initial[d.id] = { state: 'idle', etaMs: 0 }

const characters = writable(initial)

// Concurrent dispatch rule (LOCKED): core dimming is a BOOLEAN GATE, not
// additive. activeDispatchCount increments on dispatch start and decrements
// when the return settles; core opacity is 0.32 whenever count > 0 and 1.0
// only at 0 — never multiply 0.32 per concurrent dispatch.
export const activeDispatchCount = writable(0)
export const coreDimmed = derived(activeDispatchCount, (n) => n > 0)

const wait = (ms) => new Promise((r) => setTimeout(r, ms))

function setChar(id, patch) {
  characters.update((c) => ({ ...c, [id]: { ...c[id], ...patch } }))
}

// Preview (LOCKED): between absent/idle and exit — character stays seated,
// plays a lean-in micro-animation. Clearing input drops back without dispatch.
function preview(id, on = true) {
  const cur = get(characters)[id]
  if (!cur) return
  if (on && cur.state === 'idle') setChar(id, { state: 'preview' })
  else if (!on && cur.state === 'preview') setChar(id, { state: 'idle' })
}

// Fires ONLY for synchronous user-initiated actions (LOCKED). Background
// jobs must never call this — they update scoped idle status text only.
async function dispatch(id) {
  const cur = get(characters)[id]
  if (!cur || (cur.state !== 'idle' && cur.state !== 'preview')) return

  activeDispatchCount.update((n) => n + 1)

  setChar(id, { state: 'exit' })
  await wait(TIMING.exitMs)

  setChar(id, { state: 'travel' })
  await wait(TIMING.travelMs)

  // Absent: socket keeps its slot (dashed outline + hollow ring + ETA),
  // never collapses. ETA counts down the placeholder work duration.
  setChar(id, { state: 'absent', etaMs: TIMING.workMs })
  const tick = setInterval(() => {
    const c = get(characters)[id]
    if (c.state !== 'absent') return
    setChar(id, { etaMs: Math.max(0, c.etaMs - 200) })
  }, 200)
  await wait(TIMING.workMs)
  clearInterval(tick)

  setChar(id, { state: 'return', etaMs: 0 })
  await wait(TIMING.travelMs)

  setChar(id, { state: 'settle' }) // brief seat-flash
  await wait(TIMING.settleMs)

  setChar(id, { state: 'idle' })
  activeDispatchCount.update((n) => n - 1)
}

// ── Flight anchors ───────────────────────────────────────────────────────────
// Dock sockets and the core stage register their DOM nodes so the overlay can
// compute arc endpoints in viewport coordinates at flight start.
const anchors = { sockets: {}, stage: null }

function registerSocket(id, el) {
  anchors.sockets[id] = el
  return { destroy() { if (anchors.sockets[id] === el) delete anchors.sockets[id] } }
}

function registerStage(el) {
  anchors.stage = el
  return { destroy() { if (anchors.stage === el) anchors.stage = null } }
}

function centerOf(el) {
  const r = el.getBoundingClientRect()
  return { x: r.left + r.width / 2, y: r.top + r.height / 2 }
}

// Endpoints for a domain's flight: socket → stage center (viewport coords).
function flightEndpoints(id) {
  const s = anchors.sockets[id]
  const from = s ? centerOf(s) : { x: 90, y: window.innerHeight / 2 }
  const to = anchors.stage
    ? centerOf(anchors.stage)
    : { x: window.innerWidth / 2, y: window.innerHeight / 2 }
  return { from, to }
}

export const dispatchStore = {
  subscribe: characters.subscribe,
  dispatch,
  preview,
  registerSocket,
  registerStage,
  flightEndpoints,
}

export function domainById(id) {
  return DOMAINS.find((d) => d.id === id)
}
