# Meads (`md`) Task Tracking Context

## Overview

`md` is a git-native task tracker that stores all tasks in a single file — `TASKS.md` (Markdown) or `TASKS.csv` (CSV). No database, no config files — just your task file and git.

## Essential Commands

### Finding Work
- `md ready` - Show open tasks not blocked by dependencies (sorted by priority)
- `md list` - List all tasks
- `md list --json` - List all tasks as JSON
- `md get <id>` - Get a specific task
- `md get --json <id>` - Get a specific task as JSON

### Creating Tasks
- `md add "Fix the login bug"` - Add a simple task
- `md add "bug: Fix login P1. Session cookie expires"` - Rich input parsing
  - Type prefix: `bug:`, `task:`, `feature:` (optional)
  - Priority: `P0`-`P9` (0=critical, 4=backlog, default=P2)
  - Title: everything before first `.`
  - Description: everything after the `.`
- `md add --title="Fix login" --type=bug --priority=P1 --description="Details here"` - Flag-based
- `md add --title="Fix login" --description-file=/path/to/notes.md` - Description from file

### Updating Tasks
- `md update <id> --status=draft|open|inprogress|closed` - Update status
- `md update <id> --priority=P1` - Update priority
- `md update <id> --title="New title"` - Update title
- `md update <id> --description-file=/path/to/notes.md` - Update description from file
- `md set-status <id> <status>` - Shorthand for status changes
- `md del <id>` - Delete a task

### Dependencies
- `md add-dep <child> <parent>` - Make child depend on parent
- `md add --depends-on=<id> "Task title"` - Add task with dependency
- Tasks blocked by unclosed dependencies are excluded from `md ready`

## Common Workflows

**Starting a session:**
```bash
md ready              # Find available work
md get <id>           # Review task details
md set-status <id> inprogress  # Claim it
```

**Creating dependent tasks:**
```bash
md add "feature: Build API endpoint"    # Returns ID, e.g. 5
md add "Write tests for API. Cover edge cases" --depends-on=5
```

**Completing work:**
```bash
md set-status <id> closed    # Mark task done
```

## Rules
- **Do NOT read or edit the task file (TASKS.md or TASKS.csv) directly** - always use `md` commands to read and modify tasks
- **ALWAYS include changes to the task file in the next git commit after task is closed**
- Task IDs are auto-assigned integers
- Git handles versioning and history
- The file uses optimistic locking for concurrent access

# Front-end Testing

Use the **Chrome DevTools MCP** for all front-end / UI testing. Drive the running
app in a real browser to verify UI behaviour and end-to-end flows — e.g. add a
torrent in the Omni bar → track progress in the Torrent engine → download a file
— rather than asserting from code alone. Reach for the Chrome DevTools MCP
whenever validating the Vue front-end or confirming a UI change works.

When a screenshot successfully validates a task, send it to the user via the
announce endpoint (`https://announce.jpillora.com`, with an `attachment` — see
the global instructions for image attachments).

# Project Architecture (the `airewrite` rewrite)

Ground-up rewrite of cloud-torrent preserving the original UX. **Go** backend
(`anacrolix/torrent` engine, `jpillora/velox` realtime sync) + **Vue 3 + Vite +
TypeScript** frontend, compiled to a **single binary** (the built SPA is
`go:embed`-ed). Go structs are the single source of truth — TS types are
**generated** from them with `tygo`. See `SPEC.md` for the full spec. The
**`master` branch is the parity oracle** — `git show master:<path>` to compare;
it's the original AngularJS + Semantic-UI build.

Layout: `main.go` (CLI via jpillora/opts) → `server/` (state/server/api/files/
stats/search) → `engine/` (anacrolix wrapper) → `static/` (embeds `dist/`) →
`frontend/src/` (Vue). `tygo.yaml` + `generate.go` drive type-gen; `Makefile`
has `gen-types`/`check-types`.

