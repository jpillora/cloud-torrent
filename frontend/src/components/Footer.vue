<script setup lang="ts">
import { computed } from 'vue'
import { state } from '../state'
import { bytes, round, ago } from '../format'

const stats = computed(() => state.Stats)
const sys = computed(() => state.Stats?.System)
const users = computed(() => Object.keys(state.Users ?? {}).length)

const cpuPct = computed(() => (sys.value ? round(sys.value.cpu) : 0))
const memPct = computed(() =>
  sys.value && sys.value.memoryTotal ? round((100 * sys.value.memoryUsed) / sys.value.memoryTotal) : 0,
)
const diskPct = computed(() =>
  sys.value && sys.value.diskTotal ? round((100 * sys.value.diskUsed) / sys.value.diskTotal) : 0,
)
</script>

<template>
  <footer class="section section-footer" data-testid="footer">
    <div class="left">
      <a href="https://github.com/jpillora/cloud-torrent" target="_blank">jpillora/cloud-torrent</a>
      <span class="sep">·</span>
      <span>v{{ stats?.Version }}</span>
      <span class="sep">·</span>
      <span><a href="https://go.dev" target="_blank">Go</a> {{ stats?.Runtime }}</span>
      <template v-if="users > 1">
        <span class="sep">·</span>
        <span>{{ users }} users</span>
      </template>
    </div>
    <div v-if="sys?.set" class="right" data-testid="footer-stats">
      <span class="dev">goroutines {{ sys.goRoutines }}</span>
      <span class="dev">gomem {{ bytes(sys.goMemory) }}</span>
      <span>cpu {{ cpuPct }}%</span>
      <span>mem {{ memPct }}%</span>
      <span>disk {{ diskPct }}%</span>
      <span v-if="stats?.Uptime">up {{ ago(stats.Uptime) }}</span>
    </div>
  </footer>
</template>
