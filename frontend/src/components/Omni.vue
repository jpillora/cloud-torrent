<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { mdiMagnify, mdiMagnet, mdiUpload, mdiDownload } from '@mdi/js'
import { api, search, type SearchResult } from '../api'
import { state } from '../state'
import { ui } from '../ui'
import { uploadTorrentFiles } from '../upload'
import AsyncButton from './AsyncButton.vue'
import Icon from './Icon.vue'
import MagnetEditor from './MagnetEditor.vue'

type Mode = 'url' | 'magnet' | 'search' | 'upload'
interface Provider {
  name: string
  url: string
}

const input = ref(localStorage.getItem('tcOmni') ?? '')
const omnierr = ref('')
const busy = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

// live mode detection from the input
const mode = computed<Mode>(() => {
  const v = input.value.trim()
  if (/^https?:\/\//.test(v)) return 'url'
  if (/^magnet:\?/.test(v)) return 'magnet'
  if (v) return 'search'
  return 'upload'
})

// --- search mode ---------------------------------------------------------
// providers come from the synced state; the "<id>/item" detail handlers are
// excluded from the picker.
const providers = computed<Record<string, Provider>>(() => {
  const sp = (state.SearchProviders ?? {}) as Record<string, Provider>
  const out: Record<string, Provider> = {}
  for (const id in sp) {
    if (!/\/item$/.test(id)) out[id] = sp[id]
  }
  return out
})
const providerIds = computed(() => Object.keys(providers.value))

const provider = ref(localStorage.getItem('tcProvider') ?? '')
type ProviderResult = SearchResult & { providerId: string }
const results = ref<ProviderResult[]>([])
const generation = ref(0)
const page = ref(1)
const hasMore = ref(true)
const noResults = ref(false)
const searching = ref(false)

function resetSearch() {
  generation.value++
  searching.value = false
  omnierr.value = ''
  results.value = []
  page.value = 1
  hasMore.value = true
  noResults.value = false
}

// keep a valid provider selected: default to the first once providers load (or
// if the persisted one is no longer offered).
watch(
  providerIds,
  (ids) => {
    if (ids.length && !ids.includes(provider.value)) provider.value = ids[0]
  },
  { immediate: true },
)
watch(provider, (p) => {
  if (p) localStorage.setItem('tcProvider', p)
  resetSearch()
})

// run (or page through) a provider search; results accumulate across pages.
async function submitSearch() {
  if (searching.value) return
  const providerId = provider.value
  const query = input.value.trim()
  const requestPage = page.value
  const requestGeneration = generation.value
  const prov = providers.value[providerId]
  if (!prov) return
  const origin = /(https?:\/\/[^/]+)/.exec(prov.url)?.[1] ?? ''
  omnierr.value = ''
  searching.value = true
  try {
    const rows = await search.all(providerId, query, requestPage)
    if (requestGeneration !== generation.value) return
    if (!rows || rows.length === 0) {
      noResults.value = true
      hasMore.value = false
      return
    }
    for (const r of rows) {
      // provider-relative links are resolved against the provider origin
      if (r.url && r.url.startsWith('/')) {
        r.path = r.url
        r.url = origin + r.url
      }
      if (r.torrent && r.torrent.startsWith('/')) r.torrent = origin + r.torrent
      results.value.push({ ...r, providerId })
    }
    page.value = requestPage + 1
  } catch (e) {
    if (requestGeneration === generation.value) omnierr.value = e instanceof Error ? e.message : String(e)
  } finally {
    if (requestGeneration === generation.value) searching.value = false
  }
}

// build a magnet URI from a resolved infohash (master parity: dn is stripped of
// non-word characters).
function magnetURI(name: string, infohash: string, trackers: string[]): string {
  let uri = 'magnet:?xt=urn:btih:' + infohash + '&dn=' + name.replace(/\W/g, '').replace(/\s+/g, '+')
  for (const t of trackers) uri += '&tr=' + encodeURIComponent(t)
  return uri
}

// add a result: use its magnet/torrent if present, else resolve via the
// provider's "<id>/item" detail handler.
async function submitSearchItem(r: ProviderResult) {
  const requestGeneration = generation.value
  omnierr.value = ''
  try {
    if (r.magnet) return await api.magnet(r.magnet)
    if (r.torrent) return await api.url(r.torrent)
    if (!r.path) {
      omnierr.value = 'No item URL found'
      return
    }
    const data = await search.one(r.providerId, r.path)
    if (requestGeneration !== generation.value) return
    if (!data) {
      omnierr.value = 'No response'
      return
    }
    if (data.torrent) return await api.url(data.torrent)
    if (data.magnet) return await api.magnet(data.magnet)
    if (data.infohash) {
      const trackers = (data.tracker ?? '').split(',').filter((s) => /^(http|udp):\/\//.test(s))
      return await api.magnet(magnetURI(r.name ?? '', data.infohash, trackers))
    }
    omnierr.value = 'No magnet or infohash found'
  } catch (e) {
    if (requestGeneration === generation.value) omnierr.value = e instanceof Error ? e.message : String(e)
  }
}

// trailing indicator reflecting the current mode
const modeIcon = computed(() => {
  switch (mode.value) {
    case 'search':
      return mdiMagnify
    case 'magnet':
    case 'url':
      return mdiMagnet
    default:
      return mdiUpload
  }
})

watch(input, (v) => {
  localStorage.setItem('tcOmni', v)
  omnierr.value = ''
  resetSearch() // a new query starts pagination over
})

async function submit() {
  if (busy.value) return
  if (mode.value === 'search') return submitSearch()
  omnierr.value = ''
  busy.value = true
  try {
    const v = input.value.trim()
    if (mode.value === 'url') await api.url(v)
    else if (mode.value === 'magnet') await api.magnet(v)
  } catch (e) {
    omnierr.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}

// clicking the Omni icon opens the hidden file picker; chosen .torrent files
// take the same upload path as the full-page drop cover (master parity: the
// icon triggers an upload in every mode).
function pickFiles() {
  fileInput.value?.click()
}

async function onFiles(e: Event) {
  const el = e.target as HTMLInputElement
  if (!el.files) return
  omnierr.value = ''
  try {
    await uploadTorrentFiles(el.files)
  } catch (err) {
    omnierr.value = err instanceof Error ? err.message : String(err)
  } finally {
    el.value = '' // allow re-selecting the same file
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
      <button
        type="button"
        class="omni-icon"
        :title="`Upload .torrent file (${mode})`"
        aria-label="Upload torrent files"
        @click="pickFiles"
        data-testid="omni-upload"
      >
        <Icon :path="modeIcon" />
      </button>
      <input
        ref="fileInput"
        type="file"
        multiple
        accept=".torrent"
        class="hidden-file"
        data-testid="omni-file-input"
        @change="onFiles"
      />
    </div>

    <p v-if="omnierr" class="omni-error" data-testid="omni-error" role="alert">{{ omnierr }}</p>

    <div class="omni-actions">
      <AsyncButton
        v-if="mode === 'url'"
        :action="submit"
        :disabled="busy"
        data-testid="omni-start"
      >
        Start Torrent
      </AsyncButton>
      <template v-else-if="mode === 'magnet'">
        <AsyncButton :action="submit" :disabled="busy" data-testid="omni-load">Load Magnet</AsyncButton>
        <button :class="{ active: ui.omniEdit }" @click="ui.omniEdit = !ui.omniEdit">Edit</button>
      </template>
      <template v-else-if="mode === 'search'">
        <template v-if="providerIds.length">
          <select v-model="provider" data-testid="search-provider">
            <option v-for="id in providerIds" :key="id" :value="id">{{ providers[id].name }}</option>
          </select>
          <AsyncButton
            :key="generation"
            :action="submitSearch"
            :disabled="searching || noResults || (hasMore && results.length > 0)"
            data-testid="search-submit"
          >
            {{ noResults ? 'No results!' : 'Search' }}
          </AsyncButton>
        </template>
        <span v-else class="muted" data-testid="search-no-providers">You have no search providers</span>
      </template>
    </div>

    <table
      v-if="mode === 'search' && results.length"
      class="search-results"
      data-testid="search-results"
    >
      <tbody>
        <tr v-for="(r, i) in results" :key="i" data-testid="search-result">
          <td class="name">
            <a v-if="r.url" :href="r.url" target="_blank">{{ r.name }}</a>
            <span v-else>{{ r.name }}</span>
          </td>
          <td v-if="r.size" class="size">{{ r.size }}</td>
          <td class="users">
            <span class="seeds">{{ r.seeds }}</span>
            <span class="peers">{{ r.peers }}</span>
          </td>
          <td class="controls">
            <AsyncButton
              class="green download"
              title="Download"
              data-testid="search-download"
              :action="() => submitSearchItem(r)"
            >
              <Icon :path="mdiDownload" />
            </AsyncButton>
          </td>
        </tr>
        <tr v-if="hasMore">
          <td colspan="10" class="loadmore">
            <AsyncButton :disabled="searching" :key="generation"
            :action="submitSearch" data-testid="search-loadmore">
              Load more
            </AsyncButton>
          </td>
        </tr>
      </tbody>
    </table>
  </section>
</template>
