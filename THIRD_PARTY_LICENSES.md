# Third-party licenses

GoIDEA's own code is MIT (see `LICENSE`). This file lists every third-party
component GoIDEA builds from or ships, with the version, the upstream commit
when it is not a tagged release, the license, and what GoIDEA changed.

The license texts are in `third_party/<component>/`, and the release copies them
into the app (`Contents/Resources/licenses`).

## Shipped as code

### openidea-server — Apache-2.0

- Project: OpenIDEA, https://github.com/l0ng-ai/openidea
- Version: 0.1.41, release `v0.1.41` (commit `108da53`)
- Component: `openidea-server-macos-aarch64.gz`, the official prebuilt release
  asset for Apple silicon, verified against the release's SHA-256
- Modification: none; the binary is redistributed unmodified
- Attribution: `third_party/openidea/LICENSE`, `third_party/openidea/NOTICE`

GoIDEA starts this server as a child process (`openidea-server --stdio`) and
speaks its JSON-RPC protocol. The Java project model, JDT LS integration,
Maven/Gradle, run/debug, VCS, terminal, database and agent services come from
it. GoIDEA does not reimplement them.

### mygo (sorrycc fork) — MIT

- Project: MyGo, https://github.com/egoist/mygo
- Fork: https://github.com/sorrycc/mygo
- Version: `v0.4.9-0.20261106101517-f970e7091911` (commit `f970e709`), the
  version Mayoi itself pins
- Component: the native UI framework and editor (`ui`, `ui/editor`, `mygo`)
- Modification: none; used through the module replacement in `go.mod`
- Attribution: `third_party/mygo/LICENSE`

### github.com/ebitengine/purego v0.11.1 — Apache-2.0

- Indirect dependency of mygo; calls into the macOS frameworks
- Modification: none
- Attribution: `third_party/purego/LICENSE`

### github.com/go-text/typesetting v0.3.5 — BSD-3-Clause or Unlicense

- Text shaping and font handling, through mygo
- Modification: none
- Attribution: `third_party/go-text/LICENSE`

### golang.org/x/image v0.46.0 — BSD-3-Clause

- Image decoding and scaling, through mygo
- Modification: none
- Attribution: `third_party/x-image/LICENSE`

### golang.org/x/net v0.59.0 — BSD-3-Clause

- Networking primitives used by mygo
- Modification: none
- Attribution: `third_party/x-net/LICENSE`, `third_party/x-net/PATENTS`

### golang.org/x/text v0.43.0 — BSD-3-Clause

- Unicode text handling
- Modification: none
- Attribution: `third_party/x-text/LICENSE`

## Upstreams GoIDEA follows but does not copy code from

### Mayoi v0.19.3 — MIT

- Project: https://github.com/sorrycc/Mayoi
- Fork: https://github.com/misswell/Mayoi (kept in sync, not modified by GoIDEA)
- GoIDEA follows Mayoi's native-app patterns (menus, sheets, editor wiring,
  mygo tooling conventions). No Mayoi source file is copied into this
  repository; the app code is written here.
- If a future change does copy Mayoi code, the file must carry Mayoi's MIT
  notice and this table gains a row for it.

## Icons, fonts and other assets

- `resources/icon.png` is GoIDEA's own icon, generated for this project.
- mygo's built-in UI draws with the system fonts; it ships no font files of its
  own in this build.

## Keeping this list true

`scripts/check-licenses.sh` fails when a module in the build graph is missing
from this file or when its license text is not one of MIT, Apache-2.0,
BSD-2-Clause, BSD-3-Clause or ISC. Run it before every release; the release
workflow runs it too.
