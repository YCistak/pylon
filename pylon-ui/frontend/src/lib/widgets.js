import { writable } from 'svelte/store'

// Registry of widgets the user CAN enable. Home shows none of these by default;
// they are turned on from Settings. `accent` colors the card per service.
export const AVAILABLE = [
  { id: 'calendar', icon: '📅', title: 'Takvim',   action: 'calendar.list_today',   accent: '#7c8cf8' },
  { id: 'freshrss', icon: '📰', title: 'FreshRSS', action: 'freshrss.unread_count', accent: '#f4b860' },
  { id: 'github',   icon: '🐙', title: 'GitHub',   action: 'github.list_prs',       accent: '#34e0d8' },
]

export function widgetById(id) {
  return AVAILABLE.find((w) => w.id === id)
}

// Persisted layout: which widgets are enabled and in which column.
// Shape: { [id]: 'left' | 'right' }. Empty = Home starts blank (per user).
const KEY = 'pylon.widgets.v1'

function load() {
  try {
    const raw = localStorage.getItem(KEY)
    return raw ? JSON.parse(raw) : {}
  } catch {
    return {}
  }
}

function createLayout() {
  const { subscribe, update, set } = writable(load())

  function persist(value) {
    try { localStorage.setItem(KEY, JSON.stringify(value)) } catch {}
    return value
  }

  return {
    subscribe,
    // enable a widget into a column (default left), or move it
    enable(id, column = 'left') {
      update((v) => persist({ ...v, [id]: column }))
    },
    disable(id) {
      update((v) => { const n = { ...v }; delete n[id]; return persist(n) })
    },
    setColumn(id, column) {
      update((v) => (v[id] ? persist({ ...v, [id]: column }) : v))
    },
    reset() { set(persist({})) },
  }
}

export const layout = createLayout()
