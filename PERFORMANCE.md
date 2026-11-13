# Performance

This document records the performance budgets and the mechanisms that protect
them. Numbers measured on the release machine are added by
`scripts/bench.sh` (see "Measuring" below); anything not measured is marked as
such rather than estimated.

## Targets

| Path | Target |
| --- | --- |
| Cold start to first frame (warm caches) | ≤ 1.5 s |
| Startup to backend ready (`initialize`) | ≤ 2 s |
| Open a 5 000-file Maven project | tree visible ≤ 1 s; project model ready ≤ 10 s |
| Keystroke-to-buffer latency | one frame (≤ 16 ms), never blocks on RPC |
| Buffer-to-backend sync | ≤ 400 ms after the last keystroke |
| Open a 1 000 000-line file | ≤ 2 s to first frame of text |
| Idle CPU (window open, no input) | ≤ 1 % of one core |
| Resident memory with one 10 000-line file | ≤ 400 MB total for app + server |
| 100 open tabs | no orphan process; tab switch ≤ 50 ms |
| 20 concurrent tasks (search + build + terminal) | no unbounded queue growth |

## Mechanisms

- **RPC off the UI thread.** Every backend call is goroutine-backed; results
  return via `mygo.RunOnMain`. The view never waits on the server.
- **Bounded queues.** Connection notifications 8192, manager status 16,
  `Listen` copies 1024 with drop-on-lag; overflow closes the connection instead
  of growing (`internal/openidea/conn.go:70-140`, `manager.go:53-119`).
- **Debounced sync.** Buffer edits are pushed 220 ms after the last change, on
  a 120 ms ticker that runs even when frames are not drawn
  (`internal/app/app.go:283-294,838-872`).
- **Line index.** UTF-16 ↔ byte mapping uses an implicit treap with
  incremental `ApplyEdit`, so edits cost O(log lines), not O(file)
  (`internal/position`).
- **Bounded logs.** Run output keeps 3000 lines; tool install logs keep 400
  lines. Neither grows with process lifetime.
- **Large files.** Files above 4 MiB are not auto-reloaded by the watch
  (`internal/app/watch.go`).
- **Lazy tree.** The workspace tree reads directories on demand and follows the
  disk; it does not walk the whole project at startup (`internal/workspace`).
- **One document per path.** Tabs share buffers, so memory is proportional to
  open files, not open tabs.
- **Server restart cap.** 5 reconnect attempts with quadratic backoff; no
  restart storm.

## Measuring

`scripts/bench.sh` starts GoIDEA offscreen with `GOIDEA_HEADLESS=1` in a fixed
data directory, generates its scenarios in a temporary directory (a 10k-line
text file, a 1M-line file, a 10k-file source tree, a 100-tab session), samples
CPU and RSS for the app, the server and the server's children by PID, and reads
the timings the app writes when `GOIDEA_BENCH_IDLE` asks it to hold the window
open. The start scenario is launched once as a warmup and then measured.
Results go to `build/bench/<timestamp>/` (`summary.md` plus per-scenario JSON
and logs).

```sh
scripts/bench.sh                     # report against the budgets
GOIDEA_BENCH_ENFORCE=1 scripts/bench.sh   # exit 1 when over budget
```

Only numbers the script actually observes are reported. Keystroke-to-buffer
latency, sync round-trip and tab-switch time need a visible window and real
input, which the offscreen hooks do not inject, so they are marked unmeasured
rather than estimated. `firstFrameMs` is the window's first drawn frame; for
the 1M-line scenario the file is also confirmed to be open in a tab, but the
pixels of the text are not inspected offscreen.

## Measured

First recorded run: 2026-11-13, macOS 27.2.0 arm64, Go 1.27.3, server
`openidea-server-macos-aarch64.gz` from v0.1.41 (sha256
`650828f430a59f6d9cc8bc5553887a1ad821f8686cf68c052f1c4822878a5f4f`),
`build/bench/20261113-160605/`.

| Path | Target | Measured |
| --- | --- | --- |
| Cold start to first frame (warm caches) | ≤ 1.5 s | 110 ms |
| Startup to backend ready (`initialize`) | ≤ 2 s | 262 ms |
| 5 000-file Maven project, project model ready | ≤ 10 s | 287 ms for a 10 000-file tree |
| Keystroke-to-buffer latency | ≤ 16 ms | not measured offscreen |
| Buffer-to-backend sync | ≤ 400 ms | not measured offscreen |
| Open a 1 000 000-line file | ≤ 2 s | 121 ms to first frame; 1 tab |
| Idle CPU (window open, no input) | ≤ 1 % of one core | 0.00 % |
| Resident memory with one 10 000-line file | ≤ 400 MB app + server | 102 MB |
| 100 open tabs | no orphan; switch ≤ 50 ms | 100 tabs, no orphan; switch not measured |
| 20 concurrent tasks | no unbounded queue growth | 20 searches, peak 20, all answered in 21 ms |

The tree in the start scenario is the 3-file Maven fixture; the 10 000-file
measurement is the `tree10k` scenario. Idle CPU and RSS are sampled after a
six-second settle, so they describe a window that has finished starting up.
"Tree visible" is not observed offscreen; only the backend becoming ready is.
