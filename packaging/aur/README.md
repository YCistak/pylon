# AUR packaging

`pylon-bin` installs the prebuilt release binaries. Source lives here in the
main repo; the AUR is a separate git repo you push a copy of the PKGBUILD to.

## Prerequisites (both are hard blockers)

1. **The repository must be public.** The AUR clones the source URLs; a private
   repo returns 404 for the release tarball and the raw icon.
2. **The release must be published, not a draft.** `source=` points at
   `releases/download/vX.Y.Z/...`, which only resolves for a published release.

Until both are true this package cannot build for anyone, including you.

## Publishing a new version

From this directory, against a published release:

```sh
# 1. Bump pkgver to the new tag (drop the leading v), reset pkgrel=1.
# 2. Refresh the tarball + icon hashes from the live release:
updpkgsums

# 3. Build and install it locally in a clean chroot to confirm it works:
makepkg -si

# 4. Regenerate .SRCINFO (the AUR reads metadata from it, not the PKGBUILD):
makepkg --printsrcinfo > .SRCINFO

# 5. Push to the AUR (first time: clone ssh://aur@aur.archlinux.org/pylon-bin.git):
cp PKGBUILD pylon.desktop .SRCINFO /path/to/aur/pylon-bin/
cd /path/to/aur/pylon-bin && git commit -am "pylon-bin X.Y.Z" && git push
```

`.SRCINFO` is intentionally not committed here: it is generated per-version and
only belongs in the AUR repo, where it would drift from this PKGBUILD if kept
in two places.

## Why -bin and not a source build

The release binaries are already produced by CI, trimmed of build paths, and
signed. Rebuilding the Wails GUI from source in a chroot means dragging in
node, wails, and a webkit toolchain for no gain over binaries that are known
good. If a from-source `pylon` package is ever wanted, it belongs beside this
one, not instead of it.

## Self-update interaction

The daemon refuses to update itself once installed under `/usr/bin`
(`internal/selfupdate.Packaged`), so `pacman -Syu` is the only update path and
the two can never disagree about what is installed.
