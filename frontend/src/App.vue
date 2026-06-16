<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { uploadTorrentFiles } from './upload'
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
  try {
    await uploadTorrentFiles(files)
  } catch (err) {
    // surfaced by the global error banner once that lands (#20)
    console.error('torrent upload failed:', err)
  }
}

onMounted(() => {
  window.addEventListener('dragover', onDragOver)
  window.addEventListener('drop', onDrop)
})
onBeforeUnmount(() => {
  window.removeEventListener('dragover', onDragOver)
  window.removeEventListener('drop', onDrop)
  clearTimeout(hideTimer)
})
</script>

<template>
  <div class="app">
    <Header />
    <ConfigEditor />
    <Omni />
    <Torrents />
    <Downloads />
    <Footer />
    <div v-show="dragging" class="file-drop-cover" data-testid="drop-cover">
      <div class="msg">Drop torrent files here</div>
    </div>
  </div>
</template>
