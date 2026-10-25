<script setup lang="ts">
import { ref } from 'vue'

defineProps<{ id: string; label: string; text: string }>()
const open = ref(false)
const pinned = ref(false)
function toggle() {
  pinned.value = !pinned.value
  open.value = pinned.value
}
function enter(event: PointerEvent) {
  if (event.pointerType === 'mouse') open.value = true
}
function leave(event: PointerEvent) {
  if (event.pointerType === 'mouse' && !pinned.value) open.value = false
}
function dismiss() { open.value = false; pinned.value = false }
</script>

<template>
  <span class="setting-info" @pointerenter="enter" @pointerleave="leave" @keydown.esc.stop="dismiss">
    <button
      type="button"
      class="info-button"
      :aria-label="`About ${label}`"
      :aria-expanded="open"
      :aria-controls="id"
      @click="toggle"
      @focus="open = true"
      @blur="!pinned && (open = false)"
    ><span aria-hidden="true">i</span></button>
    <p v-show="open" :id="id" class="setting-explanation">{{ text }}</p>
  </span>
</template>

<style scoped>
.setting-info { display: contents; }
.info-button { display: inline-flex; align-items: center; justify-content: center; width: 1.75rem; height: 1.75rem; padding: 0; border: 1px solid var(--border); border-radius: 50%; background: var(--grey-btn); color: var(--muted); font-family: Georgia, serif; font-style: italic; flex: none; }
.info-button:hover, .info-button[aria-expanded='true'] { color: var(--text); background: var(--grey-btn-hover); }
.setting-explanation { flex: 0 0 100%; margin: .15rem 0 .35rem; color: var(--muted); font-size: .85rem; line-height: 1.5; font-weight: normal; }
</style>
