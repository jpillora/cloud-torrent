<script setup lang="ts">
import { computed } from 'vue'
import type { FsNode } from '../types/generated'
import { state } from '../state'
import { bytes } from '../format'
import FileNode from './FileNode.vue'

const children = computed<FsNode[]>(() =>
  (state.Downloads?.Children ?? []).filter((c): c is FsNode => !!c),
)
const free = computed(() => {
  const s = state.Stats?.System
  return s && s.diskTotal ? bytes(s.diskTotal - s.diskUsed) : ''
})
</script>

<template>
  <section class="section section-downloads">
    <div class="section-header">
      <h3>Downloads</h3>
      <span v-if="free" class="free">{{ free }} free</span>
    </div>
    <p v-if="children.length === 0" class="muted" data-testid="downloads-empty">
      Download files above
    </p>
    <div v-else class="tree">
      <FileNode v-for="c in children" :key="c.Name" :node="c" />
    </div>
  </section>
</template>
