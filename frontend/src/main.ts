import { createApp } from 'vue'
import App from './App.vue'
import '@fontsource/lato/400.css'
import '@fontsource/lato/700.css'
import './styles/main.css'
import { startSync } from './state'

startSync()
createApp(App).mount('#app')
