import type { Config } from './types/generated'

// post sends a typed action to POST /api/<action>. JSON bodies are stringified;
// a Uint8Array (a .torrent file) is sent as-is. Failures throw the server's
// error text. State changes are observed via velox, not the response body.
async function post(action: string, body?: unknown): Promise<void> {
  const init: RequestInit = { method: 'POST' }
  if (body instanceof Uint8Array) {
    // a .torrent file; Uint8Array is a valid BufferSource body at runtime
    init.body = body as unknown as BodyInit
  } else if (body != null) {
    init.body = JSON.stringify(body)
  }
  const res = await fetch(`/api/${action}`, init)
  if (!res.ok) {
    throw new Error((await res.text()) || res.statusText)
  }
}

// api is the typed client for every server mutation.
export const api = {
  configure: (config: Config) => post('configure', config),
  url: (url: string) => post('url', { url }),
  magnet: (uri: string) => post('magnet', { uri }),
  torrentfile: (data: Uint8Array) => post('torrentfile', data),
  torrent: (action: 'start' | 'stop' | 'delete', infohash: string) =>
    post('torrent', { action, infohash }),
  file: (action: 'start' | 'stop', infohash: string, path: string) =>
    post('file', { action, infohash, path }),
}
