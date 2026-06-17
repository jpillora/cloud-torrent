<script setup lang="ts">
import { computed } from 'vue'
import type { Torrent } from '../types/generated'
import { state } from '../state'
import TorrentItem from './TorrentItem.vue'

const torrents = computed(() =>
  Object.values(state.Torrents ?? {}).filter((t): t is Torrent => !!t),
)
const count = computed(() => torrents.value.length)
</script>

<template>
  <section class="section section-torrents">
    <div class="section-header">
      <h3>Torrents</h3>
      <span class="count">{{ count }} torrent{{ count === 1 ? '' : 's' }}</span>
    </div>
    <p v-if="count === 0" class="empty" data-testid="torrents-empty">Add torrents above</p>
    <TorrentItem v-for="t in torrents" :key="t.InfoHash" :torrent="t" />
  </section>
</template>
