<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [string]; error: [string] }>()

const name = ref('')
const infohash = ref('')
const trackers = ref<string[]>([''])

// External Omni edits replace the fields; our own emissions preserve the
// current field text and cursor (including the trailing blank tracker).
let lastEmitted: string | undefined
watch(() => props.modelValue, (value) => {
  if (value === lastEmitted) {
    lastEmitted = undefined
    return
  }
  lastEmitted = undefined
  const m = /^magnet:\?(.+)$/.exec(value)
  const params = new URLSearchParams(m?.[1] ?? '')
  const ih = /^urn:btih:([A-Za-z0-9]+)$/.exec(params.get('xt') ?? '')
  infohash.value = ih ? ih[1] : ''
  name.value = params.get('dn') ?? ''
  trackers.value = [...params.getAll('tr'), '']
}, { immediate: true })

// rebuild the magnet URI from the fields and push it up
function onEdit() {
  if (infohash.value && !/^[A-Za-z0-9]+$/.test(infohash.value)) {
    emit('error', 'Invalid Info Hash')
    return
  }
  let uri = 'magnet:?xt=urn:btih:' + infohash.value
  if (name.value) uri += '&dn=' + encodeURIComponent(name.value)
  for (const t of trackers.value) {
    if (t.trim()) uri += '&tr=' + encodeURIComponent(t.trim())
  }
  lastEmitted = uri
  emit('update:modelValue', uri)
  // keep a trailing empty tracker field for adding more
  if (trackers.value[trackers.value.length - 1].trim() !== '') trackers.value.push('')
}
</script>

<template>
  <form class="magnet-editor" @submit.prevent>
    <h4>Magnet URI Editor</h4>
    <label class="field">
      <span>Name</span>
      <input type="text" v-model="name" @input="onEdit" placeholder="Name" />
    </label>
    <label class="field">
      <span>Info Hash</span>
      <input type="text" v-model="infohash" @input="onEdit" placeholder="Info Hash" data-testid="magnet-infohash" />
    </label>
    <div class="field">
      <span>Trackers</span>
      <input
        type="text"
        v-for="(_, i) in trackers"
        :key="i"
        v-model="trackers[i]"
        @input="onEdit"
        placeholder="Tracker"
      />
    </div>
  </form>
</template>
