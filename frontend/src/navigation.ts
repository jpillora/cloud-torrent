import { ref } from 'vue'

function currentPage(): 'home' | 'settings' {
  return window.location.hash === '#/settings' ? 'settings' : 'home'
}

// Hash navigation supports direct links, reload and browser Back without a
// server route or another router dependency.
export const page = ref(currentPage())
window.addEventListener('hashchange', () => { page.value = currentPage() })
export function goHome() { window.location.hash = '/' }
