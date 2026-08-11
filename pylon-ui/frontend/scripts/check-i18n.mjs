// Guards the interface catalogs. Run with `npm run check:i18n`; the GUI CI job
// runs it before the build.
//
// It exists because two whole classes of translation bug are invisible to every
// other check the project has — the code compiles, the tests pass, and the
// window is simply wrong:
//
//   1. A key missing from one language falls back to English silently, so a
//      German window ends up half in English and nobody notices until a German
//      speaker opens it.
//   2. A catalog key stored in a data structure and then rendered without $t()
//      prints the key itself: the screen says "ui.keys.gemini_hint".
//
// Both shipped at least once. This turns them into a failed build.

import { readFileSync, readdirSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const lib = join(dirname(fileURLToPath(import.meta.url)), '..', 'src', 'lib')
const locales = join(lib, 'locales')
const problems = []

// ---- 1. Every language carries exactly the keys English does ----------------

const catalogs = {}
for (const file of readdirSync(locales).filter((f) => f.endsWith('.json'))) {
  catalogs[file.replace('.json', '')] = JSON.parse(readFileSync(join(locales, file), 'utf8'))
}
const en = catalogs.en
if (!en) problems.push('locales/en.json is missing — it is the fallback for every other language')

for (const [lang, cat] of Object.entries(catalogs)) {
  if (lang === 'en') continue
  for (const key of Object.keys(en)) {
    if (!(key in cat)) problems.push(`${lang}.json: missing key ${key}`)
  }
  for (const key of Object.keys(cat)) {
    if (!(key in en)) problems.push(`${lang}.json: key not in en.json: ${key}`)
  }
}

// Placeholders have to survive translation: "{0}" dropped from one language
// means an empty gap where a number belongs.
for (const [lang, cat] of Object.entries(catalogs)) {
  if (lang === 'en') continue
  for (const [key, value] of Object.entries(cat)) {
    if (!(key in en)) continue
    const want = [...en[key].matchAll(/\{(\d+)\}/g)].map((m) => m[1]).sort().join(',')
    const got = [...String(value).matchAll(/\{(\d+)\}/g)].map((m) => m[1]).sort().join(',')
    if (want !== got) {
      problems.push(`${lang}.json: ${key} has placeholders {${got}}, English has {${want}}`)
    }
  }
}

// ---- 2. Sources -------------------------------------------------------------

const sources = []
;(function walk(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (entry.name !== 'locales') walk(join(dir, entry.name))
    } else if (/\.(svelte|js)$/.test(entry.name) && entry.name !== 'i18n.js') {
      sources.push([join(dir, entry.name), readFileSync(join(dir, entry.name), 'utf8')])
    }
  }
})(lib)

