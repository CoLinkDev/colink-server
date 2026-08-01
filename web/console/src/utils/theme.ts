import { ref } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'auto'

const THEME_KEY = 'colink.config.theme'

// Read theme from localStorage or default to 'auto'
const theme = ref<ThemeMode>((localStorage.getItem(THEME_KEY) as ThemeMode) || 'auto')

// Apply the theme to documentElement
export function applyTheme(mode: ThemeMode) {
  const root = document.documentElement
  let isDark = false

  if (mode === 'dark') {
    isDark = true
  } else if (mode === 'light') {
    isDark = false
  } else {
    // auto mode: check system preferences
    isDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  }

  if (isDark) {
    root.classList.add('dark')
  } else {
    root.classList.remove('dark')
  }
}

// Set up event listener for system preference change
let mediaQueryListener: ((e: MediaQueryListEvent) => void) | null = null

function setupMediaQueryListener() {
  if (mediaQueryListener) return
  
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  mediaQueryListener = () => {
    if (theme.value === 'auto') {
      applyTheme('auto')
    }
  }
  
  if (media.addEventListener) {
    media.addEventListener('change', mediaQueryListener)
  } else {
    // Fallback for older browsers
    (media as any).addListener(mediaQueryListener)
  }
}

export function useTheme() {
  setupMediaQueryListener()

  const setTheme = (mode: ThemeMode) => {
    theme.value = mode
    localStorage.setItem(THEME_KEY, mode)
    applyTheme(mode)
  }

  // Initial application
  applyTheme(theme.value)

  return {
    theme,
    setTheme
  }
}
