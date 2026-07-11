import { writable } from 'svelte/store'
import { iconDocker } from './icons.js'

// Page TYPES the user can pin to the sidebar. Each opens a full-page view
// (unlike home widgets, which are a glance). Docker is the first — a full
// container manager that scales past what home widgets can show.
export const PAGE_CATALOG = [
  { type: 'docker', icon: iconDocker, title: 'Docker', accent: '#2496ed' },
]

export function pageCatalogEntry(type) {
  return PAGE_CATALOG.find((p) => p.type === type)
}

// Persisted, ordered list of pinned page instances: { id, type }.
const KEY = 'pylon.sidebar.v1'

function uid() {
  return Math.random().toString(36).slice(2, 10)
}

function load() {
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) return JSON.parse(raw)
  } catch {
    // fall through to empty
  }
  return []
}

function createPages() {
  const { subscribe, update, set } = writable(load())

  function persist(value) {
    try { localStorage.setItem(KEY, JSON.stringify(value)) } catch {}
    return value
  }

  return {
    subscribe,
    add(type) {
      const entry = pageCatalogEntry(type)
      if (!entry) return null
      const page = { id: uid(), type }
      update((list) => persist([...list, page]))
      return page
    },
    remove(id) {
      update((list) => persist(list.filter((p) => p.id !== id)))
    },
    reset() { set(persist([])) },
  }
}

export const sidebarPages = createPages()
