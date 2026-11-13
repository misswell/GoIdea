# AGENTS.md

GoIDEA is a fork of Mayoi with a pinned MyGo module and the OpenIDEA backend as
a submodule. Read `ARCHITECTURE.md` and `UPSTREAM_SYNC.md` before changing
anything shared with an upstream.

## Commits

- One imperative subject line of 72 characters or fewer, for one change: `Add mayoi run and mayoi commands`.
- Put any detail in a short body, not the subject.
- Commit a batch only after its gate passed; do not commit half-verified work.

## Build and test

- Build with `CGO_ENABLED=0 go build -o build/goidea ./cmd/goidea`. A plain
  `go build` can fail on the macOS 27 SDK with a `tapi`/arm64e linker error;
  CGO off is the supported build here.
- `scripts/gate.sh pr` runs format, vet, unit tests, build, licenses and the
  regression index. Run it before every commit.
- `scripts/gate.sh main` adds the offline end-to-end run against the real
  `openidea-server` and Maven. `scripts/gate.sh release` adds the toolchain,
  JDT and debugger phases. Do not weaken a gate to make a change pass.
- Tests must use real Maven/Gradle/JDK/OpenIDEA for build, language and debug
  acceptance. Mocks are not accepted for core Java behavior.
- Do not run `scripts/e2e.sh` online casually: it downloads the JDT LS on first
  use. Reuse `GOIDEA_E2E_DATA`.

## Runtime rules

- Never open a visible window in the owner's session. Verify UI with
  `GOIDEA_HEADLESS=1` and the `GOIDEA_SMOKE_*` flags, or in a hidden probe;
  prove probes are gone by PID afterwards.
- stdout of `openidea-server` is RPC only; logs go to stderr. GoIDEA must not
  print protocol noise to stdout.
- Use an isolated data directory (`GOIDEA_DATA_DIR`) and
  `OPENIDEA_SERVER_HOME`. Never touch the user's OpenIDEA installation, its
  `jdtls` processes, or the two upstream checkouts.
- No orphan processes and no unbounded queues. Killing the app must stop its
  server; streams are bounded.
- Never overwrite unsaved text silently. Conflicts go to the merge prompt or a
  banner.
- There is no WebView, Electron, Tauri, React, HTML or CSS in GoIDEA's UI.

## Design

- Follow docs/DESIGN.md for motion, layout stability, states, speed, failures and words.
- Take animation lengths from the tokens in cmd/mayoi/motion.go, never a number written out.

## Upstreams and licenses

- `UPSTREAMS.yml` is the only pin file; `UPSTREAM_FEATURE_MATRIX.md` records
  what GoIDEA actually exposes. A feature is `INTEGRATED` only when GoIDEA's UI
  works and a smoke or test proves it.
- Keep `local_changes: none-to-upstream`. Backend changes go through the
  OpenIDEA fork and a submodule bump.
- Every new or changed end-to-end check must be added to
  `REGRESSION_INDEX.json`; `scripts/check-regression-index.py` enforces it.
- Keep `LICENSE`, `NOTICE`, `THIRD_PARTY_LICENSES.md` and each upstream's
  license text; `scripts/check-licenses.sh` must pass.

## Changelog

- Add a bullet for every user-facing change under `## Unreleased` in CHANGELOG.md, creating that section above the latest version if it is missing.
- One sentence per bullet, about 100 characters at most, saying what the user sees.
- Use **bold** for menu paths and `code` for commands, keys and settings. Leave out internals.
- Split a big feature into several bullets rather than writing one long one.
- Skip changes users can't see: tests, CI, refactors.
