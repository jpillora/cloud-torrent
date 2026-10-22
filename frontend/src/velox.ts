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
    const timeout = setTimeout(() => fail('timed out loading /js/velox.js'), ATTEMPT_TIMEOUT)
    function fail(message: string) {
      clearTimeout(timeout)
      el.onload = el.onerror = null
      el.remove()
      reject(new Error(message))
    }
    el.onload = () => {
      clearTimeout(timeout)
      if (window.velox) resolve(window.velox)
      else fail('velox client did not register')
    }
    el.onerror = () => fail('failed to load /js/velox.js')
    document.head.appendChild(el)
  }).catch((err) => {
    loading = null
    throw err
  })
  return loading
}

// connect loads the velox client, syncs `target` in place from `url`, and drives
// a visible reconnect loop, reporting every transition via onStatus.
export function connect(
  url: string,
  target: object,
  onStatus: (s: ConnStatus) => void,
): Conn {
  let conn: VeloxConn | null = null
  let generation = 0
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
  function closeConnection() {
    if (!conn) return
    conn.onconnect = conn.ondisconnect = conn.onerror = () => {}
    conn.disconnect()
  }

  function failAttempt(error?: unknown) {
    generation++
    closeConnection()
    status.error = error instanceof Error ? error.message : typeof error === 'string' ? error : 'Could not reach the server'
    scheduleRetry()
  }

  // a connection attempt is underway (initial or a retry)
  async function attemptNow() {
    clearTimers()
    closeConnection()
    const current = ++generation
    status.connected = false
    status.phase = attempt === 0 ? 'connecting' : 'retrying'
    status.retryInSec = 0
    status.attempt = attempt
    emit()
    try {
      const velox = await loadClient()
      if (current !== generation) return
      const reconnect = conn !== null
      conn ??= velox(url, target, { retry: false }) // construction connects
      conn.onconnect = () => {
        if (current !== generation) return
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
        if (current === generation) failAttempt()
      }
      conn.onerror = (err: unknown) => {
        if (current === generation) failAttempt(err)
      }
      if (reconnect) conn.connect()
      // velox swallows SSE connection errors; bound every transport attempt.
      attemptTimer = setTimeout(() => failAttempt(), ATTEMPT_TIMEOUT)
    } catch (err) {
      if (current === generation) failAttempt(err)
    }
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

  // skip the backoff wait and try immediately when the network returns or the
  // tab is refocused (don't interrupt an attempt already in flight)
  function wake() {
    if (!status.connected && status.phase === 'waiting') attemptNow()
  }
  window.addEventListener('online', wake)
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') wake()
  })

  void attemptNow()

  return { retryNow: () => { void attemptNow() } }
}
