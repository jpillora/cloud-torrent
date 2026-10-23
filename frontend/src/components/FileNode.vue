<script setup lang="ts">
import { ref, computed, watch, onUnmounted, useId } from 'vue'
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
  mdiDotsVertical,
  mdiDownload,
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
  closeActions()
}

// folders auto-collapse when not touched in the last 24h
const open = ref(
  isFile.value ? false : dayjs().diff(dayjs(props.node.Modified), 'hour') <= 24,
)
function toggle() {
  if (!isFile.value) open.value = !open.value
}

const actionsOpen = ref(false)
const actions = ref<HTMLElement>()
const actionsButton = ref<HTMLButtonElement>()
const actionsId = useId()
function closeActions() {
  actionsOpen.value = false
  confirming.value = false
}
function dismissActions(event: PointerEvent) {
  if (!actions.value?.contains(event.target as Node)) closeActions()
}
function escapeActions(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeActions()
    actionsButton.value?.focus()
  }
}
function leaveActions(event: FocusEvent) {
  if (!actions.value?.contains(event.relatedTarget as Node | null)) closeActions()
}
watch(actionsOpen, (open, _, onCleanup) => {
  if (!open) return
  document.addEventListener('pointerdown', dismissActions)
  document.addEventListener('keydown', escapeActions)
  onCleanup(() => {
    document.removeEventListener('pointerdown', dismissActions)
    document.removeEventListener('keydown', escapeActions)
  })
})
watch(downloading, (active) => { if (active) closeActions() })

// confirm-to-delete: Delete -> confirm check -> DELETE the file
// or directory under /download. The node disappears on the next synced tree.
const confirming = ref(false)
const deleting = ref(false)
let confirmationTimer: ReturnType<typeof setTimeout> | undefined
watch(confirming, (value) => { if (!value) clearTimeout(confirmationTimer) })
onUnmounted(() => clearTimeout(confirmationTimer))
function preremove() {
  confirming.value = true
  // auto-cancel the confirmation after 3s, as master does
  clearTimeout(confirmationTimer)
  confirmationTimer = setTimeout(() => {
    confirming.value = false
  }, 3000)
}
async function remove() {
  ui.error = ''
  deleting.value = true
  try {
    const res = await fetch(href.value, { method: 'DELETE' })
    if (!res.ok) throw new Error((await res.text()) || res.statusText)
  } catch (e) {
    ui.error = `Could not delete ${path.value}: ${e instanceof Error ? e.message : String(e)}`
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
      <span class="meta">{{ bytes(node.Size) }} &middot; {{ ago(node.Modified) }}</span>
      <div v-if="!downloading" ref="actions" class="download-actions" @focusout="leaveActions">
        <button
          ref="actionsButton"
          type="button"
          class="actions-toggle"
          :aria-label="`Actions for ${node.Name}`"
          :aria-expanded="actionsOpen"
          :aria-controls="actionsId"
          data-testid="fs-actions-toggle"
          @click="actionsOpen ? closeActions() : (actionsOpen = true)"
        >
          <Icon :path="mdiDotsVertical" />
        </button>
        <div v-if="actionsOpen" :id="actionsId" class="download-actions-menu" data-testid="fs-actions-menu">
          <a
            v-if="!isFile"
            :href="href"
            :aria-label="`Download ${node.Name} as ZIP`"
            data-testid="fs-folder-download"
            @click="closeActions"
          ><Icon :path="mdiDownload" /> Download ZIP</a>
          <button
            v-if="mediaType"
            type="button"
            :aria-pressed="showPreview"
            data-testid="fs-preview-toggle"
            @click="togglePreview"
          >
            <Icon :path="showPreview ? mdiEyeOff : mdiEye" />
            {{ showPreview ? 'Hide preview' : 'Preview' }}
          </button>
          <button
            v-if="!deleting"
            type="button"
            class="delete-action"
            :data-testid="confirming ? 'fs-delete-confirm' : 'fs-delete'"
            @click="confirming ? remove() : preremove()"
          ><Icon :path="confirming ? mdiCheck : mdiDelete" /> {{ confirming ? 'Confirm delete' : 'Delete' }}</button>
          <span v-if="deleting" class="delete-action" role="status"><Icon :path="mdiLoading" spin /> Deleting…</span>
        </div>
      </div>
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
