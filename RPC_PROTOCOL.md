# RPC protocol

GoIDEA speaks JSON-RPC 2.0 to `openidea-server --stdio` with
`Content-Length` framing. The Go client is `internal/openidea`; this document
describes what the client sends, accepts, and relies on.

## Framing

```
Content-Length: <bytes>\r\n
\r\n
{"jsonrpc":"2.0", ...}
```

- Header names are case-insensitive; CRLF or LF line endings are accepted;
  blank lines before a header are skipped (`internal/openidea/frame.go:18-51`).
- Body limit 256 MiB, header line limit 8 KiB (`frame.go:14-15`).
- Anything on stdout before the first header is passed to a noise callback, not
  parsed as a response (`frame.go:63-72`). stdout remains protocol-only.
- stderr is logged, never parsed (`process.go:106-108`).

## Messages

Requests carry an id; notifications omit it; responses carry `result` or
`error{code,message,data}` (`internal/openidea/conn.go:36-49,184-243`).
Client ids are an atomic int64 from 1, kept as decimal strings in a pending map
(`conn.go:56,171-180`).

The handshake is `initialize {version: 2}`; the reply
`{version, protocol, os, arch, home}` must report `protocol == 2` or the app
refuses to continue. `commands` then returns the backend's command list
(`process.go:116-168`).

## The `invoke` envelope

Every feature call is the JSON-RPC method `invoke`:

```json
{"window":"goidea","cmd":"doc_edit","args":{...}}
```

`window` is always `"goidea"` (`internal/app/app.go:30`), so the backend never
shares documents or state with the user's other OpenIDEA windows. The client
uses these commands (`internal/openidea/*.go`, `internal/app/*.go`):

| Area | Commands |
| --- | --- |
| Project | `open_project`, `project_current`, `project_trust_status`, `project_trust_set`, `mark_recent`, `find_files` |
| Documents | `doc_open`, `doc_edit`, `doc_snapshot`, `doc_save`, `doc_close`, `doc_outline` |
| Language | `lsp_completion`, `lsp_hover`, `lsp_goto`, `lsp_format`, `lsp_code_action_query`, `lsp_apply_code_action`, `lsp_prepare_rename`, `lsp_rename`, `lsp_references`, `lsp_workspace_symbols`, `lsp_servers` |
| Workspace edits | `apply_workspace_edit`, `undo_workspace_edit`, `redo_workspace_edit` |
| Run | `run_configs`, `run_start`, `run_stop` |
| Debug | `java_resolve_launch`, `java_debug_server`, `debug_start`, `debug_set_breakpoints`, `debug_threads`, `debug_stack_trace`, `debug_scopes`, `debug_variables`, `debug_evaluate`, `debug_resume`, `debug_pause`, `debug_step`, `debug_stop` |
| Search and VCS | `search_start`, `vcs_status`, `vcs_file_at`, `vcs_diff_hunks` |
| Terminal | `term_spawn`, `term_write`, `term_kill`, `term_default_shell` |
| Database | `db_sources`, `db_connected`, `db_connect`, `db_disconnect`, `db_schemas`, `db_tables`, `db_statements`, `db_execute`, `db_save_sources` |
| Tools | `tools_list`, `tools_install`, `tools_uninstall` |
| Control | `cancel_request` |

## Streaming channels

Long-running or streaming calls name a channel in their arguments with
`{"$channel": id}` (or `onEvent`, `onPartial`, `onOutput`). The server publishes
each increment as the notification `channel`:

```json
{"id": 7, "data": {"type":"progress","fraction":0.4,"text":"…"}}
{"id": 7, "data": {"type":"line","text":"…"}}
```

The app routes by channel id to the run log, tool installer, search results or
watch expressions (`internal/app/run.go:422-440`, `internal/app/tools.go`).
Terminal output uses `onOutput`; run output uses `onOutput` with
`{"chunk":…,"stream":"stdout|stderr|system"}`.

## Cancellation

Calls that may be superseded pass a nonzero `request` id in their arguments.
Cancelling is `cancel_request {id}`. Search, usages and intentions do this when
the user types again or closes the panel
(`internal/app/search.go:60-111`, `usages.go:109`, `intents.go:205`). Tool
installs use a separate request id so cancelling one install does not disturb
language services (`internal/app/tools.go`).

## Errors

Responses may carry `error.data` with a `kind` string. `outOfSync` is singled
out structurally: the app then resynchronizes the document instead of reporting
a user-visible failure (`internal/openidea/errors.go:26-68`,
`internal/docbridge/document.go`). All other errors surface in the status bar
and the log with their kind and message.

## Backpressure

Queues are bounded. The connection's notification queue is 8192; when it is
full the connection is closed with `ErrOverflow` rather than growing without
limit (`conn.go:70-140`). The manager's streams are 8192 (main) and 16
(status); `Listen` copies with a 1024 default and drops on lag
(`manager.go:53-119`). Run output keeps at most 3000 lines in the view
(`run.go:443-504`). Nothing in the client has an unbounded queue.

## Reconnect

An unexpected connection close restarts the server up to 5 times with
`attempt² × 200 ms` backoff; a launch failure is retried 3 times at
250 ms × attempt (`manager.go:158-249`). After a restart the app reopens the
project and every open document and resets run/debug/terminal UI state, so the
window never keeps a stale session. Unsaved text is preserved across the
restart (`document.go:524-564`).

## Timeouts

| Step | Timeout |
| --- | --- |
| `initialize` | 15 s |
| `open_project` / `project_current` poll | 60 s, polled every 60 ms |
| trust query | 30 s |
| `run_configs` | 60 s |
| `run_stop`, `debug_stop` | 15 s |
| tool install | 20 min per smoke run; the UI has no fixed deadline and can cancel |

Set `GOIDEA_RPC_TRACE=1` to log every response body during diagnosis
(`conn.go:119`).
