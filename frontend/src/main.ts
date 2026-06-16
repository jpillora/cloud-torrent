import { createApp } from 'vue'
import App from './App.vue'
import './styles/main.css'
import { startSync } from './state'

startSync()
createApp(App).mount('#app')
