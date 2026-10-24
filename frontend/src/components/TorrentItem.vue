<script setup lang="ts">
import { reactive, ref, computed } from 'vue'
import { mdiCheck } from '@mdi/js'
import type { Torrent, File } from '../types/generated'
import { api } from '../api'
import { bytes } from '../format'
import AsyncButton from './AsyncButton.vue'
import Icon from './Icon.vue'

const props = defineProps<{ torrent: Torrent }>()
const showFiles = ref(false)
const stateLabel = computed(() => ({
  queued: 'Queued', metadata: 'Fetching metadata', downloading: 'Downloading',
  seeding: 'Seeding', paused: 'Paused', completed: 'Completed',
  'stopped-by-policy': 'Stopped by policy',
}[props.torrent.State] ?? 'Waiting'))
function finiteValue(value: number | undefined): number {
  return value !== undefined && Number.isFinite(value) && value >= 0 ? value : 0
}

// Per-file selection errors, keyed by file path.
const fileErrors = reactive<Record<string, string>>({})

const files = computed<File[]>(() =>
  (props.torrent.Files ?? []).filter((f): f is File => !!f),
)
// link the name to the torrent's zip download once complete
const zipUrl = computed(() =>
  props.torrent.Percent === 100 ? 'download/' + encodeURIComponent(props.torrent.Name) : '',
)

// return the promise so AsyncButton can show its loading state until the
// request settles (failures surface in the global error banner)
function start() {
  return api.torrent(files.value.some(f => f.Selected) ? 'resume' : 'start', props.torrent.InfoHash)
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
    <div class="status" data-testid="torrent-queue-state">
      {{ stateLabel }}<span v-if="torrent.QueuePosition"> · Queue position {{ torrent.QueuePosition }}</span>
    </div>

    <p v-if="torrent.StopReason" class="status" data-testid="torrent-stop-reason">
      {{ torrent.StopReason === 'seed-ratio' ? 'Stopped at the seeding ratio limit.' : torrent.StopReason === 'seed-time' ? 'Stopped at the seeding time limit.' : 'Sharing paused: session state could not be saved.' }}
    </p>
    <p v-if="torrent.SeedingOverride" class="status">Sharing limits overridden until you stop this torrent.</p>
    <div class="row">
      <div class="info">
        <div class="name" data-testid="torrent-name">
          <a v-if="zipUrl" :href="zipUrl">{{ torrent.Name }}</a>
          <span v-else>{{ torrent.Name }}</span>
        </div>
        <div class="hash">#{{ torrent.InfoHash }}</div>
        <div class="progress" :class="{ active: torrent.Percent > 0 && torrent.Percent < 100 }">
          <div class="bar" :style="{ width: torrent.Percent + '%' }"></div>
        </div>
      </div>

      <div class="controls">
        <button :class="{ active: showFiles }" @click="showFiles = !showFiles">Files</button>
        <AsyncButton
          v-if="torrent.StopReason !== 'seed-ratio' && torrent.StopReason !== 'seed-time' && torrent.State !== 'queued' && !torrent.Started"
          class="green"
          :action="start"
          data-testid="torrent-start"
        >
          {{ files.some(f => f.Selected) ? 'Resume selected' : 'Start all' }}
        </AsyncButton>
        <AsyncButton v-if="torrent.Loaded && files.some(f => f.Selected) && files.some(f => !f.Selected) && !torrent.StopReason" class="green" :action="startAll" data-testid="torrent-start-all">Start all</AsyncButton>
        <AsyncButton v-if="torrent.StopReason === 'seed-ratio' || torrent.StopReason === 'seed-time'" class="green" :action="resumeSeeding" data-testid="torrent-resume-seeding">Resume sharing (ignore limits)</AsyncButton>
        <AsyncButton v-if="torrent.Started || torrent.State === 'queued' || torrent.State === 'metadata'" class="red" :action="stop" data-testid="torrent-stop">
          Stop
        </AsyncButton>
        <AsyncButton v-if="!torrent.Started" class="red" :action="remove" data-testid="torrent-remove">
          {{ torrent.Loaded ? 'Remove' : 'Cancel' }}
        </AsyncButton>
      </div>
    </div>

    <div v-if="torrent.Loaded" class="status" data-testid="torrent-status">
      Verified <span :class="{ muted: torrent.Downloaded === 0 }">{{ bytes(torrent.Downloaded) }}</span>
      / {{ bytes(torrent.Size) }} — {{ torrent.Percent }}%
      <span> · Uploaded {{ bytes(torrent.Uploaded ?? 0) }} · Ratio {{ finiteValue(torrent.SeedRatio).toFixed(2) }} · Seeding {{ torrent.SeedSeconds }}s</span>
    </div>

    <div class="transfer-summary" data-testid="torrent-transfers" aria-label="Transfer activity">
      <span data-testid="torrent-rates">↓ {{ bytes(torrent.DownloadRate) }}/s · ↑ {{ bytes(torrent.UploadRate) }}/s</span>
      <span data-testid="torrent-peers">Peers {{ finiteValue(torrent.ActivePeers) }} active / {{ finiteValue(torrent.KnownPeers) }} known</span>
      <span data-testid="torrent-received" title="Payload received during this torrent connection session; resets on restart or engine reconfiguration.">Received {{ bytes(torrent.NetworkDownloaded) }} ({{ bytes(torrent.UsefulDownloaded) }} useful)</span>
    </div>

    <table v-if="showFiles && torrent.Loaded" class="files" data-testid="torrent-files">
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
      Skip deselects a file without deleting it. Shared pieces and outstanding writes can still add some bytes. Skipping every file pauses the torrent. Resume selected keeps your choices; Start all selects every file.
    </p>
  </div>
</template>

<style scoped>
.transfer-summary { display: flex; flex-wrap: wrap; gap: .25rem 1rem; margin: .5rem 0; font-size: .85rem; }
.transfer-summary span { max-width: 100%; overflow-wrap: anywhere; }
</style>
