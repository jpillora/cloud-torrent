# TASKS

a [meads](https://github.com/jpillora/meads) (`md`) managed task log

* created: 2026-06-16T09:52:49Z
* updated: 2026-10-22T12:14:03Z

## 2. M0: Initialize Go module + CLI entry

* status: closed
* priority: P1
* type: task
* created: 2026-06-16T09:57:13Z
* updated: 2026-06-16T12:07:26Z

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
* created: 2026-06-16T09:57:13Z
* updated: 2026-06-16T13:39:25Z

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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
* updated: 2026-06-16T21:28:05Z

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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
* updated: 2026-06-17T08:17:32Z

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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
* updated: 2026-06-17T11:23:33Z

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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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
* created: 2026-06-16T09:57:13Z
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

* status: closed
* priority: P3
* type: task
* depends-on: 5,15
* created: 2026-06-16T09:57:13Z
* updated: 2026-06-16T12:37:56Z

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
* created: 2026-06-16T09:57:13Z
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

## 27. Cancel button not visible

* status: closed
* priority: P2
* type: bug
* created: 2026-06-24T12:05:24Z
* updated: 2026-06-24T12:31:58Z

Cancel button not visible

After adding torrent, pending, not started; it should be cancellable

## 28. Slow torrent refresh speed

* status: closed
* priority: P2
* type: bug
* created: 2026-06-24T12:06:47Z
* updated: 2026-06-24T12:32:10Z

Slow torrent refresh speed

Should poll torrent state once a second while any torrent is downloading

Also deleting a file takes 10 seconds, also slow polling?

## 29. Stopping a torrent does nothing

* status: closed
* priority: P2
* type: bug
* created: 2026-06-24T12:07:58Z
* updated: 2026-06-24T12:32:19Z

Stopping a torrent does nothing

It should stop downloading, and show “remove”

Maybe related to slow refresh

## 30. Enforce resolved download paths before serving files and archives

* status: closed
* priority: P1
* type: bug
* tags: adversarial-review,simplicity,security
* created: 2026-10-22T10:01:28Z
* updated: 2026-10-22T12:14:03Z

Review: code simplicity and layout; root validation, confirmed by Astra reviewer.

Evidence: `server/files.go:88-114` calls lexical `safeJoin` (`server/files.go:132-138`) then `os.Stat` and `os.Open`, which follow symlinks. Directory ZIP generation also receives the resolved-through-symlink path. The listing skips symlink entries, but direct HTTP requests bypass listing. A symlink `downloads/link -> outside-directory` therefore lets `GET /download/link/secret.txt` read outside the documented downloads boundary. This requires a symlink already present under the downloads directory; no remote symlink-creation capability is claimed.

Impact: users with app access can retrieve files outside the configured downloads root when such links exist. Lexical validation does not satisfy the handler's documented confinement contract.

Acceptance criteria:
- Enforce confinement against filesystem resolution for file serving and archives, including symlinked intermediate directories; define deletion behavior consistently.
- Cover a symlink to an external file, an external directory, and an internal target with handler-level regression checks.
- Preserve ordinary downloads and reject root/parent escapes.

### Completion verification (2026-10-22)

Implemented os.Root confinement for file reads, directory archives and deletion. Archives omit links; relative internal links work, external/absolute links and root aliases are rejected. Deleting an internal link retains its target.

- Passed full `go test ./...` and `go test -race ./server -run TestDownload -count=1`. New real-HTTP regression covers external file/directory links, intermediate escapes, internal links, root aliases, ZIP contents and deletion target retention.
- Built/restarted the embedded app at http://100.103.141.66:3001/ and repeated external-link rejection, internal-link serving, ZIP and delete checks over its actual listener using isolated sentinels (cleaned afterward).
- Browser submitted the public Big Buck Bunny torrent URL, observed 276.4 MB / 100%. HTTP subtitle/poster matched disk, poster matched public webseed byte-for-byte; MP4 range returned 206 and the correct 65,536 bytes. Content remains in /mnt/data/downloads/Big Buck Bunny for later engine checks.

## 31. Give engine mutations and synced snapshots a single ownership and locking contract

* status: open
* priority: P1
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:28Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `engine/engine.go:103`
- `engine/engine.go:115`
- `engine/engine.go:131`
- `engine/engine.go:189`
- `server/server.go:88`
- `server/state.go:18`

### Evidence

GetTorrents locks e.mut but returns the original e.ts map and *Torrent/*File objects. newTorrent/upsertTorrent, start/stop/delete and Config read/write bypass that lock in several paths. Server assigns the same map to State and later velox marshals under State.Mutex, which is a different lock. Subsequent polling mutates objects before acquiring State.Mutex. velox v0.4.2 go/sync.go:70-78 confirms marshalling locks only the supplied State.

### Reproduction / validation

Concurrent API adds/deletes/start/stop/configure while the one-second poll and stats/user-triggered Push serialize state. Static synchronization proof; stress race reproduction was not run in this review.

### Impact

Data races, inconsistent JSON snapshots and possible fatal concurrent-map read/write or iteration/write panics in normal multi-request use. Merely adding a lock to the getters does not protect escaped references.

### Acceptance criteria

Choose one engine synchronization boundary for every client/config/cache access and return deep, detached DTO snapshots to the server (including file slices/objects), or demonstrably share a single lock through mutation and serialization. Concurrent mutation plus snapshot/Push tests pass go test -race and previously returned snapshots never mutate.

### Confidence

high; source and dependency locking verified

## 32. Make reconfiguration preserve valid torrent handles and recover from client creation failure

* status: open
* priority: P1
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `engine/engine.go:37`
- `engine/engine.go:49`
- `engine/engine.go:56`
- `engine/engine.go:109`
- `server/server.go:209`

### Evidence

Configure closes the old client before creating the replacement, then GetTorrents only upserts the new client's torrents. It never clears/rebinds e.ts. A successful replacement is initially empty while e.ts still contains old Started flags and closed torrent/file pointers. A failure returns with e.client still referencing the already-closed old client and e.config unchanged. Scratch Go harness confirmed: reconfigure_retains_old_torrent=true started=true start_error=already started.

### Reproduction / validation

Add/start a torrent and save any config change, including AutoStart only. The old row survives with Started=true although its underlying client was closed. Configure to an occupied port for the failure variant.

### Impact

Saving configuration stops existing work while continuing to advertise it as active; failed settings can disable the working engine while reporting only a validation error.

### Acceptance criteria

Implement a bounded configuration transition: apply safe changes in place or re-add/rebind active torrent specs and state when replacing a client; validate/preflight and restore the old working configuration on replacement failure. Tests assert actual piece progress resumes after successful configuration and after a rejected occupied-port change, with no stale handles.

### Confidence

high; source and standalone runtime harness verified

## 33. Implement Stop and Start as a reversible engine transition

* status: open
* priority: P1
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `engine/engine.go:138`
- `engine/engine.go:153`
- `engine/engine.go:159`
- `engine/engine.go:178`
- `frontend/src/components/TorrentItem.vue:72`

### Evidence

StopTorrent asynchronously Drop()s the underlying torrent but retains its wrapper. StartTorrent only flips Started flags and calls DownloadAll on that same dropped handle; it never re-adds the torrent. The dependency documents Drop as removing and closing the torrent. The 'anacrolix has no pause' premise is also stale: v1.59.1 has DisallowDataDownload/AllowDataDownload and per-file priorities.

### Reproduction / validation

Start a loaded torrent, stop it, wait for Drop, then invoke start again. There is also a race where a quick restart marks Started before the queued Drop closes it.

### Impact

The visible Start control reports success but cannot resume downloads, leaving users with a false running state.

### Acceptance criteria

Retain a live handle with explicit pause semantics, or store enough spec to recreate and rebind on restart. Verify Stop -> Start resumes real data transfer and immediate Stop -> Start cannot leave a closed handle marked started. Update misleading no-pause comments.

### Confidence

high; source plus anacrolix API verified, no browser validation claimed

## 34. Make per-file Start schedule the file in anacrolix

* status: open
* priority: P1
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `engine/engine.go:197`
- `engine/engine.go:216`
- `engine/torrent.go:77`
- `frontend/src/components/TorrentItem.vue:36`

### Evidence

StartFile finds the wrapper and sets t.Started/f.Started but never calls its f.f.Download or SetPriority. The underlying file pointer is populated by updateLoaded and is otherwise unused. anacrolix v1.59.1 file.go:172 implements Download by setting normal piece priority. The frontend comment incorrectly promises StartFile is supported.

### Reproduction / validation

Set AutoStart=false, add a loaded multi-file torrent, then start one file. The API succeeds and sets started flags without requesting its pieces; a later StartTorrent fails as already started because StartFile also set t.Started.

### Impact

Selective downloads are a cosmetic state change and can prevent the full-torrent Start path too.

### Acceptance criteria

Schedule the selected underlying file before publishing started state and define whole-torrent versus per-file running semantics. An integration test with AutoStart=false observes progress on the chosen file, no unwanted unrelated file priority, and a subsequent whole-torrent start can schedule remaining files.

### Confidence

high; direct implementation proof

## 35. Do not lose an explicit Start requested before metadata arrives

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `engine/engine.go:92`
- `engine/engine.go:144`
- `engine/engine.go:147`
- `engine/engine.go:153`
- `engine/engine_test.go:97`
- `server/api_test.go:51`

### Evidence

StartTorrent sets Started=true even when Info()==nil, but only DownloadAlls if metadata exists. When metadata later arrives, the auto-start callback calls StartTorrent which rejects the already-started flag, or does nothing if AutoStart=false. Existing tests specifically start unloaded magnets and assert only the flag, thereby accepting the broken transition.

### Reproduction / validation

POST a magnet then immediately POST torrent start before metadata resolves, as the current API test does; let metadata arrive afterward.

### Impact

API users can leave a torrent permanently marked running with no download scheduling. This is distinct from Stop->Start and selective-file scheduling.

### Acceptance criteria

Represent pending start intent and fulfill it on metadata arrival, or reject start-unloaded explicitly without setting Started. Extend the current tests to verify metadata arrival results in real scheduling/progress rather than just a boolean.

### Confidence

high; source control flow verified

## 36. Remove the uninitialized torrent-cache deletion path

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `engine/engine.go:19`
- `engine/engine.go:188`

### Evidence

cacheDir is declared but never assigned or otherwise used. DeleteTorrent executes os.Remove(filepath.Join(e.cacheDir, infohash+".torrent")); with empty cacheDir this resolves relative to process CWD. No code writes a corresponding cache file, so this is stale scaffolding with a live destructive side effect. Scratch Go harness confirmed unrelated_cwd_file_deleted=true in an isolated temporary working directory.

### Reproduction / validation

Place an unrelated <infohash>.torrent file in the server working directory, add that infohash to the engine, then remove the torrent. The unrelated file is unlinked.

### Impact

Torrent removal can unexpectedly delete a user-supplied torrent file outside the configured downloads directory, and the cache field falsely implies persistence exists.

### Acceptance criteria

Remove unused cacheDir and its unlink operation unless actual managed cache persistence is deliberately implemented. Regression check: removing a torrent leaves a matching CWD sentinel untouched, while preserving current downloaded-file retention behavior.

### Confidence

high; source and standalone runtime harness verified

## 37. Treat configuration persistence failure as a failed save

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `server/server.go:74`
- `server/server.go:209`
- `server/server.go:212`
- `server/server.go:213`

### Evidence

reconfigure applies the engine configuration then discards both json.MarshalIndent failure and os.WriteFile's result, publishes Config, and returns nil. Startup also ignores all configuration read errors, not just absence. Save uses direct overwrite rather than atomic replacement.

### Reproduction / validation

Start with a config path whose parent is missing or unwritable, or make the config file unwritable, then save configuration. API returns OK and state reports the new config despite no durable update. A non-ENOENT read error similarly silently loads defaults.

### Impact

Settings presented as successfully saved disappear on restart; unreadable configurations silently reset behavior and partial writes can corrupt the only copy.

### Acceptance criteria

Surface actionable read/write errors, distinguish missing config from unreadable config, and persist through an atomic replacement with an explicit application/persistence failure contract. Tests exercise invalid/unwritable config paths and ensure failed saves are not reported as successful durable saves.

### Confidence

high; direct ignored-error proof

## 38. Honor the advertised DisableEncryption configuration

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `engine/config.go:7`
- `engine/engine.go:42`
- `frontend/src/components/ConfigEditor.vue:51`

### Evidence

DisableEncryption exists in the persisted/synced Config and is automatically rendered as a toggle by the generic config editor, but Configure never reads it. Repository-wide search found no consumer beyond generated types.

### Reproduction / validation

Toggle Disable Encryption and save; the field round-trips in JSON but the underlying torrent client receives exactly the same encryption settings.

### Impact

An exposed operational setting silently does nothing, undermining the generated-config-as-contract design and complicating diagnosis of peer compatibility.

### Acceptance criteria

Map the field to the intended supported anacrolix crypto policy, or explicitly remove/deprecate the setting and stop exposing it as functional. A focused test asserts the two accepted values produce the documented client policy.

### Confidence

high; exhaustive field-use search

## 39. Use generated request payload types at the frontend API boundary

* status: open
* priority: P3
* type: task
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `frontend/src/api.ts:1`
- `frontend/src/api.ts:8`
- `frontend/src/api.ts:28`
- `server/api.go:21`
- `frontend/src/types/server.gen.ts:10`

### Evidence

Server exports URLRequest/MagnetRequest/TorrentRequest/FileRequest specifically for a typed frontend, and tygo emits them, but api.ts imports only Config. Its post accepts arbitrary string and unknown; all other payloads are handwritten object literals unchecked against generated requests. The backend-generated single source of truth promise therefore stops before most request bodies.

### Reproduction / validation

A backend JSON-tag rename or required-field addition regenerates the interface and passes check-types while the corresponding frontend wrapper still sends its old shape; TypeScript has no reference connecting them.

### Impact

Contract drift becomes runtime failures despite an apparently protective generation/CI pipeline; the unused generated payload types add misleading scaffolding.

### Acceptance criteria

Type the small action-to-request mapping or use satisfies GeneratedRequest on each existing wrapper; retain intentional action unions. A deliberate payload-field mismatch must fail vue-tsc. No generic RPC framework or broad module rewrite is necessary.

### Confidence

high; static type boundary verified

## 40. Represent file versus directory explicitly instead of inferring it from nullable children

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `server/files.go:18`
- `server/files.go:60`
- `server/files.go:71`
- `frontend/src/components/FileNode.vue:27`
- `frontend/src/types/server.gen.ts:46`

### Evidence

FsNode has no kind field. list only appends Children for accepted entries, so an empty directory or directory containing only skipped entries leaves Children nil, serialized as null exactly like a regular file. FileNode decides isFile with !props.node.Children. The generated Children array type also omits the real null possibility.

### Reproduction / validation

Create an empty directory under downloads, or one containing only dotfiles. Its node has Children:null and is consumed as a file; directory-specific expand/icon behavior is lost. No browser validation was performed by this reviewer.

### Impact

The backend/frontend data contract cannot distinguish two meaningful filesystem types. Consumers rely on an invalid structural discriminator and the generated type does not describe the wire value.

### Acceptance criteria

Add an explicit directory/file discriminator, or guarantee [] for every directory and null only for files with matching generated types. Verify empty, hidden-only, unreadable and nonempty directories retain directory identity, plus ordinary files retain file behavior.

### Confidence

high; source/JSON contract proof

## 41. Include realtime client bootstrap failure in the retry state machine

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `frontend/src/velox.ts:48`
- `frontend/src/velox.ts:54`
- `frontend/src/velox.ts:61`
- `frontend/src/velox.ts:74`
- `frontend/src/state.ts:36`
- `frontend/src/state.ts:43`

### Evidence

The retry loop is constructed only after await loadClient. A failed /js/velox.js load rejects and permanently caches loading; startSync catches only to console.error and never creates conn. retryNow uses conn?.retryNow, so it cannot recover bootstrap failures or update status with them.

### Reproduction / validation

Fail the first /js/velox.js request, then restore connectivity and request Retry now. State remains at its initial connecting phase and the retry callback has no connection to invoke. Source-verified; no browser validation by this reviewer.

### Impact

The custom reconnect abstraction handles transport errors but excludes the prerequisite that can fail during the same network outage, leaving the application unusable until a full reload.

### Acceptance criteria

Bring script loading into the same externally retryable connection lifecycle; clear rejected loading promises/remove failed script elements and publish an actionable status. A browser test blocks the first script load, restores it, and recovers with Retry now without page reload or duplicate live connections.

### Confidence

high; promise/control-flow proof

## 42. Restrict the embedded scraper to the application's intended search surface

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `server/server.go:146`
- `server/search.go:76`
- `server/search.go:83`
- `/mnt/data/go/pkg/mod/github.com/jpillora/scraper@v0.3.0/scraper/handler.go:83`
- `/mnt/data/go/pkg/mod/github.com/jpillora/scraper@v0.3.0/scraper/handler.go:94`

### Evidence

Directly mounting scraper.Handler exposes its POST root administration behavior at /search/. That route calls LoadConfig itself without updating State.SearchProviders or server.searchConfig. fetchSearchConfig skips if remote bytes still equal the old searchConfig baseline, so it does not repair this alternate mutation path.

### Reproduction / validation

POST {} to /search/ under whatever normal app authentication is configured. Scraper endpoints disappear, but synced provider choices remain. Fetching the unchanged remote config then returns early before reloading.

### Impact

A third-party dependency silently extends the public mutation API and bypasses application state ownership, leaving backend and frontend provider catalogs permanently inconsistent until restart/config changes. This is not an authentication bypass claim.

### Acceptance criteria

Wrap the scraper with an allowlisted read-only query surface, or explicitly route intentional provider mutations through a single server method that atomically updates handler, synced metadata and refresh baseline. Test POST /search/ cannot silently bypass the application contract.

### Confidence

high; dependency route and application refresh logic verified

## 43. Apply search request headers independently of scraper logging

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `server/search.go:19`
- `server/search.go:26`
- `/mnt/data/go/pkg/mod/github.com/jpillora/scraper@v0.3.0/scraper/handler.go:40`
- `/mnt/data/go/pkg/mod/github.com/jpillora/scraper@v0.3.0/scraper/handler.go:50`

### Evidence

startSearch sets Log:false and configures a browser User-Agent on Handler.Headers. In the pinned scraper v0.3.0, copying handler headers to endpoint headers is incorrectly nested under if h.Log. Endpoint.Execute reads endpoint.Headers, so the application's advertised User-Agent is not applied unless supplied separately in individual remote endpoints.

### Reproduction / validation

Load a provider without its own headers using the exact production handler options and query a local HTTP fixture that records User-Agent. It receives the Go default instead of server.userAgent. Dependency code proves the path; fixture execution not performed here.

### Impact

Providers that reject non-browser clients can fail despite an apparently configured compatibility header; guidance blaming provider churn hides an application integration defect.

### Acceptance criteria

Normalize/copy required headers into every loaded endpoint independently of logging, or update/fix the dependency behavior. A local fixture asserts browser User-Agent delivery for both embedded and refreshed provider configurations while logging is disabled.

### Confidence

high; pinned dependency implementation verified

## 44. Implement or remove the no-op --open CLI flag

* status: open
* priority: P3
* type: bug
* tags: adversarial-review,simplicity
* created: 2026-10-22T10:04:29Z

Review: Astra adversarial review — code simplicity and layout.

### Source locations

- `server/server.go:36`
- `main.go:20`
- `README.md:71`

### Evidence

Server.Open is parsed and advertised as opening the default browser, but there is no read of s.Open and no browser-opening implementation anywhere in the source. The server starts identically with or without it.

### Reproduction / validation

Run the binary with --open on a supported desktop; the option is accepted but has no effect. Static field-use verification only; no desktop launch attempted.

### Impact

Public CLI surface contains nonfunctional scaffolding and documentation promises unsupported behavior.

### Acceptance criteria

Implement browser opening after the listener is ready using a reachable host URL, or remove/deprecate the flag and update documentation. Verify the advertised behavior with a stubbed launcher rather than opening browsers in CI.

### Confidence

high; exhaustive field-use search

## 45. Index downloading paths once instead of scanning all torrent files per download row

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,performance
* created: 2026-10-22T10:05:36Z
* updated: 2026-10-22T10:06:54Z

Review: Astra adversarial review — performance.

### Source locations

- `frontend/src/components/FileNode.vue:36-44`
- `frontend/src/components/FileNode.vue:114-121`

### Evidence

Every mounted FileNode creates a computed that walks every started torrent and all its Files. There are up to 999 displayed filesystem entries and no comparable torrent-file cap. Replacing a Files array invalidates every scan.

### Reproduction / validation

Measured actual shipped frontend in Chrome through rais browser MCP, isolated iframe with fake velox target and fetch blocked. 900 visible download rows and one started torrent whose file paths differ from old downloads. Replacing Files: 100 files 56.9 ms; 1,000 files 528.3 ms; 5,000 files 3,604.5 ms, including Vue flush/layout. Fixture removed afterward; live app state unchanged. This is a synthetic worst-case path distribution, not a measurement of current 20-file live dataset.

### Impact

O(download rows × total active torrent files) work blocks the browser main thread on progress updates; a plausible several-thousand-file torrent can freeze the UI for seconds each tick.

### Acceptance criteria

Compute a shared reactive set/map of incomplete started file paths once per state change and use constant-time lookup per row. Repeat the 900-row/5,000-file fixture and show near-linear scaling and no multi-second update stalls.

### Confidence

high

## 46. Cancel metadata waiters when magnets are removed or the engine closes

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,performance
* created: 2026-10-22T10:05:36Z
* updated: 2026-10-22T10:06:54Z

Review: Astra adversarial review — performance.

### Source locations

- `engine/engine.go:89-97`
- `engine/engine.go:183-193`

### Evidence

newTorrent starts a goroutine waiting only on GotInfo. Upstream torrent v1.59.1 explicitly tests that GotInfo remains open after Drop before metadata (client_test.go:379-394); Closed is a separate signal. Every duplicate add also launches another waiter.

### Reproduction / validation

Isolated offline client with DHT, TCP/UTP, webtorrent and trackers disabled. Added then deleted 30 unique metadata-less magnets. Goroutine profile: 0 newTorrent waiters before, exactly 30 afterward, with 0 cached torrents and 0 client torrents. All 30 waiters remained after Engine.Close. Test completed in 0.36 seconds after compilation. Reproducer: /tmp/cloud-torrent-performance-scratch/engine/perf_review_test.go (TestPerfReviewMetadataLeak).

### Impact

Repeated unavailable magnets or duplicate adds accumulate permanent goroutines and retain torrent/view-model objects even after removal or client close.

### Acceptance criteria

Select between captured torrent GotInfo and Closed/engine cancellation; avoid duplicate waiters for an existing torrent. Add/cancel 30 metadata-less magnets and close the engine; goroutine profiles must return to baseline with no newTorrent waiters.

### Confidence

high

## 47. Stop filesystem enumeration when the downloads entry budget is exhausted

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,performance
* created: 2026-10-22T10:05:36Z
* updated: 2026-10-22T10:06:54Z

Review: Astra adversarial review — performance.

### Source locations

- `server/files.go:42-69`
- `server/server.go:83-95`

### Evidence

list checks the 1,000-entry limit only after receiving FileInfo. Parent loops always continue on errFileLimit, so e.Info still stats every remaining sibling. os.ReadDir also materializes and sorts the entire directory before enforcing the budget. Poll runs each second even with no viewers.

### Reproduction / validation

Isolated empty-file directories with 1,000 / 10,000 / 30,000 entries. Exact list-call counts: 1,001 / 10,001 / 30,001 including root; all returned only 999 children. Three warmed scans averaged 2.13 / 25.38 / 73.79 ms (separate full-module run: 2.78 / 29.09 / 88.93 ms). The test directly invokes unmodified list with its count pointer, proving work keeps growing after the output limit. Local filesystem timings do not measure remote storage. Reproducer: /tmp/cloud-torrent-performance-scratch/server/perf_review_test.go (TestPerfReviewListLimit).

### Impact

The advertised cap does not bound disk metadata I/O or allocation. Large or remote download directories slow torrent progress polling and continually scan invisible entries.

### Acceptance criteria

Propagate budget exhaustion out of ancestor loops before calling more Info operations, and use bounded/chunked directory enumeration if the entry limit is intended as a resource bound. Verify filesystem operations remain close to the configured budget for wide directories, while accurately signaling any truncation.

### Confidence

high

## 48. Avoid gzip-compressing already-compressed download payloads

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,performance
* created: 2026-10-22T10:05:36Z
* updated: 2026-10-22T10:06:54Z

Review: Astra adversarial review — performance.

### Source locations

- `server/server.go:147-157`
- `server/files.go:99-114`

### Evidence

The entire handler chain uses gziphandler with default compression and minSize 0, without any MIME filter. gziphandler accepts all content types by default, so browser downloads of MP4, JPEG, ZIP and other already-compressed payloads still run through DEFLATE.

### Reproduction / validation

Isolated unmodified downloadHandler wrapped in the same gziphandler configuration as server.handler. Served an 8 MiB random .mp4 payload to a discard ResponseWriter, three warmed runs: identity mean 0.641 ms / 8,388,608 bytes; gzip mean 3.701 ms / 8,389,273 bytes (5.8x handler CPU-path time, output slightly larger). These are in-process warm-cache timings, not measured network throughput. Reproducer: /tmp/cloud-torrent-performance-scratch/server/perf_review_test.go (TestPerfReviewGzip). Directory ZIP writer uses stored entries, so this finding does not claim its ZIP entries already use DEFLATE.

### Impact

Unproductive compression consumes server CPU and limits large download throughput; multiple clients compete with the torrent engine while gaining little/no bandwidth reduction.

### Acceptance criteria

Skip compression for incompressible downloads or use a deliberate compressible MIME allowlist while retaining immediate flushing for /sync. Verify large MP4/JPEG/ZIP payloads avoid gzip and representative text/JSON/SSE still work; compare download CPU/time.

### Confidence

high

## 49. Bound outbound torrent URL and provider fetches and propagate cancellation

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,performance
* created: 2026-10-22T10:05:36Z
* updated: 2026-10-22T10:06:54Z

Review: Astra adversarial review — performance.

### Source locations

- `server/api.go:81`
- `server/api.go:123-133`
- `server/search.go:62-69`

### Evidence

addTorrentFromURL uses http.Get via DefaultClient, with no overall timeout and no incoming request context. The 10 MiB limit bounds bytes only, not time. fetchSearchConfig also uses http.Get and unbounded io.ReadAll; a stalled body prevents its background refresh loop from continuing.

### Reproduction / validation

A remote server can send headers then stall its body indefinitely; POST /api/url remains pending even if its browser disconnects, since the request context was not passed. Use a local httptest origin to reproduce without adding a torrent. Remote provider-config fetch is likewise unbounded in time and size.

### Impact

Dead providers leave UI operations permanently busy and retain HTTP connections/goroutines after the requester is gone; provider-config refresh can get stuck or allocate an unbounded response.

### Acceptance criteria

Use a configured HTTP client with finite dial/header/body or total timeout, pass API request context through URL fetch, and bound provider-config body size with an explicit oversize error. A stalled local origin must time out; client cancellation must stop upstream reads promptly.

### Confidence

high

## 50. Avoid replacing whole file arrays for single-file progress updates

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,performance
* created: 2026-10-22T10:05:36Z
* updated: 2026-10-22T10:06:54Z

Review: Astra adversarial review — performance.

### Source locations

- `server/state.go:17-18`
- `server/files.go:18-22`
- `engine/torrent.go:18`
- `server/server.go:88-94`

### Evidence

Synced Downloads.Children and Torrent.Files are arrays. velox v0.4.2 computes RFC7396 JSON Merge Patches (go/state.go:234); any changed array element replaces the entire array. A changed file Modified timestamp replaces the corresponding Children array; one progress change replaces all files in that torrent.

### Reproduction / validation

Use a torrent with thousands of files and advance one file Percent/Completed, or a wide directory with one actively written file. Inspect SSE delta: it contains the entire Files/Children array rather than only the changed entry. Code-supported through the actual pinned JSON Merge Patch implementation; no additional serialized-size benchmark was run.

### Impact

Repeated O(all files) payloads, JSON parsing, allocations, and reactive invalidation per one-second poll multiply bandwidth and browser cost across every connected client, including when the torrent file table is collapsed.

### Acceptance criteria

Represent large collections with stable keyed entries or use an element-addressed patch protocol. For a several-thousand-file state, updating one file should transmit proportional to the changed data and preserve unchanged item identities. Retain ordering separately if needed.

### Confidence

high

## 51. Prevent a slow sync client from blocking state broadcasts to other clients

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,performance
* created: 2026-10-22T10:05:36Z
* updated: 2026-10-22T10:06:54Z

Review: Astra adversarial review — performance.

### Source locations

- `server/server.go:59`
- `server/server.go:178-195`
- `go.mod:16`

### Evidence

Pinned velox v0.4.2 go/state.go:250-257 holds connMut and calls c.Version for each client. go/conn.go:60-64 Version takes sendVerMut; send at :178-184 holds that same mutex throughout transport I/O. Therefore the next global Push can block on a previous slow client send. Default WriteTimeout is 30 seconds. Application accepts defaults via SyncHandler and has no per-client isolation override.

### Reproduction / validation

Connect a healthy SSE client and another that stops reading, then generate enough state to block the slow socket. The next global Push waits on slow-client Version while holding the subscriber mutex, delaying subsequent broadcasts/new subscriptions until the write timeout. This is proven lock-path analysis; no socket saturation test was run against the live app.

### Impact

One slow or abandoned browser can delay realtime updates for healthy browsers for up to the default 30-second timeout and block subscription changes.

### Acceptance criteria

Upgrade/fix the transport so reading connection version never waits on network I/O and the global subscriber lock is not held across potentially blocking client operations. An isolated slow-reader test should keep a second healthy client receiving updates promptly and reclaim timed-out send goroutines.

### Confidence

high on lock-path mechanism; impact duration is code-derived, not yet benchmarked

## 52. Discard stale search responses when the query or provider changes

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:15Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/Omni.vue:74`
- `frontend/src/components/Omni.vue:94`
- `frontend/src/components/Omni.vue:123`
- `frontend/src/components/Omni.vue:153`

### Evidence

Browser verified with actual built app and deferred fetch mock: submit query A through 1337x, change to query B/provider abb before resolving A. The table displays STALE RESULT FROM QUERY A under B/abb. Clicking that row requests /search/abb/item?item=%2Fview%2Freview-fixture even though its path came from 1337x. submitSearch appends unconditionally after await; query/provider reset does not invalidate pending requests; detail resolution uses current provider.

### Reproduction / validation

Start a slow search, change query and/or provider while it is pending, then let the old response arrive. Attempt to resolve a result without an embedded magnet/torrent.

### Impact

Results are attributed to the wrong query; item paths go to the wrong provider and fail or resolve an unintended item. An old response also increments the new query's pagination.

### Acceptance criteria

Capture query/provider/page plus a generation ID per request; discard results/errors from superseded generations. Bind result resolution to its originating provider. Switching query/provider during a delayed response must leave no stale rows and the new search must begin at page 1.

### Confidence

High; browser reproduced with deterministic delayed responses; no server mutation.

## 53. Guard keyboard submissions against in-flight Omni requests

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:15Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/Omni.vue:74`
- `frontend/src/components/Omni.vue:159`
- `frontend/src/components/Omni.vue:204`
- `frontend/src/components/Omni.vue:249`

### Evidence

Browser verified: three Enter keyup events while a search response was held produced three identical /search/abb?query=repeated+enter&page=1 requests while the Search button was disabled. The input calls submit directly, bypassing AsyncButton and neither submit nor submitSearch checks busy/searching.

### Reproduction / validation

Start a slow search or magnet/URL submission with Enter, then press Enter again before it completes.

### Impact

Duplicate searches append duplicate rows and advance pagination multiple times; URL/magnet submission can also issue redundant mutations despite disabled visible controls.

### Acceptance criteria

Enforce an in-flight guard inside the submission functions shared by keyboard and buttons. Repeated Enter during one operation must cause one request and one page advancement; a new operation should be permitted when it settles.

### Confidence

High; repeated search requests verified in browser; equivalent URL/magnet guard omission verified in source.

## 54. Make the torrent upload control keyboard operable

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:15Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/Omni.vue:207`
- `frontend/src/styles/main.css:940`

### Evidence

Live browser DOM shows the only upload trigger is span[role=button] with tabIndex -1, no key handlers, and its associated file input is display:none. Calling focus() leaves focus elsewhere.

### Reproduction / validation

Navigate the app with Tab only and try to select a local .torrent file.

### Impact

Keyboard-only users cannot reach the file picker and cannot use the upload workflow.

### Acceptance criteria

Use a native focusable button with a clear accessible name for upload. Tab must reach it and Enter/Space must open the file picker, while the icon and existing pointer behavior remain usable.

### Confidence

High; live DOM and focus behavior verified.

## 55. Make download folders expandable with the keyboard

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:15Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/FileNode.vue:114`
- `frontend/src/components/FileNode.vue:118`

### Evidence

Browser fixture rendered a real FileNode for an old collapsed folder. Its icon and folder name are spans with click handlers, tabIndex -1 and no keyboard handlers; focus() failed. No other folder expand control exists.

### Reproduction / validation

Open Downloads containing a folder older than 24 hours and use only Tab/Enter/Space to reach its children.

### Impact

Keyboard-only users cannot open collapsed directories or access any files within them.

### Acceptance criteria

Provide a native button or equivalent keyboard-operable disclosure control, an accessible folder name, and aria-expanded reflecting its state. Tab/Enter/Space must expand and collapse a folder and allow navigation to child file links.

### Confidence

High; actual built app browser fixture and source verified.

## 56. Reveal file action buttons when they receive keyboard focus

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:15Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/styles/main.css:703`
- `frontend/src/styles/main.css:722`

### Evidence

At a 1000px iframe viewport, focusing the real Downloads Delete button yielded document.activeElement===button but computed opacity '0' after transitions settled. Visibility rules cover hover, active preview, confirm and deleting, but not :focus/:focus-visible/:focus-within.

### Reproduction / validation

On desktop, move the pointer away from Downloads and Tab from a file link onto Preview/Delete.

### Impact

Focus enters an invisible interactive control, including a destructive action, leaving keyboard users unable to identify the selected action.

### Acceptance criteria

Reveal row actions whenever the row contains keyboard focus and keep a visible focus indicator. At desktop widths, tabbing through Preview/Delete/Confirm must always display the focused control without needing pointer hover.

### Confidence

High; browser computed opacity and focus verified.

## 57. Keep the magnet editor synchronized with direct Omni edits

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:16Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/MagnetEditor.vue:11`
- `frontend/src/components/MagnetEditor.vue:24`
- `frontend/src/components/Omni.vue:197`

### Evidence

Browser verified: open an empty magnet editor, paste magnet:?xt=urn:btih:1111111111111111111111111111111111111111&dn=First into Omni. The Info Hash field stays empty. Editing Name then replaces Omni with magnet:?xt=urn:btih:&dn=Edited%20name, losing the pasted hash. The editor parses modelValue only once at creation.

### Reproduction / validation

Leave the magnet editor open, paste a new magnet into Omni, then edit a name or tracker in the editor.

### Impact

Stale editor fields silently overwrite the new magnet, potentially replacing its infohash and trackers with values from a different torrent or an invalid empty hash.

### Acceptance criteria

Synchronize editor fields when modelValue changes externally without causing edit loops. Pasting a second magnet with the editor open must update all fields; a subsequent name/tracker edit must preserve the second magnet's hash and intended metadata.

### Confidence

High; live browser reproduced without submitting any magnet.

## 58. Keep torrent file tables within narrow viewports for long filenames

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:16Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/TorrentItem.vue:106`
- `frontend/src/styles/main.css:628`
- `frontend/src/styles/main.css:646`

### Evidence

Actual built app in a 390px browser iframe with a valid approximately 180-character unbroken file basename produced document scrollWidth 1535px and a file table 1501.6px wide. Stop controls were positioned at x=1475px. The torrent file name cell has no breaking/min-width strategy; Downloads and search names have separate word-break rules.

### Reproduction / validation

Expand Files on a torrent whose filename contains a long unbroken release name, at a phone-sized viewport.

### Impact

The entire page acquires horizontal overflow and per-file actions move far offscreen, making file management difficult on mobile.

### Acceptance criteria

Long valid basenames must wrap or truncate with an accessible way to inspect them; file actions must remain reachable within the viewport. Verify 320px and 390px widths with a 180-character basename without whole-page horizontal scrolling.

### Confidence

High; browser layout measurements verified.

## 59. Show enough relative path to distinguish duplicate torrent filenames

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:16Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/TorrentItem.vue:47`
- `frontend/src/components/TorrentItem.vue:108`

### Evidence

Browser fixture containing one/nested/duplicate.txt and two/nested/duplicate.txt rendered both rows as identical 'duplicate.txt 50%' with identical size and Stop labels. filename() deliberately strips the directory and the template provides no path tooltip or directory grouping.

### Reproduction / validation

Expand Files for a torrent containing identically named files in different directories, such as episodes with repeated subtitle or metadata basenames.

### Impact

Users cannot identify which file a Start/Stop action applies to, particularly when size and progress are equal.

### Acceptance criteria

Expose distinct relative paths or directory grouping in the file list, including an accessible full path. Duplicate basenames must be distinguishable before acting on them on both desktop and mobile.

### Confidence

High; browser fixture rendered both identical rows.

## 60. Do not let the global media Space shortcut override focused controls

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:16Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/App.vue:64`
- `frontend/src/App.vue:69`
- `frontend/src/App.vue:75`

### Evidence

Browser fixture opened an audio preview and focused its Preview button. Dispatching a cancelable Space keydown invoked the media play handler once and set defaultPrevented=true while leaving the preview open. The guard excludes inputs/textareas/contenteditable only, so buttons, selects, and native media controls are intercepted.

### Reproduction / validation

Open an inline audio/video preview, leave it visible, focus a button such as Hide preview, and press Space.

### Impact

Space performs the global play/pause action instead of activating the focused control. Native player controls and other standard keyboard interactions can also be hijacked.

### Acceptance criteria

Respect Space on interactive controls, including buttons, selects, links with button behavior and media controls. The global shortcut should run only when focus is on a noninteractive page area; verified focused button activation must not toggle unrelated media.

### Confidence

High for handler interception, demonstrated with a cancelable browser KeyboardEvent and a play spy; native trusted key default activation was not available through the exposed MCP.

## 61. Isolate keyboard focus while the connection overlay is shown

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:16Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/App.vue:120`
- `frontend/src/App.vue:130`

### Evidence

Triggering the actual reconnect overlay through the fixture connection callback left the background Start button focusable; activeElement became that button. The overlay has no dialog role/aria-modal/focus management and no underlying content is inert.

### Reproduction / validation

Focus a control, lose the realtime connection, then continue navigating with Tab or activating the focused button.

### Impact

The full-screen overlay visually blocks app actions while keyboard users can still focus and trigger hidden controls against stale state. Retry now is not automatically reached or announced as a modal state.

### Acceptance criteria

When connection UI blocks interaction, move focus to an appropriately labelled status/dialog and Retry now, prevent background keyboard actions, and restore prior focus after reconnect. If the design becomes nonmodal instead, make its interaction rules consistent for pointer and keyboard users.

### Confidence

High; browser focusability and modal DOM attributes verified.

## 62. Surface failed actions at the user's current location and announce errors

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:16Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/FileNode.vue:103`
- `frontend/src/components/AsyncButton.vue:20`
- `frontend/src/App.vue:99`

### Evidence

Browser fixture with 40 files and a mocked DELETE 500: deleting the last file placed the global error banner at top=-1930.9px while scrollY remained 2004.5. The banner had neither role nor aria-live and focus remained on body; the failed row simply returned to normal.

### Reproduction / validation

Scroll to an item near the bottom of a long Downloads/torrent list and perform an action that fails, such as permission-denied deletion.

### Impact

Users see no nearby failure explanation and assistive technology receives no live announcement, so an unsuccessful destructive or transfer action can appear to have been ignored.

### Acceptance criteria

Provide accessible error announcements and visible feedback where the action occurred, or a persistent visible notification without disruptive focus loss. A failed action at the bottom of a long list must be clearly identified without manually scrolling to the top.

### Confidence

High; actual app browser fixture, mocked failure, scroll position and DOM semantics verified; no files deleted.

## 63. Apply consistent readable input sizing in the magnet editor

* status: open
* priority: P3
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:16Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/MagnetEditor.vue:45`
- `frontend/src/components/MagnetEditor.vue:49`
- `frontend/src/components/MagnetEditor.vue:53`
- `frontend/src/styles/main.css:149`
- `frontend/src/styles/main.css:460`

### Evidence

Live browser computed all three magnet-editor text fields at 13.3333px font size and 21.5px height. They omit type='text', so the common 1rem padded input selector does not match; the editor-specific rule only sets width/margin.

### Reproduction / validation

Open the magnet editor on a phone or compare it with Configuration fields.

### Impact

Magnet hashes and tracker URLs are shown in small browser-default controls with much smaller touch/read targets than the application's other inputs; the documented >=16px mobile input convention is not met.

### Acceptance criteria

Ensure all magnet editor text inputs receive the standard text-input styles and at least 16px computed font size on mobile. Name/hash/tracker inputs should match the intended field padding, border and focus treatment.

### Confidence

High for computed styling; iOS focus-zoom itself was not tested.

## 64. Protect active torrent files from deletion through their parent folders

* status: open
* priority: P1
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:06:16Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/FileNode.vue:35`
- `frontend/src/components/FileNode.vue:40`
- `frontend/src/components/FileNode.vue:130`
- `server/files.go:115`

### Evidence

Browser fixture with started torrent file active-folder/active.bin at 50% showed parent Delete=true while child Delete=false and child download link=false. downloading checks exact f.Path === path only, so directories containing active files are never protected. The server DELETE handler calls os.RemoveAll(full) without checking active torrent descendants.

### Reproduction / validation

During a multi-file torrent download, use the parent folder's Delete then Confirm controls. This review verified control availability and server code only and did not execute a real deletion.

### Impact

The UI's protection for actively downloading files can be bypassed by deleting their parent, removing in-use content and disrupting active downloads.

### Acceptance criteria

Treat directories containing active incomplete files as active and prevent unintended recursive removal. Enforce the policy on the server as well as in UI, or implement an explicit coordinated stop/delete flow. Ancestors of an active file must not expose a normal destructive operation that silently removes it.

### Confidence

High; browser verified parent/child inconsistency and server removal path inspected; destructive end-to-end execution intentionally not performed.

## 65. Do not offer a per-file Stop action that the engine always rejects

* status: open
* priority: P2
* type: bug
* tags: adversarial-review,ui-ux
* created: 2026-10-22T10:07:02Z

Review: Astra adversarial review — UI/UX.

### Source locations

- `frontend/src/components/TorrentItem.vue:36`
- `frontend/src/components/TorrentItem.vue:130`
- `engine/engine.go:221`

### Evidence

The actual built app browser fixture displayed enabled Stop buttons for started files. TorrentItem invokes api.file('stop', ...); engine.StopFile unconditionally returns fmt.Errorf("unsupported"). The component comment explicitly acknowledges this mismatch. No real torrent was changed during review.

### Reproduction / validation

Expand Files for a started torrent and click a file's Stop button.

### Impact

The UI promises selective pause but the operation necessarily fails, leaving the download running and forcing users to discover the unsupported capability through an error.

### Acceptance criteria

Either support per-file pause end-to-end, or suppress/disable the unsupported action with a clear explanation and a usable whole-torrent Stop alternative. Do not present an enabled Stop button whose only result is unsupported.

### Confidence

High; button rendering verified in browser and unconditional backend rejection verified in source. Real API call omitted to avoid changing user torrents.
