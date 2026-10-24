<img src="./.github/screenshot.png" alt="screenshot" width="600"/>

# Cloud Torrent

**Cloud Torrent** is a self-hosted remote torrent client: you add torrents
remotely, they download to the server's disk as sets of files, and you browse
or stream those files over HTTP.

This is a ground-up rewrite of the original
[cloud-torrent](https://github.com/jpillora/cloud-torrent) — a **Go** backend
([anacrolix/torrent](https://github.com/anacrolix/torrent) engine,
[jpillora/velox](https://github.com/jpillora/velox) realtime sync) with a
**Vue 3 + Vite + TypeScript** frontend, all compiled into a single binary.

### Features

* Single binary, cross-platform
* Add torrents by magnet, `.torrent` URL, or drag-and-drop upload
* Embedded torrent search ([jpillora/scraper](https://github.com/jpillora/scraper))
* Real-time updates (no refresh)
* Inline media previews (audio / image / video) with spacebar play/pause
* Mobile-friendly
* Fast [content server](https://pkg.go.dev/net/http#ServeContent) (stream while downloading)

### Install

**Binaries**

[![Releases](https://img.shields.io/github/v/release/jpillora/cloud-torrent?include_prereleases)](https://github.com/jpillora/cloud-torrent/releases)

Download binaries from [GitHub Releases](https://github.com/jpillora/cloud-torrent/releases). New releases are published as prereleases for Linux, macOS, and Windows.

**Docker**

Docker image publishing is disabled. To build and run the current source locally:

```sh
docker build -f .dev/Dockerfile -t cloud-torrent .
docker run -d -p 3000:3000 -v /path/to/downloads:/downloads cloud-torrent
```

**Source**

*[Go](https://go.dev/dl/) 1.27.3+, [Node](https://nodejs.org) 24+, and [Bun](https://bun.sh) 1.4.4+ are required.*

```sh
git clone https://github.com/jpillora/cloud-torrent
cd cloud-torrent
./dev.ts build                   # builds the SPA and dist/cloud-torrent
./dist/cloud-torrent --port 3000
```

### Usage

```
$ cloud-torrent --help

  Usage: cloud-torrent [options]

  Options:
  --title, -t        Title of this instance (default Cloud Torrent)
  --port, -p         Listening port (default 3000)
  --host, -h         Listening interface (default all)
  --auth, -a         Optional basic auth in form 'user:password'
  --config-path, -c  Configuration file path (default cloud-torrent.json)
  --key-path, -k     TLS Key file path
  --cert-path        TLS Certificate file path
  --log, -l          Enable request logging
  --open, -o         Open browser after the server starts
  --version, -v      display version
  --help             display help
```

The download directory and engine options are configured in the UI (gear icon)
and persisted to `cloud-torrent.json`.

### Development

The Go structs are the single source of truth for data shapes; the TypeScript
types are generated from them with [tygo](https://github.com/gzuidhof/tygo).
Build commands use [Take](https://github.com/jpillora/take) via `./dev.ts`;
run `./dev.ts --help` to list commands. Build and release configuration lives in `.dev/`.

```sh
./dev.ts gen-types                # regenerate frontend/src/types/*.gen.ts
./dev.ts check-types              # CI guard: regenerate + fail if stale
npm --prefix frontend run dev     # Vite dev server (proxies /sync /api -> :3000)
./dev.ts test                     # backend tests
./dev.ts ci                       # types, build, Go tests, and browser E2E
```

### Browser end-to-end checks

Install Chromium once with `./dev.ts e2e --install-browser`. On Linux machines
missing browser libraries, add `--with-deps` (may require sudo). Subsequent runs
use `./dev.ts e2e`; this builds the embedded app before testing. Use `--skip-build`
only when `dist/cloud-torrent` is already current. `CHROMIUM_EXECUTABLE_PATH` can
select an existing compatible Chromium installation.

The default Playwright test uses a pinned, locally generated CC0 torrent with a
throttled loopback WebSeed. It exercises the real Omni input, metadata/progress
updates over SSE, exact downloaded bytes, keyboard folder controls, and ZIP
contents after removing the torrent card. It needs no public torrent service;
package/browser installation still requires internet. Both the app configuration
and downloads live in a temporary directory and are removed on exit, including
failures and interrupts. Production services and download folders are never used.

For the public fixture from [WebTorrent's free torrents](https://webtorrent.io/free-torrents),
run `./dev.ts e2e --public-fixture`. This opt-in check downloads only Big Buck
Bunny's `poster.jpg` (310,380 bytes), verifies its pinned SHA-256, and verifies the
folder archive. It requires outbound internet and allows a longer timeout. The
fixture provenance and refresh procedure are in [.dev/e2e/README.md](.dev/e2e/README.md).

GitHub Actions runs the deterministic browser test as a separate Take step;
`./dev.ts ci` includes it locally as well. The Go public-network poster/content smoke test
remains available with `go test -tags e2e -run E2E ./server/...`.

### Credits

Powered by [anacrolix/torrent](https://github.com/anacrolix/torrent). A rewrite
of the original [Node version](https://github.com/jpillora/node-torrent-cloud).

Operational files (the configured settings file, TLS key/certificate, and private
state directory) are excluded from download listings and archives, and cannot
be retrieved or deleted through the download API. Aliases and parent-folder
removal are checked too. `--state-directory` reserves a private location outside
the download tree; overlapping directories are rejected. Settings writes use
owner-only permissions.
