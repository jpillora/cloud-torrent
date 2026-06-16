<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [string]; error: [string] }>()

const name = ref('')
const infohash = ref('')
const trackers = ref<string[]>([''])

// parse the current magnet into fields once, when the editor opens
{
  const m = /^magnet:\?(.+)$/.exec(props.modelValue)
  if (m) {
    const params = new URLSearchParams(m[1])
    const ih = /^urn:btih:([A-Za-z0-9]+)$/.exec(params.get('xt') ?? '')
    infohash.value = ih ? ih[1] : ''
    name.value = params.get('dn') ?? ''
    trackers.value = [...params.getAll('tr'), '']
  }
}

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
      <input v-model="name" @input="onEdit" placeholder="Name" />
    </label>
    <label class="field">
      <span>Info Hash</span>
      <input v-model="infohash" @input="onEdit" placeholder="Info Hash" data-testid="magnet-infohash" />
    </label>
    <div class="field">
      <span>Trackers</span>
      <input
        v-for="(_, i) in trackers"
        :key="i"
        v-model="trackers[i]"
        @input="onEdit"
        placeholder="Tracker"
      />
    </div>
  </form>
</template>
