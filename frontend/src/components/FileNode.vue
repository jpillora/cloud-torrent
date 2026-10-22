<script setup lang="ts">
import { ref, computed } from 'vue'
import dayjs from 'dayjs'
import {
  mdiLoading,
  mdiFolder,
  mdiFolderOpen,
  mdiMusic,
  mdiImage,
  mdiMovie,
  mdiFileOutline,
  mdiEye,
  mdiEyeOff,
  mdiDelete,
  mdiCheck,
} from '@mdi/js'
import type { FsNode } from '../types/generated'
import { downloadingPaths } from '../state'
import { ui } from '../ui'
import { bytes, ago } from '../format'
import Icon from './Icon.vue'

const props = defineProps<{ node: FsNode; basePath?: string }>()

// path relative to the download directory (used for /download links)
const path = computed(() => (props.basePath ? props.basePath + '/' : '') + props.node.Name)
const isFile = computed(() => !props.node.Directory)
const href = computed(
  () => 'download/' + path.value.split('/').map(encodeURIComponent).join('/'),
)
const children = computed<FsNode[]>(() =>
  (props.node.Children ?? []).filter((c): c is FsNode => !!c),
)

// a file is "downloading" if a started torrent holds it incomplete
const downloading = computed(() => downloadingPaths.value.has(path.value))

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
  if (downloading.value) return mdiLoading
  if (!isFile.value) return open.value ? mdiFolderOpen : mdiFolder
  switch (mediaType.value) {
    case 'audio':
      return mdiMusic
    case 'image':
      return mdiImage
    case 'video':
      return mdiMovie
  }
  return mdiFileOutline
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

// confirm-to-delete (master parity): trash -> confirm check -> DELETE the file
// or directory under /download. The node disappears on the next synced tree.
const confirming = ref(false)
const deleting = ref(false)
function preremove() {
  confirming.value = true
  // auto-cancel the confirmation after 3s, as master does
  setTimeout(() => {
    confirming.value = false
  }, 3000)
}
async function remove() {
  deleting.value = true
  try {
    const res = await fetch(href.value, { method: 'DELETE' })
    if (!res.ok) throw new Error((await res.text()) || res.statusText)
  } catch (e) {
    ui.error = e instanceof Error ? e.message : String(e)
    deleting.value = false
    confirming.value = false
  }
}
</script>

<template>
  <div class="node" :class="{ dir: !isFile, file: isFile }" data-testid="fs-node">
    <div class="node-row">
      <button
        v-if="!isFile"
        type="button"
        class="folder-toggle name"
        :aria-expanded="open"
        data-testid="fs-folder-toggle"
        @click="toggle"
      >
        <Icon :path="icon" />
        {{ node.Name }}
      </button>
      <span v-else class="icon"><Icon :path="icon" :spin="downloading" /></span>
      <a v-if="isFile && !downloading" :href="href" class="name" data-testid="fs-file-link">{{
        node.Name
      }}</a>
      <span v-else-if="isFile" class="name">{{ node.Name }}</span>
      <span class="meta">{{ bytes(node.Size) }} &middot; updated {{ ago(node.Modified) }}</span>
      <button
        v-if="mediaType && !downloading"
        class="preview-toggle"
        :class="{ active: showPreview }"
        :title="showPreview ? 'Hide preview' : 'Preview'"
        data-testid="fs-preview-toggle"
        @click="togglePreview"
      >
        <Icon :path="showPreview ? mdiEyeOff : mdiEye" />
      </button>
      <button
        v-if="!downloading && !confirming && !deleting"
        class="del"
        title="Delete"
        data-testid="fs-delete"
        @click="preremove"
      >
        <Icon :path="mdiDelete" />
      </button>
      <button
        v-if="confirming && !deleting"
        class="del confirm"
        title="Confirm delete"
        data-testid="fs-delete-confirm"
        @click="remove"
      >
        <Icon :path="mdiCheck" />
      </button>
      <span v-if="deleting" class="del deleting" title="Deleting…"><Icon :path="mdiLoading" spin /></span>
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
