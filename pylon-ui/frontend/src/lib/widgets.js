import { writable } from 'svelte/store'
import {
  iconGoogleCalendar,
  iconGoogleDrive,
  iconGitHub,
  iconSpotify,
  iconFreshRSS,
  iconDocker,
  iconWeather,
  iconSysmon,
} from './icons.js'

// Registry of widget TYPES the user can add. Titles and labels are catalog
// KEYS, not text: this module is data, and the language belongs to the screen
// that renders it (see lib/i18n.js).
// Registry of widget TYPES the user can add. Each type offers one or more
// `modes` (an action + optional param fields). Instances (in the store below)
// are configured copies of a type — multiple instances of the same type are
// allowed (e.g. two GitHub widgets, one PRs one Issues).
export const CATALOG = [
  {
    type: 'calendar', icon: iconGoogleCalendar, title: 'ui.widget.calendar', accent: '#7c8cf8',
    modes: [
      { id: 'list_today', label: 'ui.widget.calendar.today', action: 'calendar.list_today', params: [] },
    ],
  },
  {
    type: 'freshrss', icon: iconFreshRSS, title: 'FreshRSS', accent: '#f4b860',
    modes: [
      { id: 'unread_count', label: 'ui.widget.freshrss.unread', action: 'freshrss.unread_count', params: [] },
    ],
  },
  {
    type: 'github', icon: iconGitHub, title: 'GitHub', accent: '#34e0d8',
    modes: [
      { id: 'list_prs', label: 'ui.widget.github.prs', action: 'github.list_prs', params: [] },
      { id: 'list_issues', label: 'ui.widget.github.issues', action: 'github.list_issues', params: [] },
    ],
  },
  {
    type: 'drive', icon: iconGoogleDrive, title: 'Drive', accent: '#34a853',
    modes: [
      { id: 'recent', label: 'ui.widget.drive.recent', action: 'drive.recent', params: [] },
      {
        id: 'find', label: 'ui.widget.drive.find', action: 'drive.find',
        params: [{ key: 'query', label: 'ui.widget.drive.query', placeholder: 'ui.widget.drive.query_ph' }],
      },
    ],
  },
  {
    type: 'weather', icon: iconWeather, title: 'ui.widget.weather', accent: '#38bdf8',
    modes: [
      { id: 'today', label: 'ui.widget.weather.today', action: 'weather.today', params: [] },
    ],
  },
  {
    type: 'spotify', icon: iconSpotify, title: 'Spotify', accent: '#1db954',
    modes: [
      { id: 'now_playing', label: 'ui.widget.spotify.now', action: 'spotify.now_playing', params: [] },
    ],
  },
  {
    type: 'sysmon', icon: iconSysmon, title: 'ui.widget.sysmon', accent: '#8b5cf6',
    modes: [
      { id: 'stats', label: 'ui.widget.sysmon.stats', action: 'sysmon.stats', params: [] },
    ],
  },
  {
    type: 'docker', icon: iconDocker, title: 'Docker', accent: '#2496ed',
    modes: [
      { id: 'ps', label: 'ui.widget.docker.ps', action: 'docker.ps', params: [] },
      {
        // Rich, interactive card (status dot + CPU/RAM + start/stop/restart + logs).
        // Rendered by DockerWidget.svelte instead of the generic card.
        id: 'container', label: 'ui.widget.docker.one', action: 'docker.status',
        params: [{ key: 'container', label: 'ui.widget.docker.container', placeholder: 'ui.widget.docker.container_ph' }],
      },
    ],
  },
]

export function catalogEntry(type) {
  return CATALOG.find((c) => c.type === type)
}

export function modeOf(type, modeId) {
  const entry = catalogEntry(type)
  return entry && entry.modes.find((m) => m.id === modeId)
}

// Auto-refresh choices, minutes (0 = off).
export const REFRESH_OPTIONS = [
  { value: 0, label: 'ui.off' },
  { value: 1, label: 'ui.minutes.1' },
  { value: 5, label: 'ui.minutes.5' },
  { value: 15, label: 'ui.minutes.15' },
  { value: 30, label: 'ui.minutes.30' },
]

const KEY = 'pylon.widgets.v2'
const OLD_KEY = 'pylon.widgets.v1'

function uid() {
  return Math.random().toString(36).slice(2, 10)
}

function defaultInstance(type, column = 'left') {
  const entry = catalogEntry(type)
  const mode = entry.modes[0]
  return {
    id: uid(),
    type,
    title: entry.title,
    column,
    mode: mode.id,
    params: {},
    refresh: 0,
    accent: entry.accent,
  }
}

// One-time migration: old v1 was { [catalogId]: 'left' | 'right' }. Convert
// each enabled widget into a default instance in the same column, then drop
// the old key so migration only runs once.
function migrateFromV1() {
  try {
    const raw = localStorage.getItem(OLD_KEY)
    if (!raw) return null
    const v1 = JSON.parse(raw)
    const instances = Object.entries(v1)
      .filter(([type]) => catalogEntry(type))
      .map(([type, column]) => defaultInstance(type, column))
    localStorage.removeItem(OLD_KEY)
    return instances
  } catch {
    return null
  }
}

function load() {
  try {
    const raw = localStorage.getItem(KEY)
    // Drop instances whose type is no longer in the catalog. A saved layout
    // outlives the catalog, so a type that disappears would otherwise reach
    // withAction() with no entry and take the whole home render down.
    if (raw) return JSON.parse(raw).filter((w) => catalogEntry(w.type))
  } catch {
    // fall through to migration/empty
  }
  const migrated = migrateFromV1()
  return migrated || []
}

function createWidgets() {
  const { subscribe, update, set } = writable(load())

  function persist(value) {
    try { localStorage.setItem(KEY, JSON.stringify(value)) } catch {}
    return value
  }

  return {
    subscribe,
    add(type, config) {
      const instance = { ...defaultInstance(type, config.column || 'left'), ...config }
      update((list) => persist([...list, instance]))
      return instance
    },
    update(id, patch) {
      update((list) => persist(list.map((w) => (w.id === id ? { ...w, ...patch } : w))))
    },
    remove(id) {
      update((list) => persist(list.filter((w) => w.id !== id)))
    },
    reset() { set(persist([])) },
  }
}

export const widgets = createWidgets()
