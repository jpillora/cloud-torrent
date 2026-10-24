<script setup lang="ts">
import { state } from '../state'
</script>

<template>
  <section v-if="state.Inbox?.Enabled" class="inbox-status" aria-label="Torrent inbox" data-testid="inbox-status">
    <strong>Torrent inbox</strong>
    <span> · {{ state.Inbox.Pending }} pending · {{ state.Inbox.Accepted }} accepted files</span>
    <p v-for="(error, index) in state.Inbox.Errors" :key="index" role="status">
      <strong v-if="error.Name">{{ error.Name }}: </strong>{{ error.Message }}
    </p>
  </section>
</template>

<style scoped>
.inbox-status { margin: 1rem 0; padding: .75rem; border: 1px solid currentColor; border-radius: .25rem; overflow-wrap: anywhere; }
p { margin: .5rem 0 0; }
</style>
