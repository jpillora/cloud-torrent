<script setup lang="ts">
import { onBeforeUnmount, ref, shallowRef } from 'vue'
import { mdiDownload } from '@mdi/js'
import { ui } from '../ui'
import Icon from './Icon.vue'

const props = defineProps<{
  action: (signal: AbortSignal) => Promise<void>
  name: string
}>()

const pending = shallowRef<AbortController | null>(null)
const canCancel = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

function cancel() {
  clearTimeout(timer)
  pending.value?.abort()
  pending.value = null
  canCancel.value = false
}
onBeforeUnmount(cancel)

async function onClick() {
  if (pending.value) {
    if (canCancel.value) cancel()
    return
  }
  const request = new AbortController()
  pending.value = request
  canCancel.value = false
  timer = setTimeout(() => { canCancel.value = true }, 5000)
  try {
    await props.action(request.signal)
  } catch (error) {
    if (!request.signal.aborted) ui.error = error instanceof Error ? error.message : String(error)
  } finally {
    // A cancelled request may finish after the user has already retried.
    if (pending.value === request) {
      clearTimeout(timer)
      pending.value = null
      canCancel.value = false
    }
  }
}
</script>

<template>
  <button
    type="button"
    class="download"
    :class="{ loading: pending }"
    :disabled="!!pending && !canCancel"
    :aria-label="pending ? (canCancel ? `Cancel download of ${name}` : `Loading ${name}`) : `Download ${name}`"
    :title="pending ? (canCancel ? 'Cancel download' : 'Loading… Cancel available after 5 seconds') : 'Download'"
    data-testid="search-download"
    @click="onClick"
  >
    <span v-if="pending" class="btn-spinner" aria-hidden="true"></span>
    <Icon v-else :path="mdiDownload" />
  </button>
</template>
