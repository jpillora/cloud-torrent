# Handover — cloud-torrent AI rewrite (`airewrite`)

Status as of the last `/work` session. Read this, then `md ready` for the next task.

## TL;DR

- **Milestones M0 + M1 are complete** (md tasks #2–#15 closed). The core product loop works
  end-to-end and is browser-verified: **add a torrent in the Omni bar → watch it download in the
  Torrent engine → browse/stream the files in Downloads**, with a live Footer and realtime sync.
- **Remaining: M2 (#16–#20), M3 (#21–#22), M4 (#23–#26).** Nothing is blocked except #22 (needs #21),
  #23 (CSS, after the UI tasks) and #26 (final parity gate, after everything).
- Working tree is **clean**; everything is committed on branch `airewrite` (an orphan branch — the
  original implementation is on `master` and is the **functional reference** for parity).
- This is built task-by-task via the `/work` loop (see below). Just run `/work` to continue.

## What this is

A ground-up rewrite of cloud-torrent preserving the original UX (see `SPEC.md` for the full spec).
Stack: **Go** backend (anacrolix/torrent engine, jpillora/velox realtime sync) + **Vue 3 + Vite +
TypeScript** frontend. Go structs are the single source of truth for data shapes — TypeScript types
are **generated** from them with `tygo`. The built SPA is embedded into the Go binary (single binary).

## Current architecture

```
main.go                  CLI entry (jpillora/opts) -> server.Server.Run
engine/                  torrent engine (wraps anacrolix/torrent)
  config.go              Config (synced + persisted to cloud-torrent.json)
  torrent.go             Torrent/File view-models, percent(), download-rate
  engine.go              Engine: Configure/Close, NewMagnet/NewTorrent, GetTorrents,
                         Start/Stop/Delete, StartFile/StopFile, defaultPort, str2ih
server/
  state.go               root State (velox.State + sync.Mutex json:"-"; Config/Downloads/
                         Torrents/Users/Stats). SearchProviders still TODO (#21).
  server.go              Run(): wires engine, loads/persists config, 1s poll + 5s stats loops,
                         tracks Users; handler() builds the mux (/sync /api/ /download/ /js/velox.js /)
  api.go                 typed POST /api/* (configure/url/torrentfile/magnet/torrent/file)
  files.go               FsNode + listFiles() + downloadHandler (file stream / dir-as-zip / delete)
  stats.go               Stats + SystemStats (gopsutil)
  *_test.go              unit tests; e2e_test.go is //go:build e2e; fixtures_test.go pins Sintel
static/
  embed.go               //go:embed all:dist  -> serves static/dist (the built SPA)
  dist/.gitkeep          placeholder so `go build` compiles before any frontend build
frontend/src/
  main.ts                mounts App, startSync()
  state.ts               reactive<State> singleton + connection status (fed by velox)
  velox.ts               typed wrapper around the served /js/velox.js client
  api.ts                 typed POST /api/* client
  format.ts              bytes()/round()/ago() (dayjs)
  types/                 generated.ts (barrel) re-exports engine.gen.ts + server.gen.ts (DO NOT EDIT)
  components/            App=Header,Omni,MagnetEditor,Torrents,TorrentItem,Downloads,FileNode,Footer
tygo.yaml                Go -> TS generation config
generate.go              //go:generate go tool tygo generate
Makefile                 gen-types, check-types
```

## Build / run / test

**Type generation** (run after changing any synced Go struct):
```
make gen-types      # go tool tygo generate -> frontend/src/types/*.gen.ts
make check-types    # regenerate + git diff --exit-code (CI freshness guard)
```

**Frontend** (note: build outputs to `../static/dist`, NOT frontend/dist):
```
npm --prefix frontend install
npm --prefix frontend run build      # -> static/dist (type-checks via vue-tsc first)
npm --prefix frontend run dev        # vite :5173, proxies /sync /api /download /js/velox.js -> :3000
```

**Backend / single binary** (build the frontend FIRST so the embed is current):
```
go build -o /tmp/ct .                # from the REPO ROOT (see CWD gotcha)
/tmp/ct --port 3000                  # serves embedded SPA + API + sync
```

**Tests:**
```
go test ./...                            # unit (fast, hermetic; engine/server)
go test -tags e2e -run E2E ./server/...  # live core-loop E2E (downloads from a real fixture, ~6s)
```

**Browser verification** — CLAUDE.md mandates the **Chrome DevTools MCP** (`.mcp.json`). It connects
to a Chrome on `127.0.0.1:9223`. If none is running, launch one:
```
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --remote-debugging-port=9223 --user-data-dir=/tmp/chrome-ct-debug --headless=new \
  --no-first-run --no-default-browser-check about:blank &
```
Then run the binary (I run it from a scratch dir, e.g. `/tmp/ct-run`, so `downloads/`+`cloud-torrent.json`
don't land in the repo), open `http://localhost:<port>/` via the MCP, and drive the UI.

## Workflow (how this project is being built)

- **Tasks live in `md`** (TASKS.md). Never edit TASKS.md directly. `md ready` → next task; `md get
  <id>`; `md set-status <id> inprogress|closed`. Dependencies are wired; ready = unblocked.
- **The `/work` loop** picks the top ready task, implements exactly its scope, verifies, then commits
  with `Closes #<id>` (Conventional Commits, co-authored, **no push**). One task per run.
- **Screenshot rule (CLAUDE.md):** when a screenshot validates a UI task, send it to the user via the
  announce endpoint (`https://announce.jpillora.com`, base64 `attachment`; global instructions have
  the exact curl). Done for M1.
- Commit the TASKS.md change with each task. Stage only that task's files.

## Hard-won gotchas (read before you trip on them)

1. **velox:** use `velox.SyncHandler(&s.state)` to bind the **embedded** `velox.State` — `velox.Sync`
   wraps a *separate* State, so `state.Push()` won't reach connections. And `init()` must run (we call
   `SyncHandler` once at startup) **before** any `Push()`, or the first client gets a stale partial
   delta instead of the full state. See `server.go` comments.
2. **tygo:** it *overwrites* a shared `output_path`, so each Go package emits its own `*.gen.ts` and
   `generated.ts` is a hand-written barrel. Non-data structs are excluded via `exclude_files`
   (server.go's `Server`, engine.go's `Engine`). Cross-package field types (e.g. `engine.Config` in
   `State`) become `any` unless mapped — fixed with `frontmatter` import + `type_mappings`
   (`engine.Config: "Config"`). `time.Time -> string`. tygo is a **go tool** (`go tool tygo`).
3. **go:embed can't traverse `..`** — that's why the frontend builds to `static/dist` (so `static/embed.go`
   can `//go:embed all:dist`). `all:` + the committed `static/dist/.gitkeep` keep `go build` compiling
   before any frontend build.
4. **Binary embeds `static/dist` at compile time** — after a frontend rebuild you MUST `go build` again
   to serve new assets. (Verify the served bundle hash in `index.html`.)
5. **CWD trap:** the Bash tool's working dir persists between calls. `cd frontend && npm run build`
   leaves you in `frontend/`, so a following `go build .` runs there and **fails silently** ("no Go
   files"). Use `npm --prefix frontend run build` from the repo root, or always absolute-`cd`.
6. **TS 5.7 typed arrays:** `Uint8Array<ArrayBufferLike>` isn't assignable to `BodyInit`; `api.ts`
   casts via `as unknown as BodyInit` for the torrentfile upload.
7. **DeleteTorrent** drops the torrent but does **not** delete downloaded files on disk (master parity).
8. **str2ih** guards `len==40` before hex-decoding (master would panic the fixed-size array on long input).
9. **E2E timing:** poll for `Percent > 0`, not a fully completed file — piece order can leave any single
   file < 100% for a long time even while overall progress is fine.
10. **No favicon yet** → a harmless `/favicon.ico` 404 in the console; fold a favicon into #23.
11. Tests that spin a real torrent client bind a free port + touch the network (DHT); they're either
    `-short`-skipped or `e2e`-tagged.

## Remaining work

All ready unless noted. Read `md get <id>` for full scope; verify UI tasks in-browser via the MCP and
send a validating screenshot.

- **#16 Header titlebar + settings/config editor** — logo/title, status icons (spinner/settings/magnet/
  connection lightning), collapsible config editor (toggles for bools, inputs otherwise) → `api.configure`.
  Header.vue is still a placeholder.
- **#17 .torrent upload** — full-page drag-drop cover + Omni upload-icon click → `api.torrentfile`
  (the API + `api.ts` client already exist).
- **#18 Per-file start/stop** — file-level Start/Stop in TorrentItem's files table → `api.file`
  (note `StopFile` returns "unsupported", per master). Likely also the per-file trash/preview controls.
- **#19 Media previews + spacebar** — inline audio/img/video preview in FileNode; space toggles the
  in-view player.
- **#20 Connect overlay + global error banner** — overlay when `!connection.connected`
  (Connecting/Disconnected); dismissible error banner. `state.ts` already exposes `connection`.
- **#21 Scraper search backend** — `server/search.go`: jpillora/scraper, `/search/:provider` (+ `/item`),
  30-min provider-config refresh, publish into `State.SearchProviders` (add that field + regen types).
- **#22 Omni search mode UI** (needs #21) — provider select, results table, Load more, item resolution,
  localStorage(tcProvider). Omni.vue already detects `search` mode but does nothing yet.
- **#23 CSS / visual parity** — port `master`'s look (it currently uses minimal CSS + emoji icons); add
  a favicon.
- **#24 Auth + TLS + request logging** — `--auth` (cookieauth), `--cert/--key` TLS (keep http2 disabled),
  `--log` (requestlog); compose into `handler()`.
- **#25 Packaging** — GoReleaser, Dockerfile, CI (gen-types → vite build → go build), README.
- **#26 Full parity-checklist vs master** (final gate) — SPEC §14.

## References

- `SPEC.md` — full spec (architecture, data model, the 5 sections, API, milestones, parity checklist).
- `master` branch — original implementation; the parity oracle. `git show master:<path>`.
- Fixture: Sintel from <https://webtorrent.io/free-torrents> (pinned in `server/fixtures_test.go`).
