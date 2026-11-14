<script setup lang="ts">
import { reactive, ref, computed, watch, useId } from 'vue'
import {
  mdiCheck, mdiArrowDown, mdiArrowUp, mdiPause, mdiPlay, mdiClockOutline,
  mdiLoading, mdiFormatListBulleted, mdiDotsHorizontal, mdiInformationOutline,
  mdiDeleteOutline, mdiPlayBoxMultipleOutline,
} from '@mdi/js'
import type { Torrent, File } from '../types/generated'
import { api } from '../api'
import { bytes } from '../format'
import AsyncButton from './AsyncButton.vue'
import Icon from './Icon.vue'

const props = defineProps<{ torrent: Torrent }>()
const showFiles = ref(false)
const showDetails = ref(false)
const panelId = useId()
const files = computed<File[]>(() =>
  (props.torrent.Files ?? []).filter((f): f is File => !!f),
)
const selectedFiles = computed(() => files.value.some(f => f.Selected))
const sharingLimit = computed(() => ['seed-ratio', 'seed-time'].includes(props.torrent.StopReason))
const progress = computed(() => Math.min(100, finiteValue(props.torrent.Percent)))
const stateIcon = computed(() => ({
  queued: mdiClockOutline, metadata: mdiLoading, downloading: mdiArrowDown,
  seeding: mdiArrowUp, paused: mdiPause, completed: mdiCheck,
  'stopped-by-policy': mdiPause,
}[props.torrent.State] ?? mdiClockOutline))
const stateLabel = computed(() => ({
  queued: 'Queued', metadata: 'Fetching metadata', downloading: 'Downloading',
  seeding: 'Seeding', paused: 'Paused', completed: 'Completed',
  'stopped-by-policy': 'Stopped by policy',
}[props.torrent.State] ?? 'Waiting'))
function finiteValue(value: number | undefined): number {
  return value !== undefined && Number.isFinite(value) && value >= 0 ? value : 0
}
const seedDuration = computed(() => {
  const seconds = Math.floor(finiteValue(props.torrent.SeedSeconds))
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  return `${Math.floor(seconds / 3600)}h ${Math.floor(seconds % 3600 / 60)}m`
})

