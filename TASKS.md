# TASKS

a [meads](https://github.com/jpillora/meads) (`md`) managed task log

* created: 2026-06-16T07:00:20Z
* updated: 2026-06-16T12:15:52Z

## 2. M0: Initialize Go module + CLI entry

* status: closed
* priority: P1
* type: task
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T07:22:49Z

**Objective:** Stand up the Go module and the binary entrypoint so everything else has a home.

**Scope**
- `go.mod` — `module github.com/jpillora/cloud-torrent`; add deps at the versions used on `master`: `anacrolix/torrent`, `jpillora/velox`, `jpillora/opts`, `shirou/gopsutil/v3`, `jpillora/scraper`, `jpillora/archive`, `jpillora/cookieauth`, `jpillora/requestlog`, `NYTimes/gziphandler`, `skratchdot/open-golang`.
- `main.go` — construct `server.Server{ Title:"Cloud Torrent", Port:3000, ConfigPath:"cloud-torrent.json" }`, parse with `jpillora/opts` (`.Version()`, `.PkgRepo()`, `.SetLineWidth(96)`), then `s.Run(version)`.
- `server/server.go` skeleton — `Server` struct with the CLI flag fields + tags (`title/port/host/auth/config-path/key-path/cert-path/log/open`) and a stub `Run(version string) error`.

**Reference (master):** `main.go`, `server/server.go` (Server struct + flag tags).

**Done when**
- `go build ./...` succeeds.
- `./cloud-torrent --help` prints every flag and `--version` works.

SPEC: §3, §12

## 3. M0: Scaffold Vue 3 + Vite + TypeScript frontend

* status: closed
* priority: P1
* type: task
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T07:28:22Z

**Objective:** Create the Vue 3 + Vite + TypeScript frontend shell that the five sections will live in.

**Scope**
- `frontend/` with `package.json`, `vite.config.ts`, `tsconfig.json`, `index.html`, `src/main.ts`, `src/App.vue`.
- Vue 3 (`<script setup>`, Composition API) + TypeScript + Vite. Add `dayjs` (replaces `moment`).
- `App.vue` lays out the single-column shell in order: **Header → Omni → Torrents → Downloads → Footer** as empty placeholder components.
- npm scripts: `dev` (Vite dev server :5173) and `build` (→ `frontend/dist`).

**Reference (master):** `static/files/index.html` (section order + overall layout).

**Done when**
- `npm run dev` serves the placeholder shell.
- `npm run build` emits `frontend/dist`.

SPEC: §3

## 4. M0: Go-struct -> TypeScript type generation (tygo)

* status: closed
* priority: P1
* type: task
* depends-on: 2,7,10
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T08:05:04Z

**Objective:** Make Go structs the single source of truth for data shapes by generating TypeScript from them.

**Scope**
- `tygo.yaml` mapping the `engine` + `server` packages → `frontend/src/types/generated.ts`; `type_mappings: time.Time -> string`.
- Tag embedded `velox.State` and `sync.Mutex` as `json:"-"` so both `tygo` and `encoding/json` skip them.
- Add `//go:generate tygo generate` and a `make gen-types` target.
- Freshness guard: a script (used in CI) that regenerates and runs `git diff --exit-code`.

**Reference:** SPEC §6.

**Done when**
- `make gen-types` produces `generated.ts` containing `Config`, `Torrent`, `File`, `FsNode`, `Stats`, `SystemStats`, `State` interfaces (plus API request types).
- Re-running produces no diff.

SPEC: §5, §6

## 5. M0: Embed frontend build + Vite dev proxy

* status: closed
* priority: P1
* type: task
* depends-on: 2,3
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T07:46:33Z

**Objective:** One binary in production; a smooth proxied dev loop.

**Scope**
- `static/embed.go` — `//go:embed dist` (the built `frontend/dist`) exposed via `http.FS`; in dev, fall back to an on-disk dir if present (parity with master's `serveFiles`).
- `vite.config.ts` dev proxy → `http://localhost:3000` for: `/sync`, `/js/velox.js`, `/api`, `/search`, `/download`.

**Reference (master):** `static/static.go`, `server/server.go serveFiles`.

**Done when**
- A production `go build` serves the embedded SPA.
- In dev, Vite proxies all backend routes to the Go server.

SPEC: §3, §12

## 6. M0: velox /sync realtime round-trip

* status: closed
* priority: P1
* type: task
* depends-on: 2,3,4,5
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T08:17:30Z

**Objective:** Prove the realtime stack end-to-end with a trivial synced value ("hello world" for velox).

**Scope**
- Minimal root `State` in `server` (embed `velox.State` + one demo field).
- Routes + handler chain: `GET /sync` (`velox.Sync`), `GET /js/velox.js` (`velox.JS`), gzip wrap.
- `frontend/src/velox.ts` — typed wrapper around the served velox client; `frontend/src/state.ts` — `reactive<State>()` singleton mutated in place by velox.
- Bind `onchange(connected)` → `connected`/`hasConnected`; render the demo field.
- Mutate the field server-side + `Push()` and confirm the browser updates with no reload.

**Reference (master):** `server/server.go handle()`, `static/files/js/run.js` (`velox("/sync", state)`, `onupdate`/`onchange`).

**Done when**
- Server-side value changes appear live in the browser; connection state is reflected in the UI.

SPEC: §4, §7

## 7. M1: Torrent engine (anacrolix/torrent wrapper)

* status: closed
* priority: P1
* type: feature
* depends-on: 2
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T07:54:56Z

**Objective:** Wrap `anacrolix/torrent` behind a small engine that produces the synced view-models.

**Scope**
- `engine/config.go` — `Config{ AutoStart, DisableEncryption, DownloadDirectory, EnableUpload, EnableSeeding, IncomingPort }`.
- `engine/engine.go` — `Engine`: `New`, `Config`, `Configure` (client lifecycle: `DataDir`, `NoUpload`, `Seed`, `ListenPort`), `NewMagnet`, `NewTorrent`, `GetTorrents`, `StartTorrent`, `StopTorrent` (drop), `DeleteTorrent`, `StartFile`/`StopFile`.
- `engine/torrent.go` — `Torrent` + `File` view-models with `Update()`/`updateLoaded()`, `percent()` and download-rate calculation.
- Behaviour parity: `IncomingPort` defaults to `50007` when out of range; on `GotInfo()` auto-start (respecting `Config.AutoStart`).

**Reference (master):** `engine/engine.go`, `engine/config.go`, `engine/torrent.go`.

**Done when**
- Unit tests cover `percent()` and magnet parsing.
- A magnet can be added + started at the engine level against a fixture.

SPEC: §5, §8

## 8. M1: Root State + velox push loops + system stats

* status: closed
* priority: P1
* type: feature
* depends-on: 6,7,10
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T08:28:49Z

**Objective:** Assemble the full velox-synced `State` and the background loops that keep it fresh.

**Scope**
- `server/state.go` — full `State{ Config, SearchProviders, Downloads, Torrents, Users, Stats }` (`velox.State` + `sync.Mutex` as `json:"-"`).
- `server/server.go` — wire the engine, load/persist `cloud-torrent.json`, `reconfigure()`, plus two goroutines:
  - **1s poll loop:** `Torrents = engine.GetTorrents()`, `Downloads = listFiles()`, then `Push()`.
  - **5s stats loop:** refresh `Stats.System`.
- `server/stats.go` — `SystemStats` + `loadStats()` via gopsutil (cpu/disk/mem) + `runtime` (goroutines, alloc).
- Track `Users` (velox `conn.ID()` → `RemoteAddr`) on sync connect/disconnect.

**Reference (master):** `server/server.go` (Run loops + `reconfigure`), `server/server_stats.go`.

**Done when**
- `/sync` streams a populated `State` that updates ~every second; system stats populate within 5s.

SPEC: §4, §5, §7

## 9. M1: HTTP action API (POST /api/*)

* status: closed
* priority: P1
* type: feature
* depends-on: 8
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T08:32:34Z

**Objective:** Typed JSON action API for every mutation the UI performs.

**Scope**
- `server/api.go` — `POST /api/*` handlers, request bodies defined as Go structs (generated to TS):
  - `configure` ← `Config`
  - `url` ← `{ url: string }` (fetch remote `.torrent`)
  - `torrentfile` ← raw `.torrent` bytes (→ metainfo → add)
  - `magnet` ← `{ uri: string }`
  - `torrent` ← `{ action: "start"|"stop"|"delete", infohash }`
  - `file` ← `{ action: "start"|"stop", infohash, path }`
- On success `Push()` (UI updates via velox, not the response body); errors return `400` text.
- **Intentional divergence from master:** clean typed JSON instead of colon-delimited strings; behaviour identical.

**Reference (master):** `server/server_api.go`.

**Done when**
- Each endpoint drives the engine; an integration test adds a magnet via `POST /api/magnet`.

SPEC: §8

## 10. M1: File serving + download tree (FsNode)

* status: closed
* priority: P1
* type: feature
* depends-on: 2
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T07:59:00Z

**Objective:** List downloaded files as a tree and serve/delete them over HTTP.

**Scope**
- `server/files.go`:
  - `FsNode{ Name, Size, Modified, Children }` + recursive `listFiles()` walk — skip dotfiles + non-regular files, cap at 1000 entries.
  - `GET /download/*path` — `http.ServeContent` for files; stream a **zip** for directories (`jpillora/archive`).
  - `DELETE /download/*path` — `os.RemoveAll`.
  - **Security:** reject any path escaping `DownloadDirectory` (prefix check; never equal to the root).

**Reference (master):** `server/server_files.go`.

**Done when**
- A file streams, a directory downloads as a valid zip, traversal is blocked, delete works.
- The `FsNode` tree is consumed by the poll loop (task: Root State + push loops).

SPEC: §8, §9 §D

## 11. M1: Omni bar - magnet + torrent URL modes

* status: closed
* priority: P1
* type: feature
* depends-on: 9,3,4
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T09:08:15Z

**Objective:** The universal Omni input — first two modes (magnet + torrent URL), the heart of "choose a torrent".

**Scope**
- `frontend/src/api.ts` — typed `POST /api/*` client.
- `Omni.vue` + `MagnetEditor.vue`. Live mode detection on input:
  - `^https?://` → **url** mode → "Start Torrent" → `api.url`.
  - `^magnet:?` → **magnet** mode → "Load Magnet" → `api.magnet`, plus an **Edit** toggle.
  - empty → **upload** icon (wired in M2).
- Trailing icon reflects mode (magnet/upload).
- Magnet parse/build: `xt=urn:btih:…&dn=…&tr=…`, infohash validation (`^[A-Za-z0-9]+$`), `omnierr` surfacing; persist last input in `localStorage` (`tcOmni`).

**Reference (master):** `omni-controller.js` (`parse`/`parseMagnet`/`magnetURI`/`parseMagnetString`), `omni.html`.

**Done when**
- Pasting a magnet or torrent URL and submitting adds a torrent (visible in synced state).

SPEC: §9 §B, §10

## 12. M1: Torrent engine UI (live progress + controls)

* status: closed
* priority: P1
* type: feature
* depends-on: 8,3,4
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T09:19:58Z

**Objective:** The Torrent-engine section — live, controllable torrent cards.

**Scope**
- `Torrents.vue` + `TorrentItem.vue`. Header "Torrents" + count; empty state "Add torrents above".
- Per card: loading dimmer until `Loaded`; name (links to the `.zip` once ready), `#InfoHash`, blue progress bar (animated `0<Percent<100`).
- Controls (→ `api.torrent`): **Files** toggle · **Start** (green, when stopped) · **Stop** (red, when started) · **Cancel/Remove** (delete; "Cancel" while `!Loaded`, "Remove" once loaded).
- Status line when started: `Downloaded / Size — Percent% — Rate/s` (muted zeros).
- Files sub-table (when toggled + loaded): per-file name, live `%` + mini progress bar, size, green check at 100%, footer totals.

**Reference (master):** `torrents-controller.js`, `torrents.html`.

**Done when**
- An added torrent shows live progress; Start/Stop/Delete + Files toggle all work.

SPEC: §9 §C, §10

## 13. M1: Downloads tree UI + file streaming

* status: closed
* priority: P1
* type: feature
* depends-on: 10,8,3
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T09:25:22Z

**Objective:** The Downloads section — browse and stream what's on disk.

**Scope**
- `Downloads.vue` + recursive `FileNode.vue`. Header "Downloads" + free-disk (`diskTotal − diskUsed`); empty state "Download files above".
- Recursive `FsNode` tree:
  - Icon by type/state: spinner while that file is actively downloading; otherwise folder (open/closed) or a file icon specialised for audio/image/video by extension.
  - Folders collapsible; auto-collapse when `Modified` > 24h ago.
  - Files link to `download/<path>`; show `Size` + "updated <ago>".
  - Cross-reference `state.Torrents[*].Files` by path to detect the downloading state.

**Reference (master):** `downloads-controller.js`, `downloads.html`, `download-tree.html`.

**Done when**
- Completed files appear in the tree and download/stream from the UI.

SPEC: §9 §D, §10

## 14. M1: Footer (version + resources)

* status: closed
* priority: P2
* type: feature
* depends-on: 8,3
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T09:28:37Z

**Objective:** The Footer — version + live resource usage.

**Scope**
- `Footer.vue` + `frontend/src/format.ts` (`bytes()`, `round()`, `ago()` via dayjs).
- Left: `github.com/jpillora/cloud-torrent version <Stats.Version>` · (`N users connected` when > 1) · `Go <Stats.Runtime>`.
- Right (when `Stats.System.Set`): `goroutines`, `gomem` (bytes), `cpu %`, `mem %`, `disk %`, and `up <Uptime ago>`.

**Reference (master):** `static/files/index.html` footer, `static/files/js/utils.js` filters (`bytes`/`round`).

**Done when**
- The footer reflects live `Stats` and updates over time.

SPEC: §9 §E

## 15. M1: Core-loop end-to-end test (webtorrent fixture)

* status: closed
* priority: P1
* type: task
* depends-on: 11,12,13,14
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T09:45:34Z

**Objective:** Prove the core loop (choose → track → download) against a real torrent.

**Scope**
- `testdata/` fixtures sourced from <https://webtorrent.io/free-torrents> (they carry WebSeeds → deterministic). Pin a known magnet/infohash (e.g. Sintel / Big Buck Bunny) in `fixtures.go` + a matching TS constant; document how to refresh.
- **Go integration** (`httptest` + real engine, network build tag): `POST /api/magnet` → poll `/sync` until `Torrent.Loaded` → `Started` → `Percent > 0` → assert files land on disk → `GET /download/<file>` returns bytes (and a directory returns a valid zip) → `torrent:delete` cleans up.
- **Playwright E2E:** type a magnet in the Omni bar → Load Magnet → assert a Torrent card appears, progress advances, a file shows in Downloads, and the download link serves content.

**Reference:** SPEC §10, §11.

**Done when**
- Both suites pass against a live fixture.

SPEC: §10, §11

## 16. M2: Header titlebar + settings/config editor

* status: closed
* priority: P2
* type: feature
* depends-on: 9,8,3
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T10:21:33Z

**Objective:** The Header section — title bar + the settings/config editor.

**Scope**
- `Header.vue`: logo icon (cloud, swaps to GitHub mark on hover) linking to the repo + `Stats.Title`; right-side status icons: request spinner (API in flight), **settings** (toggles config editor, green when open), **magnet** (toggles the Omni magnet editor), **connection** lightning (green/red).
- Config editor (collapsible): iterate `state.Config` → render a **toggle** for booleans and a labelled text/number input otherwise (keys shown spaced, e.g. `DownloadDirectory` → "Download Directory"). **Save** → `api.configure`; **Cancel** closes.

**Reference (master):** `index.html .title`, `config-controller.js`, `config.html`.

**Done when**
- Editing + saving config reconfigures the engine and persists to `cloud-torrent.json`.

SPEC: §9 §A

## 17. M2: .torrent upload (click + full-page drag-drop)

* status: closed
* priority: P2
* type: feature
* depends-on: 9,11
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T10:31:31Z

**Objective:** Add torrents by uploading `.torrent` files — drag-drop anywhere or click the Omni upload icon.

**Scope**
- Full-page **drop cover** (App-level): dragging `.torrent` files shows a "Drop torrent files here" cover; on drop, read each file as bytes → `api.torrentfile`.
- Omni **upload icon** click → hidden `<input type=file multiple accept=.torrent>` → same upload path.

**Reference (master):** `utils.js` (`ondropfile`/`onfileclick`), `run.js` (`uploadTorrent`).

**Done when**
- Dropping or choosing one or more `.torrent` files adds them as torrents.

SPEC: §9 §0, §9 §B

## 18. M2: Per-file start/stop controls

* status: closed
* priority: P2
* type: feature
* depends-on: 12,9
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T10:38:14Z

**Objective:** Per-file start/stop within a torrent.

**Scope**
- File-level Start/Stop controls in `TorrentItem.vue`'s files table → `api.file({ action, infohash, path })`.
- Reflect `File.Started` / `File.Percent` in the row.
- Match master semantics (StartFile supported; StopFile currently returns "Unsupported").

**Reference (master):** `torrents-controller.js submitFile`, `engine.go StartFile`/`StopFile`.

**Done when**
- Starting/stopping a single file calls the API and the UI reflects state.

SPEC: §8, §9 §C

## 19. M2: Media previews + spacebar play/pause

* status: closed
* priority: P2
* type: feature
* depends-on: 13
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T10:44:44Z

**Objective:** Inline media previews + the page-wide spacebar play/pause.

**Scope**
- `FileNode.vue` preview toggle for audio/image/video, sourcing `download/<path>` (rendered only when toggled on).
- App-level `keydown` on Space (when not typing in an input) → play/pause the first in-viewport `<video>/<audio>`.

**Reference (master):** `download-tree.html` preview block, `run.js` keydown handler.

**Done when**
- Media files preview inline; Space toggles playback of the visible player.

SPEC: §9 §0, §9 §D

## 20. M2: Connect overlay + global error banner

* status: closed
* priority: P2
* type: feature
* depends-on: 6
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T10:50:27Z

**Objective:** Connection overlay + global error banner — the app-shell feedback layer.

**Scope**
- When `!connected`, cover the page with a centered masthead showing `hasConnected ? "Disconnected" : "Connecting"` + a red lightning icon.
- Dismissible global error banner bound to a shared `err` value.

**Reference (master):** `index.html` connect-warning overlay + error message block.

**Done when**
- Dropping the connection shows the overlay; surfaced errors can be dismissed.

SPEC: §9 §0

## 21. M3: Scraper search backend (/search + provider refresh)

* status: closed
* priority: P2
* type: feature
* depends-on: 8
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T11:42:42Z

**Objective:** Backend torrent search via `jpillora/scraper` providers.

**Scope**
- `server/search.go`: embed the default provider config; `GET /search/:provider` (`query`, `page`) and `GET /search/:provider/item` (`item`); 30-minute auto-refresh of the remote provider config with backoff; publish `scraper.Config` into `State.SearchProviders`.

**Reference (master):** `server/server_search.go` (default config, `fetchSearchConfigLoop`, `normalize`).

**Done when**
- `/search/:provider` returns results JSON; providers appear in synced `State`.

SPEC: §8, §9 §B

## 22. M3: Omni search mode UI (results + item resolution)

* status: closed
* priority: P2
* type: feature
* depends-on: 11,21
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T11:51:29Z

**Objective:** The Omni bar's search mode UI (third mode) — find torrents and add them.

**Scope**
- Search mode in `Omni.vue` when input is plain text: provider `<select>`, **Search** / **Load more** pagination.
- Results table: Name (links to provider page), Size, Seeds/Peers, green download icon.
- `submitSearchItem` resolution: if the result has `magnet`/`torrent`, submit directly; else fetch `/search/:provider/item?item=<path>` and build a magnet from `magnet` | `torrent` | `infohash + trackers`.
- Persist last provider in `localStorage` (`tcProvider`).

**Reference (master):** `omni-controller.js` (`submitSearch`/`submitSearchItem`), `omni.html` results table.

**Done when**
- Searching a provider lists results; clicking download adds the torrent.

SPEC: §9 §B

## 23. M4: CSS / visual parity pass vs master

* status: closed
* priority: P3
* type: task
* depends-on: 11,12,13,14,16
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T12:00:48Z

**Objective:** Match the original look without Semantic UI / jQuery.

**Scope**
- Port `app.css` + section styles (`omni.css`, `torrents.css`, `downloads.css`) into scoped per-component CSS.
- Recreate the Semantic-UI visual language used by the app (icons, segments, progress bars, buttons, toggles) with modern CSS.
- Match spacing, typography (Lato), and the blue/green/red accent palette.

**Reference (master):** `static/files/css/*`.

**Done when**
- Running the rewrite side-by-side with `master` is visually faithful across all five sections.

SPEC: §1, §9

## 24. M4: Basic auth + TLS + request logging

* status: closed
* priority: P3
* type: feature
* depends-on: 6
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T12:06:05Z

**Objective:** Port the server's auth, TLS, and logging wrappers.

**Scope**
- `--auth user:pass` → basic auth via `jpillora/cookieauth`.
- `--cert-path` + `--key-path` → TLS (`ListenAndServeTLS`); keep http2 disabled (`TLSNextProto`) per master's velox note.
- `--log` → request logging via `jpillora/requestlog`.
- Compose the handler chain (gzip → auth → log) exactly as master.

**Reference (master):** `server/server.go Run` (auth / TLS / log / gzip chain).

**Done when**
- Auth gates access, TLS serves https, `--log` logs requests.

SPEC: §8, §12

## 25. M4: Packaging - GoReleaser + Docker + README

* status: open
* priority: P3
* type: task
* depends-on: 5,15
* created: 2026-06-16T07:04:44Z

**Objective:** Ship it — single-binary releases + Docker + docs.

**Scope**
- GoReleaser config + Dockerfile (`-p 3000 -v /downloads`).
- CI build pipeline: `gen-types` → `vite build` → `go build` (assets embedded).
- README with screenshot + usage; preserve the `curl … | bash` install parity.

**Reference (master):** `.github/goreleaser.yml`, `.github/Dockerfile`, `README.md`, `.github/workflows/ci.yml`.

**Done when**
- A release build produces cross-platform single binaries + a Docker image that runs the app.

SPEC: §12

## 26. M4: Full parity-checklist verification vs master

* status: closed
* priority: P2
* type: task
* depends-on: 15,16,17,18,19,20,22,23,24
* created: 2026-06-16T07:04:44Z
* updated: 2026-06-16T12:15:52Z

**Objective:** Final gate — verify behaviour is equal to `master`, item by item.

**Scope**
- Check out `master` in a worktree (`git worktree add ../ct-master master`) and run both side-by-side.
- Walk the SPEC §14 parity checklist and reconcile any gap:
  - Omni mode detection (url/magnet/search/upload)
  - Magnet build/parse (infohash, dn, tr[])
  - `Percent` / `DownloadRate` formulas
  - Torrent controls + labels (Start/Stop/Cancel/Remove)
  - Download tree icons, 24h auto-collapse, confirm-to-delete
  - `/download` path-escape guard
  - Footer fields + formatting
  - Engine add → `GotInfo()` → auto-start; delete cleanup
  - `IncomingPort` default `50007`; config persistence

**Reference:** SPEC §14.

**Done when**
- Every checklist item is verified equivalent to `master`.

SPEC: §14
