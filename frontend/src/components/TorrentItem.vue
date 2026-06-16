<script setup lang="ts">
import { reactive, ref, computed } from 'vue'
import type { Torrent, File } from '../types/generated'
import { api } from '../api'
import { bytes } from '../format'

const props = defineProps<{ torrent: Torrent }>()
const showFiles = ref(false)

// per-file API errors, keyed by file path (e.g. Stop -> "unsupported").
const fileErrors = reactive<Record<string, string>>({})

const files = computed<File[]>(() =>
  (props.torrent.Files ?? []).filter((f): f is File => !!f),
)
// link the name to the torrent's zip download once complete
const zipUrl = computed(() =>
  props.torrent.Percent === 100 ? 'download/' + encodeURIComponent(props.torrent.Name) : '',
)

function start() {
  api.torrent('start', props.torrent.InfoHash)
}
function stop() {
  api.torrent('stop', props.torrent.InfoHash)
}
function remove() {
  api.torrent('delete', props.torrent.InfoHash)
}

// per-file start/stop. StartFile is supported; StopFile returns "unsupported"
// (master parity) — the resulting error is surfaced inline on the file row.
async function setFile(action: 'start' | 'stop', f: File) {
  delete fileErrors[f.Path]
  try {
    await api.file(action, props.torrent.InfoHash, f.Path)
  } catch (e) {
    fileErrors[f.Path] = e instanceof Error ? e.message : String(e)
  }
}

function filename(path: string): string {
  const m = /\/([^/]+)$/.exec(path)
  return m ? m[1] : path
}
</script>

<template>
  <div class="torrent" :class="{ loading: !torrent.Loaded }" data-testid="torrent">
    <div v-if="!torrent.Loaded" class="dimmer">Loading…</div>

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
        <button v-if="!torrent.Started" class="green" @click="start" data-testid="torrent-start">
          Start
        </button>
        <button v-else class="red" @click="stop" data-testid="torrent-stop">Stop</button>
        <button v-if="!torrent.Started" class="red" @click="remove" data-testid="torrent-remove">
          {{ torrent.Loaded ? 'Remove' : 'Cancel' }}
        </button>
      </div>
    </div>

    <div v-if="torrent.Started" class="status" data-testid="torrent-status">
      <span :class="{ muted: torrent.Downloaded === 0 }">{{ bytes(torrent.Downloaded) }}</span>
      / {{ bytes(torrent.Size) }} — {{ torrent.Percent }}%
      <span :class="{ muted: torrent.DownloadRate === 0 }"> — {{ bytes(torrent.DownloadRate) }}/s</span>
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
            {{ filename(f.Path) }}
            <span v-if="f.Percent > 0 && f.Percent < 100" class="percent">{{ f.Percent }}%</span>
            <div v-if="f.Percent > 0 && f.Percent < 100" class="progress active">
              <div class="bar" :style="{ width: f.Percent + '%' }"></div>
            </div>
            <p v-if="fileErrors[f.Path]" class="file-error" data-testid="file-error">
              {{ fileErrors[f.Path] }}
            </p>
          </td>
          <td class="size">
            {{ bytes(f.Size) }}
            <span v-if="f.Percent === 100" class="check">✓</span>
          </td>
          <td class="file-controls">
            <button v-if="!f.Started" class="green" @click="setFile('start', f)" data-testid="file-start">
              Start
            </button>
            <button v-else class="red" @click="setFile('stop', f)" data-testid="file-stop">Stop</button>
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
  </div>
</template>
