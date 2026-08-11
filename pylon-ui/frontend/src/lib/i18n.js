// Interface text, in the language the daemon is speaking.
//
// The GUI carries its own catalogs rather than reading the daemon's: it is a
// separate Go module and cannot import internal/i18n, and the two vocabularies
// barely overlap anyway — the daemon speaks sentences ("3 events in your
// calendar"), this speaks labels ("Appearance", "Cancel").
//
// What it must not have is its own language *setting*. The language belongs to
// the daemon: Settings changes it there (App.SetLanguage) and reads it back, so
// the buttons around an answer are always in the same language as the answer,
// and the CLI agrees with both.

import { derived, writable } from 'svelte/store'
import { Language, Languages, SetLanguage } from '../../wailsjs/go/main/App.js'

import de from './locales/de.json'
import en from './locales/en.json'
import es from './locales/es.json'
import fr from './locales/fr.json'
import pt from './locales/pt.json'
import ru from './locales/ru.json'
import tr from './locales/tr.json'

const catalogs = { de, en, es, fr, pt, ru, tr }
const FALLBACK = 'en'

export const lang = writable(FALLBACK)

/** Ask the daemon which language it speaks. Unknown or unreachable keeps English. */
export async function syncLanguage() {
  try {
    const l = await Language()
    if (l && catalogs[l]) lang.set(l)
  } catch {
    // No daemon yet — the sidebar's status dot already says so, and English is
    // a working default until one appears.
  }
}

/**
 * The languages Settings can offer: [{ code, name }], each name written in its
 * own language and script. The list comes from the daemon, filtered to the ones
 * this GUI also has labels for — a daemon that speaks a language the interface
 * cannot label would produce a half-translated window.
 *
 * An unreachable daemon yields [], which the picker shows as "not running"
 * rather than as an empty list.
 */
export async function availableLanguages() {
  try {
    const raw = await Languages()
    return raw
      .split('\n')
      .map((line) => line.split('\t'))
      .filter(([code]) => code && catalogs[code])
      .map(([code, name]) => ({ code, name: name || code }))
  } catch {
    return []
  }
}

/**
 * Switch the language Pylon speaks, and follow it here. An empty code means
 * "follow the system": the daemon forgets the choice and falls back to
 * pylon.yaml or the desktop locale, and returns whichever it landed on.
 *
 * Throws when the daemon refuses or is unreachable, so the caller can say so
 * instead of leaving the interface claiming a language it is not in.
 */
export async function setLanguage(code) {
  const applied = await SetLanguage(code || 'auto')
  if (applied && catalogs[applied]) lang.set(applied)
  return applied
}

/**
 * t(key, ...args) — the message for the active language.
 *
 * A missing key returns the key itself, which is ugly on screen and therefore
 * gets noticed; falling back to an empty label would just look like a bug in
 * the layout. Arguments replace {0}, {1}, … in order.
 */
export const t = derived(lang, ($lang) => (key, ...args) => {
  const msg = catalogs[$lang]?.[key] ?? catalogs[FALLBACK][key]
  if (msg === undefined) return key
  return args.length ? msg.replace(/\{(\d+)\}/g, (m, i) => args[i] ?? m) : msg
})
