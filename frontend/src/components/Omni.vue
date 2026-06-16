<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { api } from '../api'
import { ui } from '../ui'
import MagnetEditor from './MagnetEditor.vue'

type Mode = 'url' | 'magnet' | 'search' | 'upload'

const input = ref(localStorage.getItem('tcOmni') ?? '')
const omnierr = ref('')
const busy = ref(false)

// live mode detection from the input
const mode = computed<Mode>(() => {
  const v = input.value.trim()
  if (/^https?:\/\//.test(v)) return 'url'
  if (/^magnet:\?/.test(v)) return 'magnet'
  if (v) return 'search'
  return 'upload'
})

// trailing indicator reflecting the mode (proper icons land with the CSS pass)
const icon = computed(() => {
  switch (mode.value) {
    case 'search':
      return '\u{1F50D}' // magnifier
    case 'magnet':
    case 'url':
      return '\u{1F9F2}' // magnet
    default:
      return '\u{2B06}' // upload
  }
})

watch(input, (v) => {
  localStorage.setItem('tcOmni', v)
  omnierr.value = ''
})

async function submit() {
  omnierr.value = ''
  busy.value = true
  try {
    const v = input.value.trim()
    if (mode.value === 'url') await api.url(v)
    else if (mode.value === 'magnet') await api.magnet(v)
    // search mode is handled by the search task
  } catch (e) {
    omnierr.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="section section-omni">
    <MagnetEditor v-if="ui.omniEdit" v-model="input" @error="omnierr = $event" />

    <div class="omni-input-wrap">
      <input
        class="omni-input"
        v-model="input"
        placeholder="Enter search query, magnet URI, torrent URL or drop a torrent file here"
        @keyup.enter="submit"
        data-testid="omni-input"
      />
      <span class="omni-icon" :title="mode">{{ icon }}</span>
    </div>

    <p v-if="omnierr" class="omni-error" data-testid="omni-error">{{ omnierr }}</p>

    <div class="omni-actions">
      <button
        v-if="mode === 'url'"
        :disabled="busy"
        @click="submit"
        data-testid="omni-start"
      >
        Start Torrent
      </button>
      <template v-else-if="mode === 'magnet'">
        <button :disabled="busy" @click="submit" data-testid="omni-load">Load Magnet</button>
        <button :class="{ active: ui.omniEdit }" @click="ui.omniEdit = !ui.omniEdit">Edit</button>
      </template>
    </div>
  </section>
</template>
