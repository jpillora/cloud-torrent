import type { Config, URLRequest, MagnetRequest, TorrentRequest, FileRequest } from './types/generated'
import { ui } from './ui'

// post sends a typed action to POST /api/<action>. JSON bodies are stringified;
// a Uint8Array (a .torrent file) is sent as-is. Failures throw the server's
// error text. State changes are observed via velox, not the response body.
// ui.requests is bumped for the duration so the header shows a spinner.
async function post(action: string, body?: unknown, signal?: AbortSignal): Promise<void> {
  const init: RequestInit = { method: 'POST', signal }
  if (body instanceof Uint8Array) {
    // a .torrent file; Uint8Array is a valid BufferSource body at runtime
    init.body = body as unknown as BodyInit
  } else if (body != null) {
    init.body = JSON.stringify(body)
  }
  ui.requests++
  try {
    const res = await fetch(`/api/${action}`, init)
    if (!res.ok) {
      throw new Error((await res.text()) || res.statusText)
    }
  } finally {
    ui.requests--
  }
}

// api is the typed client for every server mutation.
export const api = {
  configure: (config: Config) => post('configure', config),
  url: (url: string, signal?: AbortSignal) => post('url', { url } satisfies URLRequest, signal),
  magnet: (uri: string, signal?: AbortSignal) => post('magnet', { uri } satisfies MagnetRequest, signal),
  torrentfile: (data: Uint8Array) => post('torrentfile', data),
  torrent: (action: 'start' | 'stop' | 'delete' | 'resume-seeding' | 'resume', infohash: string) =>
    post('torrent', { action, infohash } satisfies TorrentRequest),
  file: (action: 'start' | 'stop', infohash: string, path: string) =>
    post('file', { action, infohash, path } satisfies FileRequest),
}

// SearchResult is one row from a scraper provider (scraper.Result =
// map[string]string). Known fields are typed; the index signature keeps the
// raw, provider-defined shape accessible.
export interface SearchResult {
  name?: string
  url?: string
  path?: string
  magnet?: string
  torrent?: string
  infohash?: string
  tracker?: string
  size?: string
  seeds?: string
  peers?: string
  [k: string]: string | undefined
}

// get issues a GET to /search/* and parses the JSON body, throwing the server's
// error text (the scraper returns {"error": ...} with a non-2xx status).
async function get<T>(path: string, params: Record<string, string>, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${path}?${new URLSearchParams(params)}`, { signal })
  if (!res.ok) {
    throw new Error(extractError(await res.text()) || res.statusText)
  }
  return res.json() as Promise<T>
}

// extractError pulls the message out of the scraper's {"error": "..."} body.
// The scraper does not escape quotes in the message (Go quotes URLs in its
// errors), so the body is frequently invalid JSON — parse leniently, falling
// back to a regex over the raw text rather than surfacing a bare status code.
function extractError(text: string): string {
  if (!text) return ''
  try {
    const body = JSON.parse(text)
    if (body?.error) return String(body.error)
  } catch {
    const m = /"error"\s*:\s*"(.*)"\s*}\s*$/s.exec(text)
    if (m) return m[1]
  }
  return text
}

// search queries the scraper backend (see server/search.go).
export const search = {
  // all runs a provider's list query for a page of results.
  all: (provider: string, query: string, page: number) =>
    get<SearchResult[]>(`/search/${encodeURIComponent(provider)}`, { query, page: String(page) }),
  // one resolves a single result's detail page (magnet/infohash/etc.).
  one: (provider: string, item: string, signal?: AbortSignal) =>
    get<SearchResult>(`/search/${encodeURIComponent(provider)}/item`, { item }, signal),
}
