# frontend

Pylon's GUI frontend: Svelte 3 + Vite. Wails embeds the built `dist/` into the
Go binary, so this is never served on its own in production.

```sh
npm ci
npm run build     # → dist/, which pylon-ui embeds
```

Building the binary (`make gui` from the repository root) runs this for you.

## Layout

```
src/
  App.svelte          three zones: dock, and home | settings | a pinned page
  lib/
    Sidebar.svelte      the dock — brand, pinned pages, daemon dot, settings
    PylonStage.svelte   the orb
    VoiceBar.svelte     push-to-talk
    Widget.svelte       generic text card, driven by one Do() action
    DockerWidget.svelte rich container card (start/stop/restart)
    DockerPage.svelte   full-screen container manager
    Settings.svelte     three tabs + the widget editor dialog
    Accounts.svelte     OAuth sign-in
    ApiKeys.svelte      vault keys
    VoiceSettings.svelte push-to-talk shortcut
    widgets.js          widget catalog + instance store (localStorage)
    sidebarPages.js     pinnable pages + their store
    daemon.js           one shared "is the daemon up" store
    icons.js            inline SVG
wailsjs/              generated Go bindings — committed, see ../README.md
```

## House rules

- **Turkish in the interface**, English in code and comments.
- **Real elements for real behaviour.** A thing you click is a `<button>`;
  anything with a `role` gets the keyboard handling that role implies. There is
  no `svelte-ignore` in this tree — the build is expected to be warning-free, so
  a warning means something to fix rather than something to silence.
- **State that outlives a session goes to `localStorage` under a versioned key**
  (`pylon.widgets.v2`, `pylon.sidebar.v1`, `pylon.dockerpage.v1`). Bump the
  version and migrate, rather than reading an old shape and hoping.
- **Data comes from the daemon**, always through `Do()` or a bound method. The
  frontend holds no credentials and reaches nothing over the network itself.
