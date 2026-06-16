import { reactive } from 'vue'
import type { State } from './types/generated'
import { connect, type Conn, type ConnPhase } from './velox'

// state is the root application state, synced from the Go backend via velox.
// It starts empty and is filled in place as updates arrive.
export const state = reactive<Partial<State>>({})

// connection drives the connect overlay + header lightning. Beyond connected/
// hasConnected it surfaces the reconnect loop: the current phase, the attempt
// count, a countdown to the next attempt, and the last error.
export const connection = reactive({
  connected: false,
  hasConnected: false,
  phase: 'connecting' as ConnPhase,
  attempt: 0,
  retryInSec: 0,
  error: '',
})

let conn: Conn | null = null

// startSync opens the realtime connection and keeps `state` live.
export function startSync(): void {
  connect('/sync', state, (s) => {
    connection.connected = s.connected
    connection.phase = s.phase
    connection.attempt = s.attempt
    connection.retryInSec = s.retryInSec
    connection.error = s.error
    if (s.connected) connection.hasConnected = true
  })
    .then((c) => {
      conn = c
    })
    .catch((err) => {
      console.error('velox connect failed:', err)
    })
}

// retryNow forces an immediate reconnect attempt (the overlay's "Retry now").
export function retryNow(): void {
  conn?.retryNow()
}