// Every ui.* string the code names has to exist, or $t renders the key on screen.
for (const [path, text] of sources) {
  for (const m of text.matchAll(/['"`](ui\.[a-z0-9_.]+)['"`]/g)) {
    if (!(m[1] in en)) problems.push(`${path}: no such key: ${m[1]}`)
  }
}

// A field that holds a catalog key anywhere must be translated everywhere it is
// rendered. Collecting the field names from the data (`hint: 'ui.x'`) rather
// than hard-coding them keeps this working as new ones appear.
const keyFields = new Set()
for (const [, text] of sources) {
  for (const m of text.matchAll(/\b([a-zA-Z_$][\w$]*)\s*:\s*['"`]ui\.[a-z0-9_.]+['"`]/g)) {
    keyFields.add(m[1])
  }
}

if (keyFields.size > 0) {
  const fields = [...keyFields].join('|')
  // {thing.hint} and attr={thing.hint} — a rendered value, not a $t() call, not
  // a bind:, and not an assignment.
  const rendered = new RegExp(`[={]\\s*([a-zA-Z_$][\\w$]*)\\.(${fields})\\s*}`, 'g')
  for (const [path, text] of sources) {
    for (const m of text.matchAll(rendered)) {
      const before = text.slice(Math.max(0, m.index - 40), m.index)
      if (/\$t\($/.test(before) || /bind:/.test(before)) continue
      const line = text.slice(0, m.index).split('\n').length
      // The same field name can hold a key in one place and free text in
      // another — a widget the user renamed, a brand name. Those say so on the
      // line with `i18n-raw`, which is the point: the exception is visible to
      // the next reader instead of being a rule this script quietly bends.
      // The marker is looked for on the line and the fifteen above it, because
      // it often cannot go on the line itself — an HTML comment is not valid
      // inside a tag's attribute list, so it goes above the element it explains.
      const lines = text.split('\n')
      if (/i18n-raw/.test(lines.slice(Math.max(0, line - 16), line).join('\n'))) continue
      problems.push(
        `${path}:${line}: ${m[1]}.${m[2]} is rendered raw — ` +
          `${m[2]} holds catalog keys elsewhere, so this prints the key. Wrap it in $t().`,
      )
    }
  }
}

// ---- 3. No user-facing text left hard-coded in the markup -------------------
//
// The check above only sees keys that reached a data structure. Text written
// straight into the markup — {listening ? 'Dinliyorum…' : 'Konuş'}, a
// placeholder attribute — is invisible to it and stays in one language forever.
// That is how the voice bar shipped Turkish inside a Russian window.
//
// Only the markup is scanned; the <script> block is full of legitimate string
// literals (event names, action ids, storage keys).

// Product names are the same in every language, so they are text and not a bug.
const BRANDS = new Set([
  'Pylon', 'Docker', 'Google', 'Spotify', 'GitHub', 'FreshRSS', 'Gemini', 'Wails',
])

// Literals that are clearly code rather than prose: an all-lowercase word with
// no space is an enum value ('running', 'docker', 'grid'), not a label.
function looksLikeProse(s) {
  const text = s.trim()
  if (text.length < 2) return false
  if (BRANDS.has(text)) return false
  if (/^ui\./.test(text)) return false
  if (!/\p{L}{2,}/u.test(text)) return false // no word in it: an icon, an arrow
  return /\s/.test(text) || /^\p{Lu}/u.test(text) || /[^\x00-\x7F]/.test(text)
}

for (const [path, text] of sources) {
  if (!path.endsWith('.svelte')) continue
  // Between </script> and <style>: the <style> block is CSS, where "SF Mono"
  // is a font and not a label. Comments go too — they are prose by nature and
  // every one of them would be a false report.
  const start = text.lastIndexOf('</script>')
  const end = text.lastIndexOf('<style>')
  const offset = start === -1 ? 0 : start
  let markup = text.slice(offset, end > offset ? end : text.length)
  // Blank comments out rather than deleting them, so line numbers still line up.
  markup = markup.replace(/<!--[\s\S]*?-->/g, (c) => c.replace(/[^\n]/g, ' '))

  // Quoted strings inside {…} expressions and attribute values, minus the
  // attributes that never reach the screen.
  for (const m of markup.matchAll(/(\w[\w:-]*=)?['"]([^'"\n{}<>=]{2,})['"]/g)) {
    const attr = (m[1] || '').replace('=', '')
    if (['class', 'style', 'type', 'id', 'role', 'href', 'src', 'lang', 'width', 'height'].includes(attr)) continue
    if (!looksLikeProse(m[2])) continue
    // Inline handlers put DOM key names in the markup ("Enter", "Escape").
    // They are capitalised like prose and are the one thing that reliably
    // trips the rule above.
    if (/\.key\s*[=!]==?\s*$/.test(markup.slice(Math.max(0, m.index - 20), m.index))) continue
    const line = text.slice(0, offset + m.index).split('\n').length
    if (/i18n-raw/.test(text.split('\n').slice(Math.max(0, line - 16), line).join('\n'))) continue
    problems.push(`${path}:${line}: hard-coded text "${m[2]}" — move it to the catalogs and use $t().`)
  }
}

// ---- Report -----------------------------------------------------------------

if (problems.length > 0) {
  console.error(`i18n check failed (${problems.length}):\n`)
  for (const p of problems) console.error('  ' + p)
  process.exit(1)
}
console.log(`i18n ok — ${Object.keys(catalogs).length} languages, ${Object.keys(en).length} keys`)
