<script setup lang="ts">
import Header from './components/Header.vue'
import ConfigEditor from './components/ConfigEditor.vue'
import Omni from './components/Omni.vue'
import Torrents from './components/Torrents.vue'
import Downloads from './components/Downloads.vue'
import Footer from './components/Footer.vue'
</script>

<template>
  <div class="app">
    <Header />
    <ConfigEditor />
    <Omni />
    <Torrents />
    <Downloads />
    <Footer />
  </div>
</template>
