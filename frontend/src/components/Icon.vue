<script setup lang="ts">
// A single Material Design Icon rendered as inline SVG. `path` is an mdi* path
// string from @mdi/js. The glyph is monochrome and inherits the current text
// colour (so .green/.red and hover states apply), and scales with font-size.
defineProps<{ path: string; spin?: boolean }>()
</script>

<template>
  <svg class="mdi" :class="{ spin }" viewBox="0 0 24 24" role="img" aria-hidden="true">
    <path :d="path" fill="currentColor" />
  </svg>
</template>
