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

Four things are true right now and each one blocks part of the release. None of
them are code problems; they are decisions.

### 1. `v0.1.0-alpha.1` already exists, pointing at the wrong commit

```sh
git tag -l                      # v0.1.0-alpha.1
git rev-parse v0.1.0-alpha.1    # f25786c — weeks behind master
gh release list                 # v0.1.0-alpha.1  Draft
```

That draft was produced by a `v0.1.0` tag push and later re-pointed, so its
assets are named `pylon-v0.1.0-*` while the tag says `v0.1.0-alpha.1`. Per the
list above, self-update cannot use it.

Moving a tag that has already been pushed is a rewrite: anyone who fetched it
keeps the old one until they prune. That is acceptable here only because
nothing has been published from it. To reuse the name:

```sh
gh release delete v0.1.0-alpha.1 --yes    # the draft and its assets
git push origin :refs/tags/v0.1.0-alpha.1 # the remote tag
git tag -d v0.1.0-alpha.1                 # the local tag
```

Otherwise pick the next unused version and skip this step entirely — cheaper,
and it leaves no history to explain.

### 2. The repository is private

`api.github.com/.../releases/latest` returns 404 to an anonymous client, and so
does every `browser_download_url`. That means:

- `pylon update` cannot work for anyone, including you on another machine.
- The AUR package cannot build at all (see `packaging/aur/README.md`, which
  names this as a hard blocker).

A private release is still useful — you can download it yourself — but do not
announce self-update or the AUR package until the repository is public.

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