const actionsOpen = ref(false)
const actions = ref<HTMLElement>()
const actionsButton = ref<HTMLButtonElement>()
function closeActions() {
  actionsOpen.value = false
}
function dismissActions(event: PointerEvent) {
  if (!actions.value?.contains(event.target as Node)) closeActions()
}
function escapeActions(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  closeActions()
  actionsButton.value?.focus()
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
function toggleDetails() {
  showDetails.value = !showDetails.value
  closeActions()
  actionsButton.value?.focus()
}
async function menuAction(action: () => Promise<unknown>) {
  await action()
  closeActions()
  actionsButton.value?.focus()
}

// Per-file selection errors, keyed by file path.
const fileErrors = reactive<Record<string, string>>({})

// link the name to the torrent's zip download once complete
const zipUrl = computed(() =>
  props.torrent.Percent === 100 ? 'download/' + encodeURIComponent(props.torrent.Name) : '',
)

// return the promise so AsyncButton can show its loading state until the
// request settles (failures surface in the global error banner)
function start() {
  return api.torrent(selectedFiles.value ? 'resume' : 'start', props.torrent.InfoHash)
}
function startAll() {
  return api.torrent('start', props.torrent.InfoHash)
}
function resumeSeeding() {
  return api.torrent('resume-seeding', props.torrent.InfoHash)
}
function stop() {
  return api.torrent('stop', props.torrent.InfoHash)
}
function remove() {
  return api.torrent('delete', props.torrent.InfoHash)
}

// Selection is separate from whether the torrent is currently transferring.
async function setFile(f: File, action: 'start' | 'stop') {
  delete fileErrors[f.Path]
  try {
    await api.file(action, props.torrent.InfoHash, f.Path)
  } catch (e) {
    fileErrors[f.Path] = e instanceof Error ? e.message : String(e)
  }
}

</script>

<template>
  <div class="torrent" :class="{ loading: !torrent.Loaded }" data-testid="torrent">
    <div class="torrent-header">
      <div class="torrent-name" :title="torrent.Name" data-testid="torrent-name">
        <a v-if="zipUrl" :href="zipUrl">{{ torrent.Name }}</a>
        <span v-else>{{ torrent.Name }}</span>
      </div>
      <div class="torrent-controls">
        <button
          v-if="torrent.Loaded"
          type="button"
          class="torrent-icon-button"
          :class="{ active: showFiles }"
          aria-label="Files"
          title="Files"
          :aria-expanded="showFiles"
          :aria-controls="`${panelId}-files`"
          @click="showFiles = !showFiles"
        ><Icon :path="mdiFormatListBulleted" /></button>
        <AsyncButton
          v-if="!sharingLimit && !['queued', 'metadata'].includes(torrent.State) && !torrent.Started"
          class="torrent-icon-button resume-action"
          :action="start"
          :aria-label="selectedFiles ? 'Resume selected' : 'Start all'"
          :title="selectedFiles ? 'Resume selected' : 'Start all'"
          data-testid="torrent-start"
        ><Icon :path="mdiPlay" /></AsyncButton>
        <AsyncButton
          v-if="torrent.Started || torrent.State === 'queued' || torrent.State === 'metadata'"
          class="torrent-icon-button"
          :action="stop"
          aria-label="Pause"
          title="Pause"
          data-testid="torrent-stop"
        ><Icon :path="mdiPause" /></AsyncButton>
        <div ref="actions" class="torrent-actions" @focusout="leaveActions">
          <button
            ref="actionsButton"
            type="button"
            class="torrent-icon-button"
            :class="{ active: actionsOpen }"
            :aria-label="`More options for ${torrent.Name}`"
            title="More options"
            :aria-expanded="actionsOpen"
            :aria-controls="`${panelId}-actions`"
            data-testid="torrent-actions-toggle"
            @click="actionsOpen = !actionsOpen"
          ><Icon :path="mdiDotsHorizontal" /></button>
          <div v-if="actionsOpen" :id="`${panelId}-actions`" class="torrent-actions-menu" data-testid="torrent-actions-menu">
            <button
              type="button"
              :aria-expanded="showDetails"
              :aria-controls="`${panelId}-details`"
              data-testid="torrent-details-toggle"
              @click="toggleDetails"
            ><Icon :path="mdiInformationOutline" /> {{ showDetails ? 'Hide details' : 'Details' }}</button>
            <AsyncButton
              v-if="torrent.Loaded && selectedFiles && files.some(f => !f.Selected) && !torrent.StopReason"
              :action="() => menuAction(startAll)"
              data-testid="torrent-start-all"
            ><Icon :path="mdiPlayBoxMultipleOutline" /> Start all files</AsyncButton>
            <AsyncButton
              v-if="sharingLimit"
              :action="() => menuAction(resumeSeeding)"
              data-testid="torrent-resume-seeding"
            ><Icon :path="mdiPlay" /> Resume sharing (ignore limits)</AsyncButton>
            <AsyncButton
              v-if="!torrent.Started"
              class="remove-action"
              :action="() => menuAction(remove)"
              data-testid="torrent-remove"
            ><Icon :path="mdiDeleteOutline" /> {{ torrent.Loaded ? 'Remove torrent' : 'Cancel torrent' }}</AsyncButton>
          </div>
        </div>
      </div>
    </div>

    <div class="torrent-state-row">
      <span class="torrent-state" :class="{ sharing: torrent.State === 'seeding', complete: torrent.State === 'completed' }" data-testid="torrent-queue-state">
        <Icon :path="stateIcon" :spin="torrent.State === 'metadata'" />
        {{ stateLabel }}<span v-if="torrent.QueuePosition" :title="`Queue position ${torrent.QueuePosition}`"> · #{{ torrent.QueuePosition }}</span>
        <span v-if="torrent.SeedingOverride" title="Sharing limits overridden until you pause this torrent." aria-label="Sharing limits overridden"><Icon :path="mdiInformationOutline" /></span>
      </span>
      <span v-if="torrent.Loaded" class="torrent-percent">{{ Math.round(progress * 10) / 10 }}%</span>
    </div>
    <div
      v-if="torrent.Loaded"
      class="progress torrent-progress"
      :class="{ active: torrent.Started && progress > 0 && progress < 100, complete: progress === 100 }"
      role="progressbar"
      :aria-label="`Download progress for ${torrent.Name}`"
      :aria-valuenow="progress"
      :aria-valuemin="0"
      :aria-valuemax="100"
    ><div class="bar" :style="{ width: progress + '%' }"></div></div>
    <div v-if="torrent.Loaded" class="torrent-summary" data-testid="torrent-transfers">
      <span data-testid="torrent-status" :aria-label="`${bytes(torrent.Downloaded)} verified of ${bytes(torrent.Size)}`">{{ bytes(torrent.Downloaded) }} / {{ bytes(torrent.Size) }}</span>
      <span class="torrent-rates" data-testid="torrent-rates">
        <span v-if="torrent.State === 'downloading' || torrent.DownloadRate > 0" title="Download speed" :aria-label="`Download speed: ${bytes(torrent.DownloadRate)}/s`"><Icon :path="mdiArrowDown" /> {{ bytes(torrent.DownloadRate) }}/s</span>
        <span v-if="torrent.State === 'seeding' || torrent.UploadRate > 0" title="Upload speed" :aria-label="`Upload speed: ${bytes(torrent.UploadRate)}/s`"><Icon :path="mdiArrowUp" /> {{ bytes(torrent.UploadRate) }}/s</span>
      </span>
    </div>

    <p v-if="torrent.StopReason" class="torrent-notice" data-testid="torrent-stop-reason">
      <Icon :path="mdiInformationOutline" />
      {{ torrent.StopReason === 'seed-ratio' ? 'Sharing ratio limit reached.' : torrent.StopReason === 'seed-time' ? 'Sharing time limit reached.' : 'Sharing paused: session state could not be saved.' }}
    </p>

    <dl v-if="showDetails" :id="`${panelId}-details`" class="torrent-details" data-testid="torrent-details">
      <div class="torrent-hash"><dt>Info hash</dt><dd><code>#{{ torrent.InfoHash }}</code></dd></div>
      <div><dt>Verified</dt><dd>{{ bytes(torrent.Downloaded) }}</dd></div>
      <div><dt>Uploaded</dt><dd>{{ bytes(torrent.Uploaded) }}</dd></div>
      <div><dt>Share ratio</dt><dd>{{ finiteValue(torrent.SeedRatio).toFixed(2) }}</dd></div>
      <div><dt>Seeding time</dt><dd>{{ seedDuration }}</dd></div>
      <div><dt>Peers (active / known)</dt><dd data-testid="torrent-peers">{{ finiteValue(torrent.ActivePeers) }} / {{ finiteValue(torrent.KnownPeers) }}</dd></div>
      <div title="Payload received during this connection session; resets on restart or engine reconfiguration."><dt>Received / useful</dt><dd data-testid="torrent-received">{{ bytes(torrent.NetworkDownloaded) }} / {{ bytes(torrent.UsefulDownloaded) }}</dd></div>
      <div><dt>Download speed</dt><dd>{{ bytes(torrent.DownloadRate) }}/s</dd></div>
      <div><dt>Upload speed</dt><dd>{{ bytes(torrent.UploadRate) }}/s</dd></div>
    </dl>

    <table v-if="showFiles && torrent.Loaded" :id="`${panelId}-files`" class="files" data-testid="torrent-files">
      <thead>
        <tr>
          <th class="name">File</th>
          <th class="size">Size</th>
          <th class="file-controls"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="files.length === 0">
          <td colspan="3" class="muted">No files</td>
        </tr>
        <tr v-for="f in files" :key="f.Path">
          <td class="name">
            {{ f.Path }}
            <span v-if="f.Percent > 0 && f.Percent < 100" class="percent">{{ f.Percent }}%</span>
            <div v-if="f.Percent > 0 && f.Percent < 100" class="progress active">
              <div class="bar" :style="{ width: f.Percent + '%' }"></div>
            </div>
            <p v-if="fileErrors[f.Path]" class="file-error" data-testid="file-error" role="alert">
              {{ fileErrors[f.Path] }}
            </p>
          </td>
          <td class="size">
            {{ bytes(f.Size) }}
            <Icon v-if="f.Percent === 100" class="check" :path="mdiCheck" />
          </td>
          <td class="file-controls">
            <AsyncButton
              v-if="!f.Selected"
              class="green"
              :action="() => setFile(f, 'start')"
              data-testid="file-start"
            >
              Start
            </AsyncButton>
            <span v-else>
              <span class="muted" data-testid="file-selected">Selected</span>
              <AsyncButton class="red" :action="() => setFile(f, 'stop')" data-testid="file-stop">Skip</AsyncButton>
            </span>
          </td>
        </tr>
      </tbody>
      <tfoot v-if="files.length > 1">
        <tr>
          <th class="name">{{ files.length }} Files</th>
          <th class="size">{{ bytes(torrent.Size) }} Total</th>
          <th class="file-controls"></th>
        </tr>
      </tfoot>
    </table>
    <p v-if="showFiles && torrent.Loaded" class="muted" data-testid="file-pause-help">
      Skip deselects a file without deleting it. Shared pieces and outstanding writes can still add some bytes. Skipping every file pauses the torrent. Resume keeps your selection; “Start all files” in the menu selects every file.
    </p>
  </div>
</template>
