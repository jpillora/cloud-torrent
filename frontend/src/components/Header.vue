<script setup lang="ts">
import { computed, ref } from 'vue'
import { mdiCloud, mdiGithub, mdiCog, mdiMagnet, mdiFlash, mdiLoading } from '@mdi/js'
import { state, connection } from '../state'
import { ui } from '../ui'
import { page, goHome } from '../navigation'
import Icon from './Icon.vue'

const repo = 'https://github.com/jpillora/cloud-torrent'
const title = computed(() => state.Stats?.Title || 'Cloud Torrent')

// the logo swaps from a cloud to the GitHub mark on hover.
const logoHover = ref(false)
const logo = computed(() => (logoHover.value ? mdiGithub : mdiCloud))
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
        <Icon class="logo" :path="logo" /> {{ title }}
      </a>
    </h2>
    <div class="status">
      <span
        v-show="ui.requests > 0"
        class="icon-btn"
        title="Working…"
        data-testid="status-spinner"
      >
        <Icon :path="mdiLoading" spin />
      </span>
      <a
        href="#/settings"
        class="icon-btn"
        :class="{ on: page === 'settings' }"
        :aria-current="page === 'settings' ? 'page' : undefined"
        title="Settings"
        data-testid="status-settings"
      >
        <Icon :path="mdiCog" />
      </a>
      <button
        type="button"
        class="icon-btn"
        :class="{ on: ui.omniEdit }"
        title="Magnet editor"
        data-testid="status-magnet"
        @click="goHome(); ui.omniEdit = !ui.omniEdit"
      >
        <Icon :path="mdiMagnet" />
      </button>
      <span
        class="icon-btn lightning"
        :class="{ on: connection.connected, off: !connection.connected }"
        :title="connection.connected ? 'Connected' : 'Disconnected'"
        data-testid="status-connection"
      >
        <Icon :path="mdiFlash" />
      </span>
    </div>
  </header>
</template>
