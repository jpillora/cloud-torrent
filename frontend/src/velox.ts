// Typed wrapper around the velox realtime client served by the Go backend at
// /js/velox.js. The client keeps `target` synced in place as state arrives, so
// pointing it at a Vue `reactive` object makes the UI update automatically.
//
// velox has its own auto-retry, but it is opaque (no "waiting"/"attempting"
// signal, and SSE connection errors are swallowed). We disable it and drive the
// reconnect loop here so the UI can show exactly what the loop is doing.

export interface VeloxConn {
  onupdate: (() => void) | null
  onconnect: (() => void) | null
  ondisconnect: (() => void) | null
  onerror: ((err: unknown) => void) | null
  connect: () => void
  disconnect: () => void
}

interface VeloxOpts {
  retry?: boolean
}
type VeloxFactory = (url: string, target: object, opts?: VeloxOpts) => VeloxConn

declare global {
  interface Window {
    velox?: VeloxFactory
  }
}

export type ConnPhase = 'connecting' | 'connected' | 'waiting' | 'retrying'

export interface ConnStatus {
  connected: boolean
  phase: ConnPhase
  attempt: number // failed attempts since the last successful connect
  retryInSec: number // seconds until the next attempt (during 'waiting')
  error: string // last connection error, if any
}

// A connection handle exposing a manual, immediate retry.
export interface Conn {
  retryNow: () => void
}

const RETRY_MIN = 1000 // first backoff
const RETRY_MAX = 15000 // backoff ceiling
const ATTEMPT_TIMEOUT = 4000 // give up on an attempt if not connected by now

let loading: Promise<VeloxFactory> | null = null

// loadClient injects /js/velox.js once and resolves the global factory it
// registers (served by Go in prod, proxied to the Go server in dev).
function loadClient(): Promise<VeloxFactory> {
  if (window.velox) return Promise.resolve(window.velox)
  if (loading) return loading
  loading = new Promise<VeloxFactory>((resolve, reject) => {
    const el = document.createElement('script')
    el.src = '/js/velox.js'
    el.async = false
    el.onload = () =>
      window.velox ? resolve(window.velox) : reject(new Error('velox client did not register'))
    el.onerror = () => reject(new Error('failed to load /js/velox.js'))
    document.head.appendChild(el)
  })
  return loading
}

// connect loads the velox client, syncs `target` in place from `url`, and drives
// a visible reconnect loop, reporting every transition via onStatus.
export async function connect(
  url: string,
  target: object,
  onStatus: (s: ConnStatus) => void,
): Promise<Conn> {
  const velox = await loadClient()
  const conn = velox(url, target, { retry: false }) // we own the retry loop

  let attempt = 0
  let waitTimer: ReturnType<typeof setTimeout> | undefined
  let attemptTimer: ReturnType<typeof setTimeout> | undefined
  let countdown: ReturnType<typeof setInterval> | undefined

  const status: ConnStatus = {
    connected: false,
    phase: 'connecting',
    attempt: 0,
    retryInSec: 0,
    error: '',
  }
  const emit = () => onStatus({ ...status })
  const clearTimers = () => {
    clearTimeout(waitTimer)
    clearTimeout(attemptTimer)
    clearInterval(countdown)
  }

  // an attempt didn't connect in time. velox swallows SSE connection errors, so
  // surface a generic reason unless a specific one was already reported.
  function failAttempt() {
    if (!status.error) status.error = 'Could not reach the server'
    scheduleRetry()
  }

  // a connection attempt is underway (initial or a retry)
  function attemptNow() {
    clearTimers()
    status.connected = false
    status.phase = 'retrying'
    status.retryInSec = 0
    status.attempt = attempt
    emit()
    conn.connect()
    // velox doesn't report a failed SSE attempt, so fall back to a timeout:
    // if onconnect hasn't fired by now, treat the attempt as failed.
    attemptTimer = setTimeout(failAttempt, ATTEMPT_TIMEOUT)
  }

  // wait (with a visible countdown) before the next attempt
  function scheduleRetry() {
    clearTimers()
    attempt++
    const delay = Math.min(RETRY_MIN * 2 ** (attempt - 1), RETRY_MAX)
    status.connected = false
    status.phase = 'waiting'
    status.attempt = attempt
    status.retryInSec = Math.round(delay / 1000)
    emit()
    countdown = setInterval(() => {
      status.retryInSec = Math.max(0, status.retryInSec - 1)
      emit()
    }, 1000)
    waitTimer = setTimeout(attemptNow, delay)
  }

  conn.onconnect = () => {
    clearTimers()
    attempt = 0
    status.connected = true
    status.phase = 'connected'
    status.attempt = 0
    status.retryInSec = 0
    status.error = ''
    emit()
  }
  conn.ondisconnect = () => {
    if (status.phase !== 'waiting') scheduleRetry()
  }
  conn.onerror = (err: unknown) => {
    status.error = err instanceof Error ? err.message : typeof err === 'string' ? err : 'connection error'
    emit()
  }

  // skip the backoff wait and try immediately when the network returns or the
  // tab is refocused (don't interrupt an attempt already in flight)
  function wake() {
    if (!status.connected && status.phase === 'waiting') attemptNow()
  }
  window.addEventListener('online', wake)
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') wake()
  })

  // velox connects on construction; reflect that and arm the failure backstop.
  emit()
  attemptTimer = setTimeout(failAttempt, ATTEMPT_TIMEOUT)

  return { retryNow: attemptNow }
}
