# Release

A release is cut on macOS, from `main`, only after the release gate passes.
The gate is not optional: signing and notarization run after it, so a failing
end-to-end check stops the release before any artifact exists.

## Version

`mygo.json` `"version"` is the single source of truth. The tag must equal
`v<version>`; the release workflow refuses a mismatch. `scripts/bump.sh` bumps
the version, moves the `## Unreleased` changelog entries under the new version,
checks that the tree is clean and on `main`, commits, and pushes the tag.
`UPSTREAMS.yml` carries the upstream pins for that release, not the product
version.

## Checklist

1. `scripts/gate.sh release` passes locally (PR gate + full online e2e:
   toolchain install, JDT language services, debugger).
2. `scripts/bump.sh <version>` on a clean `main`.
3. The tag push runs `.github/workflows/release.yml` on `macos-14`:
   imports the Developer ID certificate, creates the notary keychain profile
   `goidea-notary`, runs `scripts/release.sh`.
4. `scripts/release.sh`:
   - fetches `resources/openidea-server` at the pin if missing;
   - runs `scripts/gate.sh release`;
   - injects the signing identity and notarization profile into a temporary
     `mygo.json` (the committed file stays ad-hoc friendly) and restores it on
     exit, failing if the restore failed;
   - copies license texts (`LICENSE`, `NOTICE`, `THIRD_PARTY_LICENSES.md`, and
     each submodule's license) into `resources/licenses`;
   - runs `go tool mygo build -platform darwin/arm64` (or `darwin/universal`);
   - verifies the app with `codesign --verify --deep --strict`, `spctl` and
     `xcrun stapler validate`, and the DMG with `stapler` and `spctl`;
   - writes `goidea-<version>.dmg.sha256`.
5. The workflow publishes the DMG and its `.sha256` with generated release
   notes.

## Verifying a published artifact

Download the asset, check the digest against the published `.sha256`, mount the
DMG, and run:

```sh
shasum -a 256 goidea-<version>.dmg
codesign --verify --deep --strict --verbose=2 /Volumes/GoIDEA/GoIDEA.app
spctl --assess --type execute --verbose=2 /Volumes/GoIDEA/GoIDEA.app
xcrun stapler validate /Volumes/GoIDEA/GoIDEA.app
```

Gatekeeper must accept the downloaded copy, not only the local build. The
release notes record the server version, the upstream commits, and the gate
result.

## Platforms

Today the shipped artifact is macOS arm64 (`macos-13` minimum). The build
matrix for darwin amd64/arm64, Windows amd64/arm64 and Linux amd64/arm64 is
tracked in `PERFORMANCE.md` and `UPSTREAM_FEATURE_MATRIX.md`; a platform is only
announced when its `openidea-server` asset exists, verifies against the
release's SHA-256, and the app passes the offline e2e on that platform. Until
then, no other platform is claimed.

## Rollback

- A bad build: delete the GitHub release and tag, fix, and bump again; never
  re-upload a different binary under an existing tag.
- A bad server pin: restore the previous `server` block in `UPSTREAMS.yml`,
  re-run `scripts/fetch-server.sh`, and re-run the release gate.
- The notarization profile is a local keychain item; if it is missing,
  `GOIDEA_NOTARY_PROFILE=<profile>` or
  `GOIDEA_APPLE_ID` + `GOIDEA_APPLE_PASSWORD` + `GOIDEA_TEAM_ID` must be
  supplied. Notary uploads go direct and occasionally flake; retry the job
  rather than changing the build.
