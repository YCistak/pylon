#!/bin/sh
# Install Pylon for the current user — no root, no package manager.
#
# Driven by `make install-user`, which builds the two binaries first. Everything
# lands under $HOME so it can be undone by deleting the files listed at the end;
# the AUR package (packaging/aur/) is the system-wide equivalent.
#
# Two rules this script will not break:
#
#   1. An existing pylon.yaml is never touched. It is the one file here that
#      holds your decisions, and on a development machine it is often a symlink
#      to a checkout — overwriting it would throw away work and leave no trace.
#   2. An existing desktop entry is backed up before being replaced, because
#      yours may point somewhere deliberate (a dev build, a wrapper).
#
# Run with --dry-run to see the exact file operations without performing any.

set -eu

REPO="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

PREFIX="${PREFIX:-$HOME/.local}"
BINDIR="${BINDIR:-$PREFIX/bin}"
DESKTOPDIR="${DESKTOPDIR:-$PREFIX/share/applications}"
ICONDIR="${ICONDIR:-$PREFIX/share/icons/hicolor/512x512/apps}"
CONFIGDIR="${CONFIGDIR:-${XDG_CONFIG_HOME:-$HOME/.config}/pylon}"

DRY=0
for arg in "$@"; do
	case "$arg" in
	--dry-run | -n) DRY=1 ;;
	*)
		echo "kullanım: install-user.sh [--dry-run]" >&2
		exit 2
		;;
	esac
done

say() { printf '%s\n' "$*"; }
run() {
	if [ "$DRY" -eq 1 ]; then
		printf '  [dry-run] %s\n' "$*"
	else
		"$@"
	fi
}

[ "$DRY" -eq 1 ] && say "— DRY RUN: hiçbir dosya değişmeyecek —"

# --- what must exist before we start -----------------------------------------

missing=""
[ -x "$REPO/pylon" ] || missing="$missing\n  $REPO/pylon (make build)"
[ -x "$REPO/pylon-ui/build/bin/pylon-ui" ] || missing="$missing\n  $REPO/pylon-ui/build/bin/pylon-ui (make gui)"
if [ -n "$missing" ]; then
	# shellcheck disable=SC2059
	printf "kurulacak ikili yok:$missing\n" >&2
	exit 1
fi

say "Pylon kuruluyor → $PREFIX"

# --- binaries ----------------------------------------------------------------

run mkdir -p "$BINDIR"
run install -m 0755 "$REPO/pylon" "$BINDIR/pylon"
run install -m 0755 "$REPO/pylon-ui/build/bin/pylon-ui" "$BINDIR/pylon-ui"
say "  ✔ $BINDIR/pylon"
say "  ✔ $BINDIR/pylon-ui"

# --- icon --------------------------------------------------------------------

icon="$REPO/pylon-ui/build/appicon.png"
if [ -f "$icon" ]; then
	run mkdir -p "$ICONDIR"
	run install -m 0644 "$icon" "$ICONDIR/pylon.png"
	say "  ✔ $ICONDIR/pylon.png"
else
	say "  · ikon bulunamadı ($icon) — atlandı"
fi

# --- desktop entry -----------------------------------------------------------

entry="$DESKTOPDIR/pylon.desktop"
run mkdir -p "$DESKTOPDIR"
if [ -e "$entry" ] && ! cmp -s "$REPO/packaging/user/pylon.desktop" "$entry"; then
	# Yours may point at a dev build on purpose. Keep a copy either way.
	run cp -p "$entry" "$entry.bak"
	say "  ! mevcut girdi farklıydı → $entry.bak olarak yedeklendi"
fi
run install -m 0644 "$REPO/packaging/user/pylon.desktop" "$entry"
say "  ✔ $entry"

if command -v update-desktop-database >/dev/null 2>&1; then
	run update-desktop-database "$DESKTOPDIR"
fi

# --- config ------------------------------------------------------------------

# -e misses a broken symlink, which is still something we must not clobber.
if [ -e "$CONFIGDIR/pylon.yaml" ] || [ -L "$CONFIGDIR/pylon.yaml" ]; then
	say "  · $CONFIGDIR/pylon.yaml zaten var — dokunulmadı"
else
	run mkdir -p "$CONFIGDIR"
	run install -m 0644 "$REPO/pylon.yaml" "$CONFIGDIR/pylon.yaml"
	say "  ✔ $CONFIGDIR/pylon.yaml (örnek config)"
fi

# --- afterwards --------------------------------------------------------------

case ":$PATH:" in
*":$BINDIR:"*) ;;
*)
	say ""
	say "  ! $BINDIR PATH'te değil. Kabuk yapılandırmana ekle:"
	say "      fish:  fish_add_path $BINDIR"
	say "      bash:  export PATH=\"$BINDIR:\$PATH\""
	say "    Desktop girdisi \`pylon-ui\`'yi PATH'te arar, onsuz menüden açılmaz."
	;;
esac

say ""
say "Bitti. Menüden \"Pylon\" ile aç, ya da: pylon status"
say "Kaldırmak için bu dosyaları sil:"
say "  $BINDIR/pylon  $BINDIR/pylon-ui"
say "  $entry  $ICONDIR/pylon.png"
say "(config ve veritabanı $CONFIGDIR altında kalır — kasıtlı.)"
