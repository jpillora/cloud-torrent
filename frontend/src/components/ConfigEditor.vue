<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { state } from '../state'
import { api } from '../api'
import type { Config } from '../types/generated'
import AsyncButton from './AsyncButton.vue'
import SettingHelp from './SettingHelp.vue'
import { themePreference } from '../theme'
import { goHome } from '../navigation'

// Seed this page's editable copy once, including when a direct settings link
// loads before realtime state. Later sync updates must preserve unsaved edits.
const form = reactive<Partial<Config>>({})
const error = ref('')
const saved = ref(false)
const initialized = ref(false)
watch(() => state.Config, (config) => {
  if (!initialized.value && config) {
    Object.assign(form, config)
    initialized.value = true
  }
}, { immediate: true })

const settings: Record<keyof Config, { label: string; help: string }> = {
  SeedRatioLimit: { label: 'Seeding ratio limit', help: 'Stop sharing when uploaded bytes reach this multiple of the selected content size. Existing verified content counts toward that size. Set 0 to disable. Either the ratio or time limit can stop sharing; Resume sharing overrides both until Stop.' },
  SeedTimeMinutes: { label: 'Seeding time limit (minutes)', help: 'Stop sharing after this many minutes spent actively seeding. Paused time is excluded and the total survives restarts. Set 0 to disable. Either the ratio or time limit can stop sharing.' },
  MaxActiveDownloads: { label: 'Active downloads', help: 'Maximum simultaneous downloads, from 1 to 256. Set 0 for unlimited. Waiting torrents start in queue order; completed selections can seed without using a download slot. At most 256 torrents can be admitted in total.' },
  MaxActiveMetadata: { label: 'Metadata concurrency', help: 'Maximum simultaneous magnet metadata fetches, up to 64. Set 0 to use four slots. Additional magnets wait in the queue without starting a metadata connection.' },
  DownloadLimitKiB: { label: 'Download limit (KiB/s)', help: 'Shared download payload limit across all torrents, including webseeds. One KiB is 1024 bytes. Set 0 for unlimited; a short 64 KiB burst is allowed. Changes apply without restarting downloads.' },
  UploadLimitKiB: { label: 'Upload limit (KiB/s)', help: 'Shared upload payload limit across all torrents. One KiB is 1024 bytes. Set 0 for unlimited; a short 64 KiB burst is allowed. Changes apply without restarting torrents.' },
  AutoStart: { label: 'Auto start', help: 'Automatically start newly added torrents when queue capacity is available. Turning this on does not resume torrents you have explicitly paused.' },
  DisableEncryption: { label: 'Disable encryption', help: 'Require plaintext peer transport. When off, encrypted transport is preferred with plaintext fallback. Peer transport encryption does not provide anonymity.' },
  DownloadDirectory: { label: 'Download directory', help: 'Server filesystem directory used for torrent data and served downloads. It must not overlap private session state or the import inbox. Changing it recreates the torrent client and may require checking files in the new location.' },
  EnableUpload: { label: 'Enable upload', help: 'Allow sending torrent payload to peers while downloading. Turn off to disable payload uploads, including seeding.' },
  EnableSeeding: { label: 'Enable seeding', help: 'Continue sharing selected content after it finishes downloading. Enable upload must also be on. Optional ratio and active-time limits can stop sharing automatically.' },
  IncomingPort: { label: 'Incoming port', help: 'Port on the server for incoming BitTorrent peer connections. Container or router port mappings must allow this port for remote peers to reach it. This does not change the web interface port.' },
}

async function save() {
  error.value = ''
  saved.value = false
  try {
    await api.configure(form as Config)
    saved.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
}
</script>

<template>
  <main class="section section-config" data-testid="config-editor" aria-labelledby="settings-title">
    <div class="settings-heading">
      <h3 id="settings-title" tabindex="-1">Settings</h3>
      <a href="#/" data-testid="settings-back">← Back to torrents</a>
    </div>
    <div class="config-form appearance-settings">
      <h4>Appearance</h4>
      <div class="field">
        <div class="setting-label">
          <label for="theme-preference">Theme</label>
          <SettingHelp id="theme-help" label="Theme" text="Applies immediately in this browser. System follows your device’s appearance. This preference is saved per browser, separately from server settings." />
        </div>
        <select id="theme-preference" v-model="themePreference" data-testid="theme-preference" aria-describedby="theme-help">
          <option value="system">System</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </div>
    </div>
    <form v-if="initialized" class="config-form" @submit.prevent="save" @input="saved = false">
      <h4>Torrent configuration</h4>
      <div v-for="(definition, key) in settings" :key="key" class="field" :class="{ 'field-toggle': typeof form[key] === 'boolean' }">
        <div class="setting-label">
          <label :for="`config-${key}`">{{ definition.label }}</label>
          <SettingHelp :id="`help-${key}`" :label="definition.label" :text="definition.help" />
        </div>
        <input v-if="typeof form[key] === 'boolean'" :id="`config-${key}`" v-model="form[key]" type="checkbox" :data-testid="`config-${key}`" :aria-describedby="`help-${key}`" />
        <input
          v-else-if="typeof form[key] === 'number'"
          :id="`config-${key}`"
          type="number"
          :min="key.endsWith('LimitKiB') || key.startsWith('Seed') || key.startsWith('MaxActive') ? 0 : undefined"
          :max="key.endsWith('LimitKiB') ? 1073741824 : key === 'MaxActiveDownloads' ? 256 : key === 'MaxActiveMetadata' ? 64 : undefined"
          :step="key === 'SeedRatioLimit' ? 'any' : 1"
          v-model.number="form[key]"
          :data-testid="`config-${key}`"
          :aria-describedby="`help-${key}`"
        />
        <input v-else :id="`config-${key}`" v-model="form[key]" type="text" :data-testid="`config-${key}`" :aria-describedby="`help-${key}`" />
      </div>
      <p v-if="error" class="config-error" data-testid="config-error" role="alert">{{ error }}</p>
      <p v-if="saved" data-testid="config-saved" role="status">Settings saved.</p>
      <div class="buttons">
        <AsyncButton class="blue" :action="save" data-testid="config-save">Save</AsyncButton>
        <button type="button" @click="goHome" data-testid="config-cancel">Cancel</button>
      </div>
    </form>
  </main>
</template>

<style scoped>
.settings-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: .5rem; margin-bottom: 1.5rem; }
.settings-heading h3 { margin: 0; }
.setting-label { display: flex; flex-wrap: wrap; align-items: center; gap: .25rem .4rem; }
.setting-label label { font-weight: 700; font-size: .9rem; }
.field-toggle { justify-content: space-between; }
.field-toggle .setting-label { flex: 1; min-width: 0; }
</style>
