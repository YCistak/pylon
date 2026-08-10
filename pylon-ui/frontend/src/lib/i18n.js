// Interface text, in the language the daemon is speaking.
//
// The GUI carries its own catalogs rather than reading the daemon's: it is a
// separate Go module and cannot import internal/i18n, and the two vocabularies
// barely overlap anyway — the daemon speaks sentences ("3 events in your
// calendar"), this speaks labels ("Appearance", "Cancel").
//
// What it must not have is its own language *setting*. The language comes from
// the daemon (App.Language), so the buttons around an answer are always in the
// same language as the answer.

import { derived, writable } from 'svelte/store'
import { Language } from '../../wailsjs/go/main/App.js'

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
