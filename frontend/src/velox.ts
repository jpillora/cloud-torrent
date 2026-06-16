// Typed wrapper around the velox realtime client served by the Go backend at
// /js/velox.js. The client keeps `target` synced in place as state arrives, so
// pointing it at a Vue `reactive` object makes the UI update automatically.

export interface VeloxConn {
  onupdate: (() => void) | null
  onchange: ((connected: boolean) => void) | null
}

type VeloxFactory = (url: string, target: object) => VeloxConn

declare global {
  interface Window {
    velox?: VeloxFactory
  }
}

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

// connect loads the velox client and syncs `target` in place from `url`,
// reporting connect/disconnect via onConnectionChange.
export async function connect(
  url: string,
  target: object,
  onConnectionChange?: (connected: boolean) => void,
): Promise<VeloxConn> {
  const velox = await loadClient()
  const conn = velox(url, target)
  if (onConnectionChange) {
    conn.onchange = (connected: boolean) => onConnectionChange(connected)
  }
  return conn
}
