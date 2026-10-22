import { ref, watch } from 'vue'

export type ThemePreference = 'system' | 'light' | 'dark'
const storageKey = 'tcTheme'
const systemTheme = window.matchMedia('(prefers-color-scheme: dark)')
const validPreference = (value: string | null): ThemePreference =>
  value === 'light' || value === 'dark' ? value : 'system'

function readPreference(): ThemePreference {
  try {
    return validPreference(localStorage.getItem(storageKey))
  } catch {
    return 'system'
  }
}

export const themePreference = ref<ThemePreference>(readPreference())

function applyTheme() {
  document.documentElement.dataset.theme = themePreference.value === 'system'
    ? systemTheme.matches ? 'dark' : 'light'
    : themePreference.value
}

watch(themePreference, (value) => {
  applyTheme()
  try {
    localStorage.setItem(storageKey, value)
  } catch { /* The preference still works for this page when storage is unavailable. */ }
}, { flush: 'sync' })
systemTheme.addEventListener('change', applyTheme)
window.addEventListener('storage', (event) => {
  if (event.storageArea === localStorage && (event.key === storageKey || event.key === null)) {
    themePreference.value = readPreference()
  }
})
applyTheme()