## Build / run / test (and the CWD trap)

```
npm --prefix frontend install                  # first time
npm --prefix frontend run build                # vue-tsc + vite -> static/dist (NOT frontend/dist)
npm --prefix frontend run dev                  # vite :5173, proxies /sync /api /download /js/velox.js -> :3000
go build -o /tmp/ct .                           # from REPO ROOT; embeds static/dist
go test ./...                                   # unit (fast, hermetic)
go test -tags e2e -run E2E ./server/...         # live core-loop E2E (downloads a real fixture, ~6s)
make check-types                                # regen types + git diff --exit-code (CI guard)
```

- **CWD trap:** the Bash tool's working dir persists. `cd frontend && npm build`
  leaves you in `frontend/`, so a following `go build .` runs there and fails
  ("no Go files"). Always `npm --prefix frontend …` from the repo root.
- **Rebuild order:** the binary embeds `static/dist` **at compile time** — after
  any frontend change you MUST rebuild the frontend *then* `go build` again, or
  you'll serve stale assets.
- For browser testing, run the binary from a scratch dir (e.g. `/tmp/ct-test`)
  so `downloads/` + `cloud-torrent.json` don't litter the repo.

# Hard-won learnings (read before you trip on them)

## Build & embed
- Frontend builds to `../static/dist` (vite `outDir`), not `frontend/dist` —
  because `go:embed` **can't traverse `..`**, so the embed must live under
  `static/`. The committed `static/dist/.gitkeep` (+ `//go:embed all:dist`) keeps
  `go build` compiling before any frontend build. `static/dist/*` is gitignored.

## Type generation (tygo)
- tygo is a **go tool**: `go tool tygo generate` (via `make gen-types`).
- It **overwrites a shared output**, so each Go package emits its own `*.gen.ts`
  and `frontend/src/types/generated.ts` is a **hand-written barrel** re-exporting
  them. Don't edit `*.gen.ts`.
- **Cross-package field types become `any`** unless mapped (e.g. `engine.Config`
  in `State` → fixed via frontmatter import + `type_mappings`). `scraper.Config`
  is intentionally left `any`. `time.Time → string`.
- Non-data structs (`server.Server`, `engine.Engine`) are excluded via
  `exclude_files`. After changing any **synced** struct, run `make check-types`
  and commit the regenerated `*.gen.ts` (CI fails if stale).

## velox realtime sync (`/sync`)
- Bind the **embedded** `velox.State` with `velox.SyncHandler(&s.state)` once at
  startup (NOT `velox.Sync`, which wraps a separate State so `state.Push()` won't
  reach clients). `SyncHandler` must run **before any `Push()`**.
- velox is **SSE by default**; its built-in auto-retry is **opaque** — no
  waiting/attempting signal and it **swallows SSE connection errors**. We disable
  it (`{retry:false}`) and drive the reconnect loop ourselves in `frontend/src/
  velox.ts` (backoff + an attempt-timeout backstop, since velox never reports a
  failed SSE attempt; surfaced via `ConnStatus`). Retry-on-`online`/refocus too.
- **gzip must wrap the whole handler chain with `minSize 0`** — otherwise
  gziphandler buffers to its threshold and the `/sync` SSE stream stops flushing
  in realtime (clients hang on "Connecting"). See `server.go handler()`.
- **HTTP/2 must stay disabled** (`http.Server{TLSNextProto: map[...]{}}`) — velox
  streaming is incompatible with it. Matters under `--cert/--key` TLS.

## Torrent engine (anacrolix) — see `engine/`
- Engine owns its mutable cache/config/client behind one mutex. `*Locked` helpers
  require that lock. `GetTorrents` returns detached torrent/file DTO snapshots;
  API actions explicitly publish a fresh snapshot before pushing realtime state.
