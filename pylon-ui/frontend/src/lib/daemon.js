import { readable } from 'svelte/store'
import { DaemonRunning } from '../../wailsjs/go/main/App.js'

// Single shared daemon-reachability probe. The Pylon GUI auto-starts the daemon
// in the background (app.go / daemon.go), so on a cold launch the socket isn't
// answering for the first second or two. PylonStage, Sidebar and every Widget
// subscribe to THIS store instead of each keeping its own timer — one probe
// drives the whole UI, and widgets reload themselves the moment it flips online
// (instead of getting stuck on a start-up error until manually refreshed).
//
// Value: null = still probing (fresh launch), true = up, false = down.
export const daemonOnline = readable(null, (set) => {
  async function probe() {
    try { set(await DaemonRunning()) } catch { set(false) }
  }
  probe()
  // Poll briskly so the workspace flips to "online" right after the spawned
  // daemon finishes starting; readable stops the interval when nobody listens.
  const timer = setInterval(probe, 1500)
  return () => clearInterval(timer)
})
