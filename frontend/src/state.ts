import { reactive } from 'vue'
import type { State } from './types/generated'
import { connect } from './velox'

// state is the root application state, synced from the Go backend via velox.
// It starts empty and is filled in place as updates arrive.
export const state = reactive<Partial<State>>({})

// connection drives connection-dependent UI (disconnect overlay, header
// lightning, etc.).
export const connection = reactive({
  connected: false,
  hasConnected: false,
})

// startSync opens the realtime connection and keeps `state` live.
export function startSync(): void {
  connect('/sync', state, (connected) => {
    connection.connected = connected
    if (connected) connection.hasConnected = true
  }).catch((err) => {
    console.error('velox connect failed:', err)
  })
}