- AutoStart-only configuration changes keep the live client. Client-level
  changes validate the directory/port, preserve metainfo and file start flags,
  and rebind torrents to the replacement client. Failure restores the previous
  configuration. anacrolix closes listener sockets asynchronously, so only bind
  conflicts get a bounded retry; there is no unconditional restart sleep.
- `DisableEncryption=true` requires plaintext peer transport (no obfuscation);
  false retains upstream encryption preference with plaintext fallback.
- Whole-torrent Stop disables data upload/download and clears file priorities
  while retaining metadata and its live handle. Start restores priorities and
  resumes; only Delete drops the handle.
- `DeleteTorrent` drops the torrent but does **NOT** delete files on disk
  (master parity). `StopFile` returns `"unsupported"` (no per-file pause).
- `str2ih` guards `len==40` before hex-decoding (a longer string panics the
  fixed-size array). `percent()`/`DownloadRate` formulas are byte-identical to
  master — don't "simplify" them.
- Add → `<-GotInfo()` → auto-start according to the initial `AutoStart` policy.
  An explicit Start before metadata sets pending whole-torrent intent too; the
  metadata callback schedules files after they exist. Stop cancels that intent,
  and reconfiguration preserves it. Partial file starts clear whole-torrent intent.
- **`anacrolix/torrent` does NOT compile on OpenBSD** (`unix.SEEK_DATA`
  undefined) — OpenBSD is dropped from the goreleaser matrix.
- E2E timing: poll for `Percent > 0`, not a fully-complete file — piece order can
  leave any single file < 100% for ages. Tests that spin a real client touch the
  network (DHT) and are `-short`/`e2e`-gated.

## Configuration persistence
- Saves serialize the engine transition and file replacement. Stage and sync JSON
  beside the destination before applying settings; atomically rename afterward.
  A replacement failure restores the previous engine config and returns an error.
  Startup accepts a missing config, but reports read/parse failures.

## HTTP server chain — `server/server.go`
- `handler()` composes exactly as master: route mux → **gzip** (whole chain,
  minSize 0) → **cookieauth** (`--auth user:pass`) → **requestlog** (`--log`),
  log outermost. TLS via `--cert-path`/`--key-path` (both required).
- `/download/` uses `os.Root` in `files.go` to confine reads, archives and
  deletion during filesystem resolution. Relative symlinks within the root work;
  external/absolute symlinks and root requests are rejected. Archives omit
  symlinks; deleting an internal symlink removes the link, retaining its target.
- The **download directory is config-file/UI only — there is no CLI flag.**
  Engine port defaults to **50007**; HTTP port to **3000**.

## Search backend (`server/search.go`) — it scrapes live sites
- Providers come from a **remote config** (a gist) fetched on startup + every
  30 min, replacing the embedded default. So the live provider set differs from
  the embedded one.
- **Providers are flaky/dead** — they scrape real torrent sites that churn
  domains (e.g. limetorrents → NXDOMAIN, tpb removed). A failing search is almost
  always the provider, not a bug. `nyaa` is usually up for testing.
- `jpillora/scraper` returns `{"error": "..."}` for failures but **does NOT
  escape quotes** in the message (Go quotes URLs), so the body is often **invalid
  JSON**. The search client (`api.ts`) parses leniently (regex fallback) — don't
  assume `res.json()` succeeds on an error response.

## Frontend / Vue conventions — `frontend/src/`
- **Icons: `@mdi/js` paths via `components/Icon.vue`** (inline `<svg
  fill="currentColor">`). Tree-shaken, and they inherit `currentColor` so
  `.green`/`.red` state colours actually work (emoji ignore CSS colour — don't
  reintroduce emoji icons).
- **Async action buttons use `components/AsyncButton.vue`** (`:action` runs,
  button disables + spins until it settles; works inside `v-for` because each
  instance owns its state; uncaught errors → global banner). Use it for anything
  that hits the API.
