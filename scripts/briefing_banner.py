#!/usr/bin/env python3
"""Show Pylon's daily briefing as a desktop banner on Wayland.

A short, styled rectangle anchored to the top-centre of the screen — not a
transient notification and not part of the app window, but a real overlay drawn
by the compositor via the wlr layer-shell protocol (Hyprland, Sway, ...). It
reads the briefing text from argv[1] or, if absent, from stdin, so the daemon
can pipe it in.

The banner dismisses itself after a few seconds (DURATION below, overridable
with PYLON_BANNER_SECONDS), or immediately on a click, the × button, or Escape.
It never grabs keyboard focus, so it won't interrupt whatever you are doing.

Requires: python-gobject, gtk3, gtk-layer-shell. Invoked from config as
    briefing.banner_cmd: ["python3", "scripts/briefing_banner.py"]
"""

import os
import sys

import gi

gi.require_version("Gtk", "3.0")
gi.require_version("Gdk", "3.0")
gi.require_version("GtkLayerShell", "0.1")
from gi.repository import Gdk, GLib, Gtk, GtkLayerShell  # noqa: E402

DEFAULT_DURATION = 5


def duration() -> int:
    """Seconds the banner stays before dismissing itself; 0 keeps it forever.

    PYLON_BANNER_SECONDS overrides the default. Garbage in the environment
    falls back rather than killing the banner — a briefing you cannot read is
    worse than one that lingers a little.
    """
    try:
        return max(0, int(os.environ.get("PYLON_BANNER_SECONDS", "")))
    except ValueError:
        return DEFAULT_DURATION

CSS = b"""
.banner {
  background-color: rgba(9, 9, 11, 0.98);
  border: 1px solid rgba(255, 255, 255, 0.10);
  border-radius: 16px;
  padding: 14px 18px;
  box-shadow: 0 14px 44px rgba(0, 0, 0, 0.6);
}
.msg {
  color: #eef1ff;
  font-size: 15px;
  font-weight: 500;
}
.dot {
  background-color: #7c8cf8;
  border-radius: 8px;
  box-shadow: 0 0 10px rgba(124, 140, 248, 0.9);
}
.close {
  color: rgba(238, 241, 255, 0.55);
  font-size: 20px;
  padding: 0 4px;
  min-height: 0;
  min-width: 0;
  background: none;
  border: none;
  box-shadow: none;
}
.close:hover { color: #eef1ff; }
"""


def read_text() -> str:
    """Briefing text from argv[1], else stdin. Empty means nothing to show."""
    if len(sys.argv) > 1 and sys.argv[1].strip():
        return sys.argv[1].strip()
    if not sys.stdin.isatty():
        return sys.stdin.read().strip()
    return ""


def main() -> int:
    text = read_text()
    if not text:
        return 0

    win = Gtk.Window()
    win.set_app_paintable(True)  # let the rounded corners show through

    # Draw with an RGBA visual so the corners are actually transparent.
    screen = win.get_screen()
    visual = screen.get_rgba_visual()
    if visual is not None:
        win.set_visual(visual)

    # Anchor to the top of the output, centred horizontally (only the top edge
    # is anchored). Sit on the overlay layer, above normal windows, reserving no
    # space and never taking keyboard focus.
    GtkLayerShell.init_for_window(win)
    GtkLayerShell.set_namespace(win, "pylon-briefing")
    GtkLayerShell.set_layer(win, GtkLayerShell.Layer.OVERLAY)
    GtkLayerShell.set_anchor(win, GtkLayerShell.Edge.TOP, True)
    GtkLayerShell.set_margin(win, GtkLayerShell.Edge.TOP, 18)
    GtkLayerShell.set_keyboard_mode(win, GtkLayerShell.KeyboardMode.NONE)

    row = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=12)
    row.get_style_context().add_class("banner")
    # A layer-shell surface anchored only at the top takes its child's natural
    # width, and a wrapping label's natural width is its narrowest — which would
    # collapse the banner into a tall thin strip. Pin a comfortable width so it
    # reads as a proper rectangle and the text wraps within it.
    row.set_size_request(560, -1)

    dot = Gtk.Box()
    dot.get_style_context().add_class("dot")
    dot.set_size_request(9, 9)
    dot.set_valign(Gtk.Align.CENTER)

    label = Gtk.Label(label=text)
    label.get_style_context().add_class("msg")
    label.set_line_wrap(True)
    label.set_xalign(0.0)
    label.set_max_width_chars(72)

    close = Gtk.Button(label="×")
    close.get_style_context().add_class("close")
    close.set_relief(Gtk.ReliefStyle.NONE)
    close.set_valign(Gtk.Align.CENTER)

    row.pack_start(dot, False, False, 0)
    row.pack_start(label, True, True, 0)
    row.pack_start(close, False, False, 0)

    # An event box over the whole row so a click anywhere dismisses.
    click = Gtk.EventBox()
    click.add(row)
    win.add(click)

    provider = Gtk.CssProvider()
    provider.load_from_data(CSS)
    Gtk.StyleContext.add_provider_for_screen(
        screen, provider, Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION
    )

    done = {"v": False}

    def dismiss(*_):
        if done["v"]:
            return False
        done["v"] = True
        Gtk.main_quit()
        return False

    close.connect("clicked", dismiss)
    click.connect("button-press-event", dismiss)
    win.connect("destroy", dismiss)
    win.connect(
        "key-press-event",
        lambda _w, e: dismiss() if e.keyval == Gdk.KEY_Escape else None,
    )

    secs = duration()
    if secs > 0:
        GLib.timeout_add_seconds(secs, dismiss)

    win.show_all()
    Gtk.main()
    return 0


if __name__ == "__main__":
    sys.exit(main())
