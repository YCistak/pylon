# Per-user install

The files `scripts/install-user.sh` puts in place. Nothing here needs root, and
nothing here is the system-wide package — that is `packaging/aur/`.

| File | Installed to |
| --- | --- |
| `pylon.desktop` | `~/.local/share/applications/` |
| `pylon.service` | `~/.config/systemd/user/` |

```sh
make install-user                    # copy the binaries
make install-user ARGS=--link        # symlink them out of the checkout
make install-user ARGS=--dry-run     # show the file operations, do none
```

`--link` is for a machine you also develop on: `make build` then updates what
the menu entry and the service launch, with no reinstall step in between.

## Autostart

The unit is installed but never enabled, and that is deliberate.

`WantedBy=graphical-session.target` only fires if something reaches that target.
A full desktop environment does; a bare Wayland compositor session frequently
does not, and there `systemctl --user enable pylon` looks like it worked while
starting nothing.

Start it from the compositor instead, which also guarantees the session
variables the daemon needs (`WAYLAND_DISPLAY`, and on Hyprland
`HYPRLAND_INSTANCE_SIGNATURE`) are already exported by the time it runs:

```
# Hyprland (hyprland.conf)
exec-once = systemctl --user start pylon

# Hyprland (Lua config)
hl.exec_cmd("systemctl --user start pylon")

# sway
exec systemctl --user start pylon
```

If your session does reach `graphical-session.target`, `systemctl --user enable
pylon` works as usual and you can skip the compositor line.

## The environment gap

A user unit does **not** inherit your interactive shell's environment. An API
key exported from `.bashrc`, `.profile` or as a fish universal variable is
visible to a terminal and to the compositor's children, but not to the daemon
started by systemd — it will log `no API key` and quietly fall back.

Put keys in the vault rather than in the unit file:

```sh
printf '%s' "$YOUR_KEY" | pylon secret set gemini
```

The intent chain reads `secret:gemini` when the environment variable is empty,
and the vault is encrypted; `Environment=` lines in a unit are world-readable
plain text.
