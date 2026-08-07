# pylon-ui

Pylon's desktop GUI: [Wails](https://wails.io) v2 with a Svelte 3 frontend.

It is a **separate Go module** (`module pylon-ui`), which is the whole point of
the arrangement: Wails needs CGo and webkit, and keeping that out of the daemon's
module is what lets `go build ./...` at the repository root stay CGo-free and
cross-compile to Linux, macOS and Windows.

The price is that this module imports nothing from the daemon. It talks to it
over the same Unix socket the CLI uses and carries its own copy of the wire
protocol (`request` / `response` in `app.go`) — about thirty lines, kept in step
by hand. `PYLON_SOCKET` overrides the path on both sides.

What the GUI actually does, screen by screen: [`../docs/UI.md`](../docs/UI.md).

## Build

From the repository root:

```sh
make gui          # frontend + binary → pylon-ui/build/bin/pylon-ui
make install-user # that, plus the daemon, an icon and a desktop entry, into ~/.local
```

`make gui` passes `-tags desktop,production,webkit2_41`. All three matter:
without Wails' own `desktop,production` tags the binary compiles and then
refuses to start —

```
Error: Wails applications will not build without the correct build tags.
```

— and `webkit2_41` selects WebKitGTK 4.1 over Wails' 4.0 default. CI only
type-checks this module (`go build -tags webkit2_41`), so nothing catches a
binary built without the runtime tags until someone tries to open it.

With the Wails CLI installed you can use it directly instead:

```sh
wails dev   -tags webkit2_41   # hot-reload dev window
wails build -tags webkit2_41
```

Linux needs `libwebkit2gtk-4.1-dev` and `libgtk-3-dev`. On Arch that is
`webkit2gtk-4.1` and `gtk3`.

## Wails bindings

`frontend/wailsjs/` is generated from the exported methods on `App`, and is
**committed**. The Wails CLI is not required to build this module, so when you
add or rename a bound method and do not have the CLI to hand, edit
`frontend/wailsjs/go/main/App.js` and `App.d.ts` to match — keeping the entries
alphabetical, the way the generator emits them. Getting that wrong is silent:
the frontend calls a `window.go` name that does not exist.

## Tests

```sh
go test -tags webkit2_41 ./...
```

`app_test.go` stands a fake daemon up on a temporary socket, which is the only
way to exercise the IPC helpers without a real one. Keep the socket path short —
macOS caps it near 104 bytes, and it is the *directory* that spends the budget.
