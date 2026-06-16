<script setup lang="ts">
import { computed, ref } from 'vue'
import { state, connection } from '../state'
import { ui } from '../ui'

const repo = 'https://github.com/jpillora/cloud-torrent'
const title = computed(() => state.Stats?.Title || 'Cloud Torrent')

// emoji stand-ins for the Semantic-UI icons; the real icon font is swapped in
// during the CSS parity pass. (Glyphs live here as escapes so the template
// stays plain text, matching Omni.vue.)
const icons = {
  cloud: '\u{2601}\u{FE0F}', // logo, default
  github: '\u{1F419}', // logo, on hover (GitHub mark stand-in)
  spinner: '\u{1F504}', // request in flight
  settings: '\u{2699}\u{FE0F}', // config editor toggle
  magnet: '\u{1F9F2}', // Omni magnet editor toggle
  lightning: '\u{26A1}', // connection state
}

// the logo swaps from a cloud to a GitHub mark on hover.
const logoHover = ref(false)
const logo = computed(() => (logoHover.value ? icons.github : icons.cloud))
</script>

<template>
  <header class="section section-header title">
    <h2>
      <a
        :href="repo"
        target="_blank"
        @mouseover="logoHover = true"
        @mouseleave="logoHover = false"
      >
        <span class="logo">{{ logo }}</span> {{ title }}
      </a>
    </h2>
    <div class="status">
      <i
        v-show="ui.requests > 0"
        class="icon spinner"
        title="Working…"
        data-testid="status-spinner"
        >{{ icons.spinner }}</i
      >
      <i
        class="icon settings"
        :class="{ green: ui.configEdit }"
        title="Settings"
        data-testid="status-settings"
        @click="ui.configEdit = !ui.configEdit"
        >{{ icons.settings }}</i
      >
      <i
        class="icon magnet"
        :class="{ green: ui.omniEdit }"
        title="Magnet editor"
        data-testid="status-magnet"
        @click="ui.omniEdit = !ui.omniEdit"
        >{{ icons.magnet }}</i
      >
      <i
        class="icon lightning"
        :class="{ green: connection.connected, red: !connection.connected }"
        :title="connection.connected ? 'Connected' : 'Disconnected'"
        data-testid="status-connection"
        >{{ icons.lightning }}</i
      >
    </div>
  </header>
</template>
