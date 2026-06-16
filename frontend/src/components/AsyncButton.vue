<script setup lang="ts">
import { ref } from 'vue'
import { ui } from '../ui'

// A button bound to an async action: while the action runs it disables itself
// and shows a spinner (master's `loading`/`apiing` button behaviour). Each
// rendered instance tracks its own state, so it works inside v-for lists too.
// Uncaught action errors surface in the global error banner.
const props = defineProps<{
  action: () => unknown | Promise<unknown>
  disabled?: boolean
}>()

const running = ref(false)
async function onClick() {
  if (running.value || props.disabled) return
  running.value = true
  try {
    await props.action()
  } catch (e) {
    ui.error = e instanceof Error ? e.message : String(e)
  } finally {
    running.value = false
  }
}
</script>

<template>
  <button type="button" :disabled="disabled || running" :class="{ loading: running }" @click="onClick">
    <span v-if="running" class="btn-spinner" aria-hidden="true"></span>
    <slot />
  </button>
</template>
