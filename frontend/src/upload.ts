import { api } from './api'

// uploadTorrentFiles filters a file list down to `.torrent` files, reads each as
// bytes and uploads it via api.torrentfile. It is the shared upload path for
// both the full-page drop cover and the Omni upload-icon picker (master's
// run.js `uploadTorrent`). Returns the number of files uploaded (0 if none
// matched). api.torrentfile throws on a server error; callers surface it.
export async function uploadTorrentFiles(files: FileList | File[]): Promise<number> {
  const torrents = Array.from(files).filter((f) => f.name.endsWith('.torrent'))
  await Promise.all(
    torrents.map(async (file) => {
      const buf = await file.arrayBuffer()
      await api.torrentfile(new Uint8Array(buf))
    }),
  )
  return torrents.length
}
