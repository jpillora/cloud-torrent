<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { mdiFlash, mdiClose } from '@mdi/js'
import { uploadTorrentFiles } from './upload'
import { connection, retryNow } from './state'
import { ui } from './ui'

// connection overlay copy, driven by the reconnect loop's phase
const connTitle = computed(() => (connection.hasConnected ? 'Connection lost' : 'Connecting'))
const connDetail = computed(() => {
  switch (connection.phase) {
    case 'waiting':
      return `Next attempt in ${connection.retryInSec}s · attempt ${connection.attempt}`
    case 'retrying':
      return `Reconnecting… · attempt ${connection.attempt}`
    default:
      return 'Reaching the server…'
  }
})
import Icon from './components/Icon.vue'
import Header from './components/Header.vue'
import ConfigEditor from './components/ConfigEditor.vue'
import Omni from './components/Omni.vue'
import Torrents from './components/Torrents.vue'
import Downloads from './components/Downloads.vue'
import Footer from './components/Footer.vue'

// Full-page .torrent drop target. Listeners live on window so a drop anywhere
// on the page is caught (the app column is narrower than the viewport).
// dragleave is unreliable (it fires when crossing child elements), so — like
// master's ondropfile — each dragover re-arms a short timer that hides the
// cover once the drag stops.
const dragging = ref(false)
let hideTimer: number | undefined

function hasFiles(e: DragEvent): boolean {
  return !!e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files')
}

function onDragOver(e: DragEvent) {
  if (!hasFiles(e)) return
  e.preventDefault() // allow the drop
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
  dragging.value = true
  clearTimeout(hideTimer)
  hideTimer = window.setTimeout(() => (dragging.value = false), 200)
}

async function onDrop(e: DragEvent) {
  if (!hasFiles(e)) return
  e.preventDefault()
  clearTimeout(hideTimer)
  dragging.value = false
  const files = e.dataTransfer?.files
  if (!files) return
  ui.error = ''
  try {
    await uploadTorrentFiles(files)
  } catch (err) {
    ui.error = err instanceof Error ? err.message : String(err)
  }
}

// Page-wide spacebar: play/pause the first media player in the viewport (unless
// focus is on an interactive control).
function onKeydown(e: KeyboardEvent) {
  if (e.code !== 'Space' || e.defaultPrevented || e.repeat || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return
  const el = document.activeElement as HTMLElement | null
  if (el && (el.isContentEditable || el.closest(
    'input, textarea, select, button, a[href], summary, audio, video, [role="button"], [role="link"], [role="checkbox"], [role="switch"], [role="slider"], [role="textbox"], [role="combobox"], [tabindex]:not([tabindex="-1"])',
  ))) return
  const h = window.innerHeight || document.documentElement.clientHeight
  for (const m of document.querySelectorAll<HTMLMediaElement>('video, audio')) {
    const r = m.getBoundingClientRect()
    const inView = (r.top >= 0 && r.top <= h) || (r.bottom >= 0 && r.bottom <= h)
    if (!inView) continue
    if (m.paused) void m.play().catch(() => {})
    else m.pause()
    e.preventDefault()
    break
  }
}

onMounted(() => {
  window.addEventListener('dragover', onDragOver)
  window.addEventListener('drop', onDrop)
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('dragover', onDragOver)
  window.removeEventListener('drop', onDrop)
  window.removeEventListener('keydown', onKeydown)
  clearTimeout(hideTimer)
})
</script>

<template>
  <div class="app">
    <Header />

    <!-- global, dismissible error banner -->
    <div v-if="ui.error" class="error-banner" data-testid="error-banner">
      <button class="close" title="Dismiss" data-testid="error-dismiss" @click="ui.error = ''">
        <Icon :path="mdiClose" />
      </button>
      <div class="error-header">Error</div>
      <p>{{ ui.error }}</p>
    </div>

    <ConfigEditor />
    <Omni />
    <Torrents />
    <Downloads />
    <Footer />

    <div v-show="dragging" class="file-drop-cover" data-testid="drop-cover">
      <div class="msg">Drop torrent files here</div>
    </div>

    <!-- connection overlay: dims/blurs the app whenever the realtime sync is
         down, and shows the reconnect loop's live status -->
    <div v-if="!connection.connected" class="connect-overlay" data-testid="connect-overlay">
      <div class="connect-card">
        <div class="connect-pulse" :class="connection.phase">
          <Icon :path="mdiFlash" />
        </div>
        <h2 data-testid="connect-status">{{ connTitle }}</h2>
        <p class="connect-detail" data-testid="connect-detail">{{ connDetail }}</p>
        <p v-if="connection.error" class="connect-error" data-testid="connect-error">
          {{ connection.error }}
        </p>
        <button class="blue" data-testid="connect-retry" @click="retryNow">Retry now</button>
      </div>
    </div>
  </div>
</template>
