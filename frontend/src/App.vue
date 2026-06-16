<script setup lang="ts">
import Header from './components/Header.vue'
import Omni from './components/Omni.vue'
import Torrents from './components/Torrents.vue'
import Downloads from './components/Downloads.vue'
import Footer from './components/Footer.vue'
import { state, connection } from './state'
</script>

<template>
  <div class="app">
    <!-- temporary realtime indicator; replaced by the real sections later -->
    <p class="sync-demo" data-testid="sync-demo">
      velox:
      {{ connection.connected ? 'connected' : connection.hasConnected ? 'disconnected' : 'connecting…' }}
      — {{ Object.keys(state.Torrents ?? {}).length }} torrents
    </p>
    <Header />
    <Omni />
    <Torrents />
    <Downloads />
    <Footer />
  </div>
</template>
