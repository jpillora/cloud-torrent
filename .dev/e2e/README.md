# Browser contract fixtures

`./dev.ts e2e` builds and runs the embedded app in temporary storage, drives the
real Vue UI in Playwright Chromium, and checks the actual SSE connection, progress,
file contents, and decoded ZIP entries. No mocked sync or injected app state is used.
Playwright and ZIP-reader versions are locked in this directory's package-lock.json.

Appearance checks cover the system default, live OS changes, explicit light/dark
overrides, reload persistence, tab synchronization, denied theme storage, keyboard
selection and mobile settings layout. The torrent/download workflow runs in dark mode.

## Deterministic fixture (default, CI)

`fixture.ts` authors two CC0 text files and canonically bencodes a v1 torrent with
16 KiB SHA-1 pieces. Its pinned infohash is
`5705481528e4be430011e1afab78b230664c6e2e`. A loopback HTTP server supplies metadata
and throttled range requests, so intermediate progress is observable without an
external swarm. Folder/file names cover spaces, `#`, `+`, `%`, Unicode and nesting.

To deliberately change this fixture, update the content/metadata in `fixture.ts`,
recompute its infohash using SHA-1 of the canonical bencoded info dictionary, update
the assertion in `core.ts` and this document, then run the browser test. Expected
file and ZIP content comes from the authored bytes, independently of the engine.

## Public fixture (opt-in network check)

`./dev.ts e2e --public-fixture` uses:

- Source/catalog: https://webtorrent.io/free-torrents
- Metainfo: https://webtorrent.io/torrents/big-buck-bunny.torrent
- Content: Blender Foundation's freely distributed Big Buck Bunny `poster.jpg`.
- Expected length: 310,380 bytes.
- SHA-256: `d082c4ae86f4f8d323c4a03665cdec24c20ad0dcaddcfef1bcab073457a101ff`.

AutoStart is disabled in this mode. The test selects only the poster through the
UI, avoiding the film download. Verifying the poster also fetches a shared boundary
piece into a sparse movie `.part` file; after removing the torrent card, the test
deletes that partial file through the UI before downloading the archive. It verifies
both the downloaded poster and ZIP
content against the checksum. This mode requires public HTTP access; it is not
part of routine CI. Refresh a changed public fixture by confirming the source and
license, independently downloading and hashing the poster, updating the URL/name,
length/checksum in `core.ts` and here, and rerunning the public check. Never update
the expected hash merely to silence a mismatch.

## Runtime isolation and cleanup

The test allocates loopback ports and creates its own config, downloads and logs
under the OS temporary directory. A fresh browser context supplies empty storage.
Finally blocks and SIGINT/SIGTERM handlers close the browser, terminate the child
app, close the WebSeed, and remove scratch files. SIGKILL and machine failure cannot
run cleanup handlers. On failures, browser errors and the isolated server log are
printed before cleanup. Neither test targets the production URL or mounts live data.

Chromium is installed with `./dev.ts e2e --install-browser`; `--with-deps` also
installs Linux dependencies (potentially via sudo). CI installs both explicitly.
`--skip-build` reuses the current binary; otherwise frontend and Go builds run first.