- Cross-component view state (in-flight request count, config/omni toggles,
  global error banner) lives in `ui.ts`; synced server data in `state.ts`.
  `api.ts` bumps `ui.requests` around every request (header spinner).
- `u\NNNN` escapes only work in `<script>` string literals, **not** in template
  text (they render literally). Put glyphs/paths in script, bind with `{{ }}`.
- An `<input>` with **no `type` attribute is not matched by `input[type='text']`
  selectors** — it'll silently use the browser-default border. Style by class or
  add the type.
- **TS 5.7 typed arrays:** `Uint8Array<ArrayBufferLike>` isn't assignable to
  `BodyInit`; `api.ts` casts via `as unknown as BodyInit` for the `.torrent`
  upload body.

## Browser testing — Chrome DevTools MCP (techniques that work here)
- MCP connects to a Chrome on `127.0.0.1:9223`; launch headless if none. Drive
  the running binary, not a dev server, to test the embedded build. Launch with:
  ```
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
    --remote-debugging-port=9223 --user-data-dir=/tmp/chrome-ct-debug --headless=new \
    --no-first-run --no-default-browser-check about:blank &
  ```
- **SVG elements have no `.click()` method** — Vue's `@click` works for real
  clicks, but in `evaluate_script` use `el.dispatchEvent(new MouseEvent('click',
  {bubbles:true}))`.
- **Synthetic key events aren't "trusted"** so `media.play()` is blocked — use
  the `press_key` MCP tool (CDP) for spacebar/keyboard tests.
- `v-show`/reactive DOM updates flush on the **next tick** — `await` a short
  sleep before asserting the DOM after a state change.
- To build a real **drag-drop** with file bytes: fetch the app's own
  `/download/<file>` to get bytes into the page, make a `File`, attach a
  `DataTransfer` to a synthetic event via `Object.defineProperty(e,
  'dataTransfer', …)`.
- To get real `.torrent` bytes for upload tests, the Sintel torrent is at
  `https://webtorrent.io/torrents/sintel.torrent` (infohash in
  `server/fixtures_test.go`).

## CSS & mobile — `frontend/src/styles/main.css` (single global sheet)
- Styling is centralized in `main.css` (CSS-var palette; cards = `var(--border)`
  + `var(--radius)` + `var(--card-shadow)`). Keep the three primary panels (Omni
  input, torrent card, downloads tree) visually identical.
- **Prevent mobile zoom:** viewport meta has `maximum-scale=1, user-scalable=no`
  **and** `body { touch-action: manipulation }` (kills double-tap zoom). Keep
  inputs ≥16px to avoid iOS focus-zoom.
- Mobile (`@media (max-width:600px)`): the downloads row meta drops below the
  name (`flex-basis:0` on the name keeps the row control pinned to line 1) — so
  long filenames aren't squeezed. Touch has no hover, so row controls are
  revealed there.

## Packaging — `.github/`
- `goreleaser.yml` has a **`before` hook that runs `npm ci` + `vite build`** so
  the SPA is embedded into every release binary. CI = `check-types` → vite build
  → `go build`/`test`; tagged `v*` pushes publish GitHub prerelease binaries only. Docker image publishing is disabled.
- `Dockerfile` is multi-stage (node → golang → scratch). Container CWD is `/`, so
  the default `./downloads` resolves to `/downloads` (the volume); config is
  pinned there via `--config-path /downloads/cloud-torrent.json`.

## Local dev environment
- **LittleSnitch / macOS firewall** can block the binary's network (inbound LAN,
  and sometimes outbound to trackers/providers): symptom is `dial tcp … no such
  host`/`connection refused` or a LAN IP refusing while `localhost` works. The
  user must allow the binary's path.
- macOS has **no `timeout`** command — use the Bash tool's `timeout` param (or
  `gtimeout`). Always bound `curl` with `--max-time` (providers/trackers hang).
- `Date.now()`/`Math.random()` are fine in app code (the ban is only inside
  Workflow scripts).
