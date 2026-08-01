<template>
  <div class="min-h-screen bg-background text-foreground flex overflow-hidden">
    <!-- Mobile header with hamburger -->
    <div class="md:hidden fixed top-0 left-0 right-0 h-14 border-b border-border bg-background/95 backdrop-blur z-20 flex items-center justify-between px-4">
      <h1 class="text-base font-semibold">CoLink</h1>
      <button @click="mobileMenuOpen = !mobileMenuOpen" class="p-1.5 text-muted-foreground hover:text-foreground transition-colors">
        <Menu class="w-5 h-5" />
      </button>
    </div>

    <!-- Sidebar (responsive) -->
    <aside 
      :class="[
        'fixed md:relative top-0 left-0 z-30 h-screen w-[240px] border-r border-border bg-background flex flex-col transition-transform duration-300',
        mobileMenuOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'
      ]"
    >
      <div class="h-14 flex items-center justify-between px-5">
        <h1 class="text-base font-semibold">CoLink</h1>
        <button @click="mobileMenuOpen = false" class="md:hidden p-1.5 text-muted-foreground">
          <X class="w-4 h-4" />
        </button>
      </div>
      
      <nav class="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
        <router-link 
          v-for="item in navItems" 
          :key="item.path" 
          :to="item.path"
          @click="mobileMenuOpen = false"
          class="flex items-center gap-3 px-3 py-2 rounded-md transition-colors text-sm"
          :class="[
            $route.path === item.path 
              ? 'bg-muted font-medium text-foreground' 
              : 'text-muted-foreground hover:bg-muted hover:text-foreground'
          ]"
        >
          <component :is="item.icon" class="w-4 h-4 shrink-0" />
          {{ $t(item.nameKey) }}
        </router-link>
      </nav>

      <!-- User, Language & Logout at bottom -->
      <div class="px-3 pb-4 space-y-1">
        <!-- User profile -->
        <div class="flex items-center gap-3 px-3 py-2 text-sm text-foreground" :title="displayName">
          <div class="w-4 h-4 rounded-full bg-muted flex items-center justify-center font-bold border border-border text-[8px] shrink-0 uppercase">
            {{ displayInitial }}
          </div>
          <span class="truncate font-medium">{{ displayName }}</span>
        </div>

        <!-- Language selector -->
        <button 
          @click="openLanguageDialog"
          class="flex w-full items-center gap-3 px-3 py-2 rounded-md text-sm text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
        >
          <Languages class="w-4 h-4 shrink-0" />
          <span class="truncate font-medium flex-1 text-left">
            {{ languageOptions.find(opt => opt.code === locale)?.name || 'English' }}
          </span>
        </button>

        <!-- Theme selector -->
        <button 
          @click="openThemeDialog"
          class="flex w-full items-center gap-3 px-3 py-2 rounded-md text-sm text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
        >
          <component :is="themeIcon" class="w-4 h-4 shrink-0" />
          <span class="truncate font-medium flex-1 text-left">
            {{ $t(`theme.${theme}`) }}
          </span>
        </button>

        <!-- Logout button -->
        <button 
          @click="triggerLogout"
          class="flex w-full items-center gap-3 px-3 py-2 rounded-md text-sm text-muted-foreground hover:bg-destructive/10 hover:text-destructive transition-colors"
        >
          <LogOut class="w-4 h-4 shrink-0" />
          {{ $t('nav.logout') }}
        </button>
      </div>
    </aside>

    <!-- Logout Dialog Modal -->
    <Dialog 
      v-model:open="logoutDialogOpen"
      :title="$t('settings.signOutConfirm')"
      description=""
      :confirmText="$t('nav.logout')"
      variant="destructive"
      @confirm="confirmLogout"
    />

    <!-- Language Selection Dialog Modal -->
    <Dialog 
      v-model:open="langDialogOpen"
      :title="$t('common.selectLanguage')"
      description=""
      hideConfirm
    >
      <div class="grid grid-cols-2 gap-2 mt-2">
        <button 
          v-for="opt in languageOptions" 
          :key="opt.code"
          @click="selectLanguage(opt.code)"
          class="flex flex-col items-start justify-center px-4 py-3 rounded-lg border text-sm font-medium transition-all duration-200"
          :class="locale === opt.code 
            ? 'border-primary bg-primary/5 text-primary' 
             : 'border-border bg-card hover:bg-muted text-foreground'"
        >
          <span class="text-sm font-semibold">{{ opt.name }}</span>
          <span class="text-xs text-muted-foreground font-normal mt-0.5">{{ opt.nativeName }}</span>
        </button>
      </div>
    </Dialog>

    <!-- Theme Selection Dialog Modal -->
    <Dialog 
      v-model:open="themeDialogOpen"
      :title="$t('theme.title')"
      description=""
      hideConfirm
    >
      <div class="grid gap-2 mt-2">
        <button 
          v-for="opt in themeOptions" 
          :key="opt.code"
          @click="selectTheme(opt.code)"
          class="flex w-full items-center justify-between px-4 py-3 rounded-lg border text-sm font-medium transition-all duration-200"
          :class="theme === opt.code 
            ? 'border-primary bg-primary/5 text-primary' 
             : 'border-border bg-card hover:bg-muted text-foreground'"
        >
          <div class="flex items-center gap-3">
            <component :is="opt.icon" class="w-4 h-4 shrink-0" />
            <span>{{ opt.name }}</span>
          </div>
          <span v-if="theme === opt.code" class="w-1.5 h-1.5 rounded-full bg-primary shrink-0"></span>
        </button>
      </div>
    </Dialog>

    <!-- Overlay for mobile menu -->
    <div 
      v-if="mobileMenuOpen" 
      @click="mobileMenuOpen = false"
      class="fixed inset-0 bg-background/80 backdrop-blur-sm z-20 md:hidden"
    ></div>

    <!-- Main Content -->
    <main class="flex-1 flex flex-col relative h-screen overflow-y-auto bg-muted/20 pt-14 md:pt-0">
      <header class="hidden md:flex h-14 items-center px-8 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 sticky top-0 z-10 border-b border-border/50">
        <h2 class="text-sm font-medium text-muted-foreground">{{ currentRouteName }}</h2>
      </header>
      <div class="p-4 md:p-8 max-w-6xl mx-auto w-full">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Settings, LogOut, Menu, X, Languages, Sun, Moon, Monitor as MonitorIcon } from 'lucide-vue-next'
