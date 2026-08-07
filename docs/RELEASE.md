# Releasing Pylon

A release is one thing: a pushed `v*` tag. Everything after that is
`.github/workflows/release.yml` — three platform builds, a leak guard, signed
checksums, and a **draft** GitHub release you publish by hand.

This document exists because the first attempt left the repository in a state
that will bite the second one. Read [Before you tag](#before-you-tag) first.

---

## What the workflow does

| Step | Detail |
| --- | --- |
| Trigger | `push` of a tag matching `v*` |
| Builds | `linux-amd64`, `macos-universal` (lipo of amd64+arm64), `windows-amd64` |
| Version | `-X main.version=$GITHUB_REF_NAME` — the tag *is* the version |
| OAuth | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `SPOTIFY_CLIENT_ID` baked from repo secrets |
| Leak guard | fails the build if any binary contains a `/home/<user>/` or `C:\Users\<user>\` path |
| Archives | `pylon-<tag>-<target>.{tar.gz,zip}`, each holding `pylon`, `pylon-ui`, `pylon.yaml` |
| Signing | `SHA256SUMS` signed with `RELEASE_SIGNING_KEY` → `SHA256SUMS.sig` |
| Release | `gh release create <tag> --generate-notes --draft` |

Unsigned is a supported outcome, not a failure: with no `RELEASE_SIGNING_KEY`
the workflow warns, drops `SHA256SUMS`, and publishes anyway — self-update is
simply disabled for that release, because `internal/selfupdate` refuses to
install anything it cannot verify.

## What self-update needs from a release

`pylon update` reads `GET /repos/YCistak/pylon/releases/latest` and then:

1. Compares tags with `selfupdate.Newer` (semver, prerelease-aware).
2. Downloads `SHA256SUMS` **and** `SHA256SUMS.sig`, verifying the signature
   against the ed25519 public key baked into `internal/selfupdate`.
3. Downloads the archive named by `AssetName(version)` and checks its hash
   against that signed list.
4. Replaces only the `pylon` binary. **The GUI is never self-updated.**

So an asset filename that does not match the tag breaks self-update even though
the release looks fine on the web page.

---

## Before you tag

Two of the four things that used to block a release are resolved. The remaining
two are not code problems; they are decisions.

### ~~1. A stale tag and a broken draft~~ — cleared 2026-08-07

`v0.1.0-alpha.1` used to point weeks behind master, and its draft carried assets
named `pylon-v0.1.0-*` — produced by a different tag push, so self-update could
never have used them. Nothing had been published from it, so the draft and the
tag were deleted and the name reused for the first real release.

Kept here as the recipe, should it happen again:

```sh
gh release delete <tag> --yes --cleanup-tag   # draft, assets and the tag
```

Only safe while nothing has been published from that tag. Once people have
downloaded it, cut the next version instead — moving a published tag breaks
every checkout that already has it.

### ~~2. The repository is private~~ — public since 2026-08-07

Before making a repository public, check the whole history, not the working
tree: a later "scrub personal data" commit does not remove anything from the
commits before it.

```sh
git log --all --format='%ae' | sort -u                 # emails in commit metadata
git grep -I -l -E '<pattern>' $(git rev-list --all)    # content, every commit
```

Worth searching for: your email, absolute home paths, `build.env`,
`*.local.yaml`, provider key prefixes (`AIzaSy`, `ghp_`, `github_pat_`),
`BEGIN.*PRIVATE KEY`. Test fixtures match some of these — read the hits rather
than trusting the count.

### 3. `releases/latest` ignores prereleases

GitHub's "latest" endpoint excludes drafts *and* prereleases. A `-alpha.N` tag
therefore never reaches `pylon update`, however correct the release is. The
version comparison handles prereleases properly now, but the endpoint never
offers it one. First stable tag fixes this by itself; until then, alpha users
update by downloading.

### 4. No OAuth client is baked in

`gh secret list` shows only `RELEASE_SIGNING_KEY`. Without
`GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` / `SPOTIFY_CLIENT_ID` as repository
secrets, the released binaries report Google and Spotify as *unavailable* and
the Settings screen shows "not yet active in this build". Everything else works.

---

## Cutting a release

```sh
# 1. Tests, on the commit you intend to ship.
make test && make vet
(cd pylon-ui && go test -tags webkit2_41 ./...)

# 2. CHANGELOG.md — add the section for this version.

# 3. Tag an existing commit, annotated, and push only that tag.
git tag -a v0.1.0-alpha.2 -m "v0.1.0-alpha.2"
git push origin v0.1.0-alpha.2

# 4. Watch the run.
gh run watch

# 5. Check the assets are named after the tag you pushed.
gh release view v0.1.0-alpha.2 --json assets --jq '.assets[].name'
#   pylon-v0.1.0-alpha.2-linux-amd64.tar.gz
#   pylon-v0.1.0-alpha.2-macos-universal.zip
#   pylon-v0.1.0-alpha.2-windows-amd64.zip
#   SHA256SUMS
#   SHA256SUMS.sig

# 6. Publish (it is created as a draft).
gh release edit v0.1.0-alpha.2 --draft=false --prerelease
```

Then verify the artifact rather than trusting it:

```sh
gh release download v0.1.0-alpha.2 -p '*linux-amd64.tar.gz' -p 'SHA256SUMS*'
sha256sum -c SHA256SUMS --ignore-missing
tar xzf pylon-*-linux-amd64.tar.gz
./pylon-*/pylon version     # must print the tag, not "dev"
```

## Signing keys

`cmd/pylon-sign` owns both halves:

```sh
go run ./cmd/pylon-sign keygen
```

The public half is committed as `publicKey` in
`internal/selfupdate/selfupdate.go`; the private half goes in the
`RELEASE_SIGNING_KEY` repository secret. Rotating the key means every older
binary can no longer verify newer releases — those users must download once by
hand. Only rotate on compromise.

## After the release

`packaging/aur/README.md` covers the AUR package: bump `pkgver`, `updpkgsums`,
regenerate `.SRCINFO`, push. It needs a **published, public** release, so it
waits on the two blockers above.
