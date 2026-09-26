# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Cloud Torrent is a self-hosted remote torrent client written in Go. It allows users to start torrents remotely, which are downloaded to the server's local disk and then retrievable or streamable via HTTP.

## Build & Development Commands

**Building the project:**
```bash
go build -o cloud-torrent
```

**Running locally:**
```bash
go run main.go
# Default port: 3000
# Default config: cloud-torrent.json
```

**Test build with goreleaser (multi-platform):**
```bash
goreleaser build --snapshot --config .github/goreleaser.yml
```

**Run with Docker:**
```bash
docker run -d -p 3000:3000 -v /path/to/downloads:/downloads jpillora/cloud-torrent
```

## Architecture

### Three-Layer Design

1. **Main Entry (`main.go`)**: Minimal entry point that initializes the Server with default config and handles CLI argument parsing via `jpillora/opts`
   - Sets version via ldflags: `-X main.version={{.Version}}`

2. **Server Layer (`server/`)**: HTTP server and application state management
   - `server.go`: Core server setup, HTTP routing, real-time sync via Velox (WebSocket-based state sync)
   - `server_api.go`: REST API endpoints for torrent control (magnet, torrent file upload, start/stop/delete)
   - `server_files.go`: File serving and directory listing
   - `server_search.go`: Embedded torrent search via scraper integration
   - `server_stats.go`: System statistics collection

3. **Engine Layer (`engine/`)**: Torrent operations powered by `anacrolix/torrent`
   - `engine.go`: Torrent lifecycle management (add, start, stop, delete)
   - `torrent.go`: Torrent metadata and state tracking
   - `config.go`: Engine configuration (download directory, upload/seeding settings, incoming port)

### Key Components

**State Management**: The server maintains a shared state structure (`server.state`) that includes:
- Torrent list and metadata
- Download directory file tree
- Configuration
- Connected users
- System stats

This state is synchronized to connected web clients in real-time via the Velox library (WebSocket-based).

**Static Assets**: Frontend files are embedded into the binary using Go 1.16+ `embed.FS` (`static/files/`). This allows single-binary distribution.

**API Endpoints** (`/api/`):
- `POST /api/magnet` - Add magnet link
- `POST /api/url` - Download and add torrent from URL
- `POST /api/torrentfile` - Upload .torrent file
- `POST /api/torrent` - Control torrent (start:hash, stop:hash, delete:hash)
- `POST /api/file` - Control individual file download (start:hash:path, stop:hash:path)
- `POST /api/configure` - Update engine configuration

**Search Integration**: Uses `jpillora/scraper` to search multiple torrent indexers through a unified interface.

## Configuration

Configuration is stored in `cloud-torrent.json` (configurable via `--config-path`):
```json
{
  "AutoStart": true,
  "DisableEncryption": false,
  "DownloadDirectory": "./downloads",
  "EnableUpload": true,
  "EnableSeeding": false,
  "IncomingPort": 50007
}
```

Configuration is auto-saved when changed via the API and auto-loaded on startup.

## Runtime Flags

```
--title, -t        Title of this instance (env: TITLE)
--port, -p         Listening port (default 3000, env: PORT)
--host, -h         Listening interface (default all)
--auth, -a         Basic auth 'user:password' (env: AUTH)
--config-path, -c  Config file path (default cloud-torrent.json)
--key-path, -k     TLS Key file path
--cert-path, -r    TLS Certificate file path
--log, -l          Enable request logging
--open, -o         Open browser on startup
```

## Important Implementation Notes

- No test files currently exist in the codebase
- The project uses Go modules (`go.mod` requires Go 1.23.0+)
- Static files must be placed in `static/files/` to be embedded
- The engine polls torrents every 1 second to update state
- System stats are collected every 5 seconds
- HTTP/2 is explicitly disabled due to a Velox compatibility issue (see `server.go:180-181`)
- Authentication is optional but recommended for production (uses `jpillora/cookieauth`)
