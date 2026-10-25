<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

defineProps<{ id: string; label: string; text: string }>()
const trigger = ref<HTMLButtonElement | null>(null)
const tooltip = ref<HTMLElement | null>(null)
const open = ref(false)
const pinned = ref(false)
const positioned = ref(false)
const left = ref(0)
const top = ref(0)
const width = ref(320)
let closeTimer: ReturnType<typeof setTimeout> | undefined

function cancelClose() { clearTimeout(closeTimer) }
function dismiss() {
  cancelClose()
  open.value = false
  pinned.value = false
}
function toggle() {
  cancelClose()
  pinned.value = !pinned.value
  open.value = pinned.value
}
function enter(event: PointerEvent) {
  if (event.pointerType === 'mouse') { cancelClose(); open.value = true }
}
function leave(event: PointerEvent) {
  if (event.pointerType === 'mouse' && !pinned.value) {
    cancelClose()
    // Allow the pointer to cross the small gap into the popup to read it.
    closeTimer = setTimeout(() => { if (!pinned.value) open.value = false }, 120)
  }
}
function position() {
  if (!open.value || !trigger.value || !tooltip.value) return
  width.value = Math.min(320, Math.max(0, window.innerWidth - 24))
  tooltip.value.style.width = `${width.value}px`
  const anchor = trigger.value.getBoundingClientRect()
  const height = tooltip.value.getBoundingClientRect().height
  left.value = Math.max(12, Math.min(anchor.left + anchor.width / 2 - width.value / 2, window.innerWidth - width.value - 12))
  const below = anchor.bottom + 8
  top.value = Math.max(12, Math.min(below + height <= window.innerHeight - 12 ? below : anchor.top - height - 8, window.innerHeight - height - 12))
  positioned.value = true
}
watch(open, async (visible) => {
  positioned.value = false
  if (visible) { await nextTick(); position() }
})
function outside(event: PointerEvent) {
  if (event.target instanceof Node && !trigger.value?.contains(event.target) && !tooltip.value?.contains(event.target)) dismiss()
}
function keydown(event: KeyboardEvent) { if (event.key === 'Escape') dismiss() }
onMounted(() => {
  document.addEventListener('pointerdown', outside)
  document.addEventListener('keydown', keydown)
  window.addEventListener('scroll', position, true)
  window.addEventListener('resize', position)
})
onBeforeUnmount(() => {
  cancelClose()
  document.removeEventListener('pointerdown', outside)
  document.removeEventListener('keydown', keydown)
  window.removeEventListener('scroll', position, true)
  window.removeEventListener('resize', position)
})
</script>

<template>
  <span class="setting-info" @pointerenter="enter" @pointerleave="leave">
    <button
      ref="trigger"
      type="button"
      class="info-button"
      :aria-label="`About ${label}`"
      :aria-expanded="open"
      :aria-controls="id"
      :aria-describedby="open ? id : undefined"
      @click="toggle"
      @focus="open = true"
      @blur="!pinned && (open = false)"
    ><span aria-hidden="true">i</span></button>
    <Teleport to="body">
      <p
        v-show="open"
        :id="id"
        ref="tooltip"
        role="tooltip"
        class="setting-tooltip"
        :style="{ left: `${left}px`, top: `${top}px`, width: `${width}px`, visibility: positioned ? 'visible' : 'hidden' }"
        @pointerenter="enter"
        @pointerleave="leave"
      >{{ text }}</p>
    </Teleport>
  </span>
</template>

<style scoped>
.setting-info { display: inline-flex; flex: none; }
.info-button { display: inline-flex; align-items: center; justify-content: center; width: 1.75rem; height: 1.75rem; padding: 0; border: 1px solid var(--border); border-radius: 50%; background: var(--grey-btn); color: var(--muted); font-family: Georgia, serif; font-style: italic; flex: none; }
.info-button:hover, .info-button[aria-expanded='true'] { color: var(--text); background: var(--grey-btn-hover); }
.setting-tooltip { position: fixed; z-index: 100; box-sizing: border-box; max-height: calc(100dvh - 24px); overflow: auto; margin: 0; padding: .8rem 1rem; border: 1px solid var(--border); border-radius: var(--radius); background: var(--card); color: var(--text); box-shadow: 0 5px 20px rgb(0 0 0 / 20%); font-size: .85rem; line-height: 1.5; font-weight: normal; overflow-wrap: anywhere; }
</style>
