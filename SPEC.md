# Cloud Torrent — AI Rewrite (`airewrite`)

> A ground-up rewrite of [cloud-torrent](https://github.com/jpillora/cloud-torrent) that preserves the
> **exact user experience** while replacing the implementation with a modern, type-safe stack.

Cloud Torrent is a self-hosted remote torrent client: you add torrents from a browser, they download
to the server's disk as sets of files, and those files are then retrievable or streamable over HTTP.

This document is the source of truth for the rewrite. The original implementation lives on the
`master` branch and is the **functional reference** — when behaviour is ambiguous, match `master`.

---

## 1. Goals & Principles

1. **Identical UX.** Same five sections, same flows, same visual language. A user familiar with the
   old app must feel zero difference. The first deliverable is the core loop: **choose a torrent in
   the Omni bar → watch progress in the Torrent engine → download the resulting files.**
2. **Ground-up implementation.** No code is copied from `master`. AngularJS + Semantic UI + hand-rolled
   JS are replaced by Vue 3 + Vite + TypeScript. The Go backend is re-implemented cleanly.
3. **One canonical data model.** Application state is defined **once** as Go structs and **generated**
   into TypeScript. The frontend never hand-writes a shape that exists in Go.
4. **Realtime by default.** Server → browser state is synced in realtime with
   [`jpillora/velox`](https://github.com/jpillora/velox) (same library as `master`).
5. **Single binary.** The built frontend is embedded into the Go binary. `go build` produces one
   cross-platform, dependency-free executable.
6. **Verified against reality.** End-to-end correctness is proven by downloading real, legal torrents
   from <https://webtorrent.io/free-torrents> (they carry WebSeeds, so they complete reliably).

### Non-goals (for the initial rewrite)

- The "0.9 / general cloud-transfer engine" vision (remote backends, transforms, RSS). Out of scope.
- Changing the torrent engine. We stay on [`anacrolix/torrent`](https://github.com/anacrolix/torrent).
- Auth/TLS feature expansion — port the existing basic-auth + TLS options as-is, no more.

---

## 2. Tech Stack

| Layer            | Choice                                                                 |
|------------------|------------------------------------------------------------------------|
| Language (server)| Go (match `go.mod` toolchain on `master`)                              |
| Torrent engine   | `github.com/anacrolix/torrent` (`v1.59.x`)                             |
| Realtime sync    | `github.com/jpillora/velox` (`v0.4.2`) — same API as `master`         |
| CLI / flags      | `github.com/jpillora/opts`                                             |
| Search scraper   | `github.com/jpillora/scraper`                                          |
| System stats     | `github.com/shirou/gopsutil/v3`                                       |
| Zip streaming    | `github.com/jpillora/archive`                                          |
| Language (web)   | TypeScript                                                             |
| UI framework     | Vue 3 (`<script setup>`, Composition API)                             |
| Build tool       | Vite                                                                   |
| Go → TS types    | [`tygo`](https://github.com/gzuidhof/tygo)                            |
| Date formatting  | `dayjs` (replaces `moment`)                                           |
| Asset embedding  | Go `embed.FS`                                                         |
| E2E tests        | Playwright (UI) + Go `httptest` (API/engine)                         |

---

## 3. Repository Layout

```
cloud-torrent/                  (orphan branch: airewrite)
├── SPEC.md                     ← this file
├── go.mod / go.sum
├── main.go                     ← CLI entry (jpillora/opts → server.Server)
├── dev.ts                      ← Take command entrypoint
├── .dev/                       ← build commands, Docker/release/type-gen configs
│
├── engine/                     ← torrent engine (wraps anacrolix/torrent)
│   ├── engine.go               ← Engine: configure, add, start/stop/delete, poll
│   ├── config.go               ← Config struct
│   └── torrent.go              ← Torrent + File view-models (the synced shapes)
│
├── server/                     ← HTTP server
│   ├── server.go               ← routing, lifecycle, velox wiring, poll loops
│   ├── state.go                ← root State struct (the velox-synced object)
│   ├── api.go                  ← typed action API (POST /api/*)
│   ├── files.go                ← file tree, /download (GET file/zip, DELETE)
│   ├── search.go               ← scraper search providers + auto-refresh
│   └── stats.go                ← system stats (gopsutil) + Stats struct
│
├── static/
│   └── embed.go                ← go:embed frontend/dist  (prod asset serving)
│
└── frontend/
    ├── index.html
    ├── package.json
    ├── vite.config.ts          ← dev proxy → Go backend; build → dist/
    ├── tsconfig.json
    └── src/
        ├── main.ts             ← mount App
        ├── App.vue             ← layout: the 5 sections + drop cover + connect overlay
        ├── state.ts            ← reactive<State> singleton, fed by velox
        ├── velox.ts            ← typed velox client wrapper (composable)
        ├── api.ts              ← typed POST /api/* client
        ├── format.ts           ← bytes(), round(), ago() helpers
        ├── types/
        │   ├── generated.ts    ← GENERATED from Go (do not edit)
        │   └── velox.d.ts      ← types for the served velox.js client
        ├── components/
        │   ├── Header.vue      ← §A titlebar + settings editor
        │   ├── Omni.vue        ← §B omni bar (search / magnet / url / file)
        │   ├── MagnetEditor.vue
        │   ├── Torrents.vue    ← §C torrent engine list
        │   ├── TorrentItem.vue
        │   ├── Downloads.vue   ← §D files / downloads tree
        │   ├── FileNode.vue    ← recursive tree node (self-referential)
        │   └── Footer.vue      ← §E version + resources
        └── styles/             ← ported, modernised CSS (scoped per component)
```

---

## 4. Architecture Overview

```
┌──────────────────────────── Browser (Vue 3 + TS) ────────────────────────────┐
│  App.vue ─ Header · Omni · Torrents · Downloads · Footer                      │
│      │ reads (reactive)            │ writes (typed)                            │
│  state.ts ◄── velox.ts (/sync, /js/velox.js)        api.ts ──► POST /api/*     │
└──────────┼────────────────────────────────────────────────────┼──────────────┘
           │ realtime JSON state + deltas (SSE/WS)               │ actions
┌──────────┼────────────────────────────────────────────────────┼──────────────┐
│  server  ▼                                                      ▼   (Go)       │
│  State{ Config, Torrents, Downloads, SearchProviders, Users, Stats }          │
│     ▲ velox.Push() every 1s            ▲ engine ops      ▲ scraper   ▲ gopsutil│
│  poll loop ──────────────► engine (anacrolix/torrent) ──► disk ./downloads     │
└───────────────────────────────────────────────────────────────────────────────┘
```

- The **root `State`** struct is the single object velox keeps in sync with the browser.
- A 1-second **poll loop** copies engine torrents + the download file tree into `State`, then
  `Push()`es. A 5-second loop refreshes system `Stats`. (Mirrors `master`'s `server.go` loops.)
- The browser **reads** everything from the synced `State` and **writes** only via `POST /api/*`.

---

## 5. Data Model (Go → the canonical shapes)

These structs are the contract. They are defined in Go and generated into
`frontend/src/types/generated.ts` via `tygo`. **Field names are the JSON keys** unless a `json:` tag
says otherwise (the frontend reads `state.Torrents`, `t.InfoHash`, etc., exactly as on `master`).

```go
// engine/config.go
type Config struct {
    AutoStart         bool
    DisableEncryption bool
    DownloadDirectory string
    EnableUpload      bool
    EnableSeeding     bool
    IncomingPort      int
}

// engine/torrent.go — view-models projected from anacrolix/torrent each poll
type Torrent struct {
    InfoHash     string
    Name         string
    Loaded       bool      // Info() has arrived
    Downloaded   int64
    Size         int64
    Files        []*File
    Started      bool
    Dropped      bool
    Percent      float32   // 0..100, 2 dp
    DownloadRate float32   // bytes/sec
    // unexported: t *torrent.Torrent, updatedAt time.Time (not synced)
}

type File struct {
    Path      string
    Size      int64
    Chunks    int
    Completed int
    Started   bool
    Percent   float32
}

// server/files.go
type FsNode struct {
    Name     string
    Size     int64
    Modified time.Time      // tygo: → string (RFC3339)
    Children []*FsNode
}

// server/stats.go
type Stats struct {
    Title   string
    Version string
    Runtime string            // Go version, "go" prefix stripped
    Uptime  time.Time         // → string
    System  SystemStats
}
type SystemStats struct {
    Set         bool    `json:"set"`
    CPU         float64 `json:"cpu"`
    DiskUsed    int64   `json:"diskUsed"`
    DiskTotal   int64   `json:"diskTotal"`
    MemoryUsed  int64   `json:"memoryUsed"`
    MemoryTotal int64   `json:"memoryTotal"`
    GoMemory    int64   `json:"goMemory"`
    GoRoutines  int     `json:"goRoutines"`
}

// server/state.go — THE velox-synced root object
type State struct {
    velox.State `json:"-"`   // embedded; skipped by tygo + json
    sync.Mutex  `json:"-"`
    Config          engine.Config
    SearchProviders scraper.Config           // map[providerID]provider
    Downloads       *FsNode
    Torrents        map[string]*engine.Torrent  // keyed by infohash
    Users           map[string]string           // velox connID → remote addr
    Stats           Stats
}
```

> **`Percent` semantics** (match `master`): `percent(n,total) = trunc(10000*n/total)/100` → a float in
> `0..100` with 2 decimal places; `0` when total is `0`.
> **`DownloadRate`**: bytes downloaded since last poll ÷ elapsed seconds.

---

## 6. Type Generation (Go → TypeScript)

The frontend's `src/types/generated.ts` is produced from the Go packages — **never hand-edited**.

```yaml
# .dev/tygo.yaml
packages:
  - path: "github.com/jpillora/cloud-torrent/engine"
    output_path: "frontend/src/types/generated.ts"
  - path: "github.com/jpillora/cloud-torrent/server"
    output_path: "frontend/src/types/generated.ts"   # appended
    type_mappings:
      time.Time: "string"
```

Rules:
- Embedded `velox.State` and `sync.Mutex` are tagged `json:"-"` so tygo and `encoding/json` both skip
  them; the generated `State` interface contains only data fields.
- `time.Time` maps to `string` (RFC3339); the frontend parses with `dayjs`.
- **API request/response bodies (§8) are also Go structs** and are generated the same way, so the
  typed `api.ts` client and the server share one definition.
- Regenerate via `./dev.ts gen-types` using `.dev/tygo.yaml`.
  CI fails if `generated.ts` is stale (run gen, `git diff --exit-code`).

---

## 7. Realtime Sync (velox)

velox keeps the Go `State` and a browser JS object identical in realtime. We use it on **both** ends.

**Server** (as on `master`):
```go
s.state.Push()                          // throttled broadcast of current State (full or JSON delta)
velox.Sync(&s.state, w, r)              // GET /sync  → upgrades to SSE or WebSocket, returns *Conn
velox.JS.ServeHTTP(w, r)                // GET /js/velox.js → serves the client library
conn.ID(); conn.Wait()                  // track connection in State.Users; block until disconnect
```
On connect, add `conn.ID()→RemoteAddr` to `State.Users` and `Push()`; on disconnect, delete and
`Push()`. (This drives the footer's "N users connected".)

**Browser** — `velox.ts` wraps the served client with types and feeds the reactive store:
```ts
// state.ts
export const state = reactive<Partial<State>>({})

// velox.ts (conceptual)
const v = velox("/sync", state)         // mutates `state` in place with synced data
v.onupdate = () => { /* reactivity is automatic; optional hook */ }
v.onchange = (connected: boolean) => { connection.connected = connected; if (connected) connection.hasConnected = true }
```
- `connected` / `hasConnected` drive the connect overlay (§9.0) and the header lightning icon.
- The velox client (`/js/velox.js`) is served by Go and loaded by the app; `types/velox.d.ts` provides
  its TypeScript signatures. (Do **not** reimplement the velox protocol — use the official client.)

---

## 8. HTTP API

The wire format is internal (not user-visible), so it is **cleaned up** from `master`'s colon-delimited
strings into typed JSON. Behaviour is identical. All mutations are `POST /api/<action>`; on success the
server `Push()`es new state, so the UI updates via velox rather than via the response body.

| Method | Path                     | Body (Go struct → generated TS)         | Effect (ref: `master`)                          |
|--------|--------------------------|------------------------------------------|-------------------------------------------------|
| POST   | `/api/configure`         | `Config`                                 | reconfigure engine, persist `cloud-torrent.json`|
| POST   | `/api/url`               | `{ url: string }`                        | fetch remote `.torrent`, add it                 |
| POST   | `/api/torrentfile`       | raw bytes (`.torrent`)                   | parse metainfo → add torrent                    |
| POST   | `/api/magnet`            | `{ uri: string }`                        | `engine.NewMagnet`                              |
| POST   | `/api/torrent`           | `{ action, infohash }` action∈start/stop/delete | start/stop/delete a torrent              |
| POST   | `/api/file`              | `{ action, infohash, path }` action∈start/stop  | start/stop a single file (stop = TODO/master parity) |
| GET    | `/sync`                  | —                                        | velox realtime connection                       |
| GET    | `/js/velox.js`           | —                                        | velox client library                            |
| GET    | `/search/:provider`      | `?query=&page=`                          | scraper search → `[]Result` JSON                |
| GET    | `/search/:provider/item` | `?item=`                                 | scrape a result's detail page (magnet/infohash) |
| GET    | `/download/*path`        | —                                        | stream a file, or a **zip** of a directory      |
| DELETE | `/download/*path`        | —                                        | `os.RemoveAll` within the download dir          |

**Engine operations** (mirror `engine.go`):
- Add (magnet/url/file) → on `GotInfo()`, auto-start (respect `Config.AutoStart`).
- Start → `t.DownloadAll()`, mark torrent + files Started.
- Stop → `t.Drop()` (anacrolix has no pause), mark stopped.
- Delete → drop + remove from map + delete cached `.torrent`.
- `IncomingPort` defaults to `50007` if out of range.

**Security:** `/download` must reject paths escaping `DownloadDirectory` (prefix check, as `master`).
Optional basic auth (`--auth user:pass`, via `cookieauth`) and TLS (`--cert`/`--key`) wrap the handler.

---

## 9. The Five Sections (UX spec)

The layout is a single column: **Header → Omni → Torrents → Downloads → Footer**, with a full-page
drop cover and a disconnect overlay on top. Each maps to original code for parity verification.

### §0 Global shell (`App.vue`)
- **Connect overlay**: when `!connected`, cover the page with a centered masthead showing
  `hasConnected ? "Disconnected" : "Connecting"` + a red lightning icon. *(ref: `index.html` connect-warning)*
- **Drop cover**: dragging `.torrent` files anywhere shows a "Drop torrent files here" cover; on drop,
  read each as bytes → `api.torrentfile`. *(ref: `utils.js` `ondropfile`, `run.js` `uploadTorrent`)*
- **Global error banner**: dismissible error message bound to a global `err`. *(ref: `index.html`)*
- **Spacebar media control**: pressing space (when not typing) toggles play/pause on the first
  in-viewport `<video>/<audio>`. *(ref: `run.js` keydown handler)*

### §A Header — title bar + settings  *(ref: `index.html .title`, `config-controller.js`, `config.html`)*
- Left: logo icon (cloud, swaps to GitHub mark on hover) linking to the repo + `Stats.Title`.
- Right status icons: request spinner (visible while an API call is in flight), **settings** (server
  icon — toggles the Config editor; green when open), **magnet** (toggles the Omni magnet editor;
  green when open), **connection** lightning (green connected / red disconnected).
- **Config editor** (collapsible form): iterate `state.Config` entries; render a **toggle** for
  booleans and a labelled text/number input otherwise (keys shown spaced: `DownloadDirectory` →
  "Download Directory"). **Save** → `api.configure(state.Config)`; **Cancel** closes.
- **Appearance** (above server configuration): a labelled System / Light / Dark theme selector.
  Defaults to System, follows live OS appearance changes, and applies immediately. Overrides
  persist only in the current browser and synchronize between its tabs; they do not alter server
  configuration or require Save. Apply the initial palette before paint to avoid a light flash.

### §B Omni bar — the universal input  *(ref: `omni-controller.js`, `omni.html`)*
One text input: *"Enter search query, magnet URI, torrent URL or drop a torrent file here"*. The input
is parsed live into a **mode**, and the trailing icon + action buttons change accordingly:

| Detected input            | Mode      | Icon   | Action                                                        |
|---------------------------|-----------|--------|---------------------------------------------------------------|
| `^https?://`              | torrent   | magnet | **Start Torrent** → `api.url(input)`                          |
| `^magnet:?…`              | magnet    | magnet | **Load Magnet** → `api.magnet(input)`; **Edit** → magnet editor |
| non-empty (anything else) | search    | search | provider `<select>` + **Search** / **Load more** → `/search`  |
| empty                     | upload    | upload | click/drag a `.torrent` → `api.torrentfile`                   |

- **Magnet editor** (`MagnetEditor.vue`): Name, Info Hash, and a growing list of Tracker fields.
  Editing any field rebuilds the magnet URI live (`xt=urn:btih:…&dn=…&tr=…`) and validates the
  infohash (`^[A-Za-z0-9]+$`), surfacing `omnierr` on failure. Parsing a pasted magnet populates these
  fields (name, infohash, trackers). *(ref: `parseMagnet`, `magnetURI`, `parseMagnetString`)*
- **Search results** table: Name (links to the provider page), Size, Seeds/Peers, and a green download
  icon. Clicking download: if the result already has a `magnet`/`torrent`, submit it directly;
  otherwise fetch `/search/:provider/item?item=<path>` and resolve a magnet from
  `magnet` | `torrent` | `infohash + tracker` list. Pagination via "Load more" (increment `page`).
  *(ref: `submitSearch`, `submitSearchItem`)*
- Remember last query + provider in `localStorage` (`tcOmni`, `tcProvider`). *(ref: `storage`)*

### §C Torrent engine — active torrents  *(ref: `torrents-controller.js`, `torrents.html`)*
- Header "Torrents" + count (`N torrent(s)`). Empty → "Add torrents above".
- One card per `state.Torrents[hash]`:
  - While `!Loaded`: a "Loading" dimmer (metadata still being fetched).
  - **Name** (links to the `.zip` of the torrent once ready), `#InfoHash`, and a blue **progress bar**
    (`Percent`, animated while `0 < Percent < 100`).
  - **Controls**: **Files** toggle · **Start** (green, when not started → `torrent:start`) ·
    **Stop** (red, when started → `torrent:stop`) · **Cancel/Remove** (→ `torrent:delete`; label
    "Cancel" while `!Loaded`, "Remove" once loaded).
  - **Status line** when started: `Downloaded / Size — Percent% — Rate/s` (zeros are muted).
  - **Files table** (when Files toggled & loaded): per file → name, live percent + mini progress bar
    while downloading, size, green check at 100%; footer shows file count + total size.

### §D Files / Downloads — what's on disk  *(ref: `downloads-controller.js`, `downloads.html`, `download-tree.html`)*
- Header "Downloads" + free-disk (`diskTotal − diskUsed`). Empty → "Download files above".
- A **recursive tree** of `state.Downloads` (`FsNode`), rendered by `FileNode.vue` (self-referential):
  - **Icon** by state/type: spinner while that file is actively downloading; otherwise folder
    (open/closed) or a file icon specialised for audio / image / video by extension.
  - Folders are collapsible; auto-collapsed when `Modified` is > 24h ago.
  - Files are links to `download/<path>`. Each node shows `Size` + "updated <ago>".
  - **Controls**: trash → **confirm** (3s window) → `DELETE download/<path>`; for media files, a
    preview toggle that inlines an `<audio>/<img>/<video>` pointing at `download/<path>`.
  - Cross-reference each file path against `state.Torrents[*].Files` to know if it's mid-download.

### §E Footer — version + resources  *(ref: `index.html footer`)*
- Left: `github.com/jpillora/cloud-torrent version <Stats.Version>` · (`N users connected` when > 1) ·
  `Go <Stats.Runtime>`.
- Right (when `Stats.System.Set`): `goroutines`, `gomem` (bytes), `cpu %`, `mem %`, `disk %`, and
  `up <Uptime ago>`.

**Shared formatters** (`format.ts`, ref: `utils.js`): `bytes(n)` (metric-prefixed, e.g. `1.5 GB`),
`round(n)` (1 dp), `ago(t)` (relative time via dayjs).

---

## 10. Core End-to-End Flow (the first milestone)

The flow the rewrite must nail first, exactly as the brief states:

1. **Choose** — paste a magnet (or torrent URL) into the **Omni bar** → **Load Magnet** →
   `POST /api/magnet`.
2. **Track** — the torrent appears in the **Torrent engine**; once metadata loads it auto-starts and
   its progress bar, byte counts, and rate update in realtime (via velox).
3. **Download** — completed files appear in **Downloads**; clicking a file streams it from
   `GET /download/<path>` (a folder streams as a zip); media files preview inline.

---

## 11. Testing Strategy

**Reference oracle:** the `master` branch. For each behaviour, compare against the cited
`file:function`. Keep `master` checked out elsewhere (`git worktree add ../ct-master master`) for
side-by-side runs.

**Fixtures:** routine CI uses authored CC0 bytes and a pinned torrent infohash in
`.dev/e2e/fixture.ts`, served by a throttled loopback WebSeed so progress is observable
without public swarm access. The opt-in `./dev.ts e2e --public-fixture` check uses
Big Buck Bunny's poster from <https://webtorrent.io/free-torrents>, with its length
and SHA-256 pinned in `.dev/e2e/core.ts`. Provenance and refresh instructions live
in `.dev/e2e/README.md`.

**Layers:**
1. **Go unit** — `percent()`, magnet parsing/encoding, `FsNode` tree walk + path-escape guard,
   Config defaulting (`IncomingPort`).
2. **Go integration** (`httptest` + real engine, network-gated build tag) — `POST /api/magnet` with a
   pinned fixture → poll `/sync` until `Torrent.Loaded`, then `Started`, then `Percent > 0` → assert
   files land on disk → `GET /download/<file>` returns bytes (and a directory returns a valid zip) →
   `torrent:delete` cleans up.
3. **Playwright E2E** (`./dev.ts e2e`, implemented in `.dev/e2e/`) — run the built single binary against a temp download dir; drive the **UI**:
   type a magnet in the Omni bar → click Load Magnet → assert a Torrent card appears, its progress
   advances, a file shows in Downloads, and the download link serves content. Covers the §10 loop end
   to end through real Vue components + velox.

**Smoke:** `--open` launches the browser; `go build` must yield a single working binary with assets
embedded.

---

## 12. Build, Dev & Packaging

**Dev:** run Go (`:3000`) and Vite (`:5173`) together; `vite.config.ts` proxies `/sync`, `/js/velox.js`,
`/api`, `/search`, `/download` → `:3000`. Hot-reload on the frontend, normal rebuilds on Go.

**Type pipeline:** `./dev.ts gen-types` runs `tygo generate` → `frontend/src/types/generated.ts`. Run it
whenever a synced Go struct changes; CI enforces freshness.

**Prod build:** `vite build` → `frontend/dist` → embedded by `static/embed.go` (`//go:embed dist`) →
`go build` produces the single binary. The server serves embedded assets, falling back to a local dir
in dev if present (parity with `master`'s `serveFiles`).

**CLI flags** (port from `master` via `jpillora/opts`): `--title/-t`, `--port/-p`, `--host/-h`,
`--auth/-a`, `--config-path/-c`, `--key-path/-k`, `--cert-path/-r`, `--log/-l`, `--open/-o`,
`--version/-v`. Config persists to `cloud-torrent.json`.

**Release:** GoReleaser + Docker (`-p 3000 -v /downloads`), as on `master`.

---

## 13. Implementation Milestones

Ordered so the §10 core loop works as early as possible.

- **M0 — Scaffold.** Go module + `main.go` + empty server; Vite + Vue + TS app; `embed.go`; dev proxy;
  `tygo` pipeline producing `generated.ts`; velox `/sync` round-trip rendering a trivial value.
- **M1 — Core loop (priority).** Engine (configure/add-magnet/add-url/start/stop/delete + poll),
  root `State` + velox push loop, **Omni** (magnet + URL modes), **Torrents** list with live progress,
  **Downloads** tree with file streaming + zip, **Footer**. Green Go-integration + Playwright E2E
  against a webtorrent fixture. *Delivers: choose → track → download.*
- **M2 — Header & inputs.** Settings/Config editor; `.torrent` upload (click + full-page drag-drop);
  per-file start/stop; media previews; spacebar control; connect overlay + error banner.
- **M3 — Search.** Scraper providers in the Omni bar (list + item resolution), auto-refreshing
  provider config, `localStorage` of last query/provider.
- **M4 — Parity polish & ship.** CSS/visual parity pass against `master`, basic auth + TLS, request
  logging, `--open`, GoReleaser/Docker, README.

---

## 14. Parity Checklist (verify against `master`)

- [ ] Omni mode detection (url / magnet / search / upload) matches `omni-controller.js`.
- [ ] Magnet build/parse (infohash, dn, tr[]) byte-identical to `magnetURI`/`parseMagnet`.
- [ ] `Percent` / `DownloadRate` formulas match `engine/torrent.go`.
- [ ] Torrent controls + labels (Start/Stop/Cancel/Remove) match `torrents.html` state logic.
- [ ] Download tree icons, 24h auto-collapse, confirm-to-delete match `download-tree.html`.
- [ ] `/download` path-escape guard matches `server_files.go`.
- [ ] Footer fields + formatting match `index.html` footer + `utils.js` filters.
- [ ] Engine add → `GotInfo()` → auto-start, and delete cleanup, match `engine.go`.
- [ ] `IncomingPort` default `50007`; config persists to `cloud-torrent.json`.

---

*Reference implementation: `master` branch. When in doubt, run both and compare.*
