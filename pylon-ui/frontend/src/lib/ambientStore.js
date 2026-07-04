import { writable } from 'svelte/store'

// ── Ambient signals (PLANNED.md Phase 5, LOCKED) ─────────────────────────────
// Ambient signals are environment-wide state broadcast to EVERY currently
// mounted (visible) character, regardless of domain — as opposed to scoped
// signals, which belong to exactly one character's own services.
//
// RULE: ALL mounted characters must react to this SINGLE global store. Never
// give a character its own per-character copy of an ambient value — one store
// update fans out to whichever characters are mounted; mounting/unmounting
// naturally handles "react only if visible."
//
// Not wired to real Spotify data yet (Phase 2 integration pending); flip it
// with toggleMusic() (dev panel / window.pylonDev) for visual testing.
export const musicPlaying = writable(false)

export function toggleMusic() {
  musicPlaying.update((v) => !v)
}
