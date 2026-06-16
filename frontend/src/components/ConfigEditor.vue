<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { state } from '../state'
import { ui } from '../ui'
import { api } from '../api'
import type { Config } from '../types/generated'
import AsyncButton from './AsyncButton.vue'

// A local, editable copy of the synced Config. Editing a copy (rather than
// state.Config directly) keeps an incoming velox push from clobbering an
// in-progress edit. It is (re)seeded from state.Config each time the editor
// opens. Values are loosely typed so the dynamic v-model below stays simple;
// the shape still matches Config because it is copied from it.
const form = reactive<Record<string, string | number | boolean>>({})
const error = ref('')

watch(
  () => ui.configEdit,
  (open) => {
    if (open && state.Config) {
      error.value = ''
      Object.assign(form, state.Config)
    }
  },
  { immediate: true },
)

// "DownloadDirectory" -> "Download Directory" (master's addspaces filter).
function addSpaces(s: string): string {
  return s.replace(/([A-Z]+[a-z]*)/g, (_, w) => ' ' + w).replace(/^ /, '')
}

async function save() {
  error.value = ''
  try {
    await api.configure(form as unknown as Config)
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
}

function cancel() {
  ui.configEdit = false
}
</script>

<template>
  <section v-if="ui.configEdit" class="section section-config" data-testid="config-editor">
    <form class="config-form" @submit.prevent="save">
      <h4>Configuration</h4>
      <template v-for="(v, k) in form" :key="k">
        <!-- booleans render as a toggle, everything else as a labelled input -->
        <label v-if="typeof v === 'boolean'" class="field field-toggle">
          <input type="checkbox" v-model="form[k]" :data-testid="`config-${k}`" />
          <span>{{ addSpaces(k) }}</span>
        </label>
        <label v-else class="field">
          <span>{{ addSpaces(k) }}</span>
          <input
            v-if="typeof v === 'number'"
            type="number"
            v-model.number="form[k]"
            :data-testid="`config-${k}`"
          />
          <input v-else type="text" v-model="form[k]" :data-testid="`config-${k}`" />
        </label>
      </template>

      <p v-if="error" class="config-error" data-testid="config-error">{{ error }}</p>

      <div class="buttons">
        <AsyncButton class="blue" :action="save" data-testid="config-save">Save</AsyncButton>
        <button type="button" @click="cancel" data-testid="config-cancel">Cancel</button>
      </div>
    </form>
  </section>
</template>
