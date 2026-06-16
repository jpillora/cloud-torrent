<script setup lang="ts">
import { ref, computed } from 'vue'
import dayjs from 'dayjs'
import type { FsNode } from '../types/generated'
import { state } from '../state'
import { bytes, ago } from '../format'

const props = defineProps<{ node: FsNode; basePath?: string }>()

// path relative to the download directory (used for /download links)
const path = computed(() => (props.basePath ? props.basePath + '/' : '') + props.node.Name)
const isFile = computed(() => !props.node.Children)
const href = computed(
  () => 'download/' + path.value.split('/').map(encodeURIComponent).join('/'),
)
const children = computed<FsNode[]>(() =>
  (props.node.Children ?? []).filter((c): c is FsNode => !!c),
)

// a file is "downloading" if a started torrent holds it incomplete
const downloading = computed(() => {
  for (const t of Object.values(state.Torrents ?? {})) {
    if (!t || !t.Started) continue
    for (const f of t.Files ?? []) {
      if (f && f.Path === path.value && f.Percent < 100) return true
    }
  }
  return false
})

const ext = computed(() => /\.([^.]+)$/.exec(props.node.Name)?.[1].toLowerCase() ?? '')

// previewable media kind for files (drives both the icon and the inline preview)
type Media = 'audio' | 'image' | 'video'
const mediaType = computed<Media | null>(() => {
  if (!isFile.value) return null
  if (/^(mp3|m4a|flac|wav|ogg)$/.test(ext.value)) return 'audio'
  if (/^(jpe?g|png|gif|webp)$/.test(ext.value)) return 'image'
  if (/^(mp4|mkv|mov|avi|webm)$/.test(ext.value)) return 'video'
  return null
})

const icon = computed(() => {
  if (downloading.value) return '\u{23F3}' // hourglass
  if (!isFile.value) return open.value ? '\u{1F4C2}' : '\u{1F4C1}' // open/closed folder
  switch (mediaType.value) {
    case 'audio':
      return '\u{1F3B5}'
    case 'image':
      return '\u{1F5BC}'
    case 'video':
      return '\u{1F3AC}'
  }
  return '\u{1F4C4}' // file
})

// inline media preview, rendered (and so sourced from download/<path>) only
// while toggled on
const showPreview = ref(false)
function togglePreview() {
  showPreview.value = !showPreview.value
}

// folders auto-collapse when not touched in the last 24h
const open = ref(
  isFile.value ? false : dayjs().diff(dayjs(props.node.Modified), 'hour') <= 24,
)
function toggle() {
  if (!isFile.value) open.value = !open.value
}
</script>

<template>
  <div class="node" :class="{ dir: !isFile, file: isFile }" data-testid="fs-node">
    <div class="node-row">
      <span class="icon" @click="toggle">{{ icon }}</span>
      <a v-if="isFile && !downloading" :href="href" class="name" data-testid="fs-file-link">{{
        node.Name
      }}</a>
      <span v-else class="name" @click="toggle">{{ node.Name }}</span>
      <span class="meta">{{ bytes(node.Size) }} &middot; updated {{ ago(node.Modified) }}</span>
      <button
        v-if="mediaType && !downloading"
        class="preview-toggle"
        :class="{ active: showPreview }"
        :title="showPreview ? 'Hide preview' : 'Preview'"
        data-testid="fs-preview-toggle"
        @click="togglePreview"
      >
        {{ showPreview ? '▾' : '▶' }}
      </button>
    </div>
    <div v-if="showPreview && mediaType" class="preview" data-testid="fs-preview">
      <audio v-if="mediaType === 'audio'" controls :src="href"></audio>
      <img v-else-if="mediaType === 'image'" :src="href" :alt="node.Name" />
      <video v-else-if="mediaType === 'video'" controls :src="href"></video>
    </div>
    <div v-if="!isFile && open" class="children">
      <FileNode v-for="c in children" :key="c.Name" :node="c" :base-path="path" />
    </div>
  </div>
</template>