import { useAuthStore } from '@/store/auth'
import request from '@/utils/request'
import Dialog from '@/components/Dialog.vue'
import { useTheme, type ThemeMode } from '@/utils/theme'

const router = useRouter()
const route = useRoute()
const { locale, t } = useI18n()
const auth = useAuthStore()
const { theme, setTheme } = useTheme()

const themeIcon = computed(() => {
  if (theme.value === 'light') return Sun
  if (theme.value === 'dark') return Moon
  return MonitorIcon
})

const displayName = computed(() => auth.username || auth.email || 'User')
const displayInitial = computed(() => displayName.value.charAt(0) || 'U')

const mobileMenuOpen = ref(false)
const logoutDialogOpen = ref(false)

const navItems = [
  { nameKey: 'nav.devices', path: '/', icon: MonitorIcon },
  { nameKey: 'nav.settings', path: '/settings', icon: Settings }
]

const currentRouteName = computed(() => {
  const currentItem = navItems.find(i => i.path === route.path)
  return currentItem ? t(currentItem.nameKey) : ''
})

const triggerLogout = () => {
  logoutDialogOpen.value = true
}

const confirmLogout = async () => {
  try {
    await request.post('/auth/logout', {
      refreshToken: auth.refreshToken
    })
  } catch (e) {
    // Ignore error
  }
  auth.clearToken()
  router.push({ name: 'Login' })
}

const langDialogOpen = ref(false)

const languageOptions = [
  { code: 'en', name: 'English', nativeName: 'English' },
  { code: 'zh-CN', name: '简体中文', nativeName: 'Simplified Chinese' },
  { code: 'zh-TW', name: '繁體中文', nativeName: 'Traditional Chinese' },
  { code: 'ja', name: '日本語', nativeName: 'Japanese' },
  { code: 'ko', name: '한국어', nativeName: 'Korean' },
  { code: 'es', name: 'Español', nativeName: 'Spanish' },
  { code: 'de', name: 'Deutsch', nativeName: 'German' },
  { code: 'ru', name: 'Русский', nativeName: 'Russian' }
]

const openLanguageDialog = () => {
  langDialogOpen.value = true
}

const selectLanguage = (code: string) => {
  locale.value = code
  localStorage.setItem('locale', code)
  langDialogOpen.value = false
}

const themeDialogOpen = ref(false)

const themeOptions = computed(() => [
  { code: 'light' as ThemeMode, name: t('theme.light'), icon: Sun },
  { code: 'dark' as ThemeMode, name: t('theme.dark'), icon: Moon },
  { code: 'auto' as ThemeMode, name: t('theme.auto'), icon: MonitorIcon }
])

const openThemeDialog = () => {
  themeDialogOpen.value = true
}

const selectTheme = (mode: ThemeMode) => {
  setTheme(mode)
  themeDialogOpen.value = false
}

onMounted(async () => {
  if (!await auth.fetchProfile()) {
    router.push({ name: 'Login' })
  }
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
