<template>
  <div class="min-h-screen w-full flex bg-background">
    <!-- Left Pane (Artistic brand panel for desktop) -->
    <div class="hidden lg:flex lg:w-1/2 bg-zinc-950 text-zinc-50 flex-col justify-center p-12 relative overflow-hidden border-r border-zinc-900">
      <!-- Glow & Grid -->
      <div class="absolute inset-0 bg-[radial-gradient(circle_at_30%_30%,#18181b,transparent_70%)] opacity-80"></div>
      
      <!-- Premium Constellation SVG -->
      <svg class="absolute inset-0 w-full h-full opacity-35" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id="grid-left" width="32" height="32" patternUnits="userSpaceOnUse">
            <circle cx="1" cy="1" r="1" fill="rgba(255,255,255,0.12)" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill="url(#grid-left)" />
        
        <g stroke="rgba(255,255,255,0.08)" stroke-width="0.75" fill="none">
          <line x1="20%" y1="30%" x2="45%" y2="25%" />
          <line x1="45%" y1="25%" x2="35%" y2="60%" />
          <line x1="45%" y1="25%" x2="70%" y2="40%" />
          <line x1="70%" y1="40%" x2="60%" y2="75%" />
          <line x1="35%" y1="60%" x2="60%" y2="75%" />
          <line x1="20%" y1="30%" x2="35%" y2="60%" />
        </g>
        <g fill="none" stroke="rgba(255,255,255,0.3)" stroke-width="1.5">
          <circle cx="20%" cy="30%" r="3" fill="#09090b" />
          <circle cx="45%" cy="25%" r="4" fill="#09090b" />
          <circle cx="35%" cy="60%" r="3.5" fill="#09090b" />
          <circle cx="70%" cy="40%" r="4.5" fill="#09090b" />
          <circle cx="60%" cy="75%" r="3" fill="#09090b" />
        </g>
      </svg>
      
      <!-- Top Section -->
      <div class="absolute top-12 left-12 z-10 flex items-center gap-2 select-none">
        <span class="text-xl font-bold tracking-tighter text-zinc-100 uppercase">CoLink</span>
      </div>

      <!-- Center tagline -->
      <div class="relative z-10 select-none">
        <h2 class="text-3xl font-semibold tracking-tight leading-snug text-zinc-100">
          {{ $t('login.tagline') }}
        </h2>
      </div>
    </div>

    <!-- Right Pane (Clean Sign Up form) -->
    <div class="w-full lg:w-1/2 flex flex-col justify-center items-center p-6 md:p-12 relative">
      <!-- Subtle dotted grid for mobile background -->
      <div class="absolute inset-0 bg-[radial-gradient(circle_at_50%_120%,rgba(120,119,198,0.03),transparent_60%)] lg:bg-none pointer-events-none"></div>
      <div class="absolute inset-0 bg-[linear-gradient(to_right,#8080800a_1px,transparent_1px),linear-gradient(to_bottom,#8080800a_1px,transparent_1px)] bg-[size:24px_24px] pointer-events-none opacity-50"></div>

      <div class="w-full max-w-sm relative z-10">
        <div class="flex flex-col space-y-2 text-center mb-8">
          <h1 class="text-xl font-bold tracking-tight text-foreground sm:text-2xl">{{ $t('register.title') }}</h1>
          <p class="text-sm text-muted-foreground">{{ $t('register.subtitle') }}</p>
        </div>

        <form @submit.prevent="handleRegister" class="space-y-4">
          <div class="space-y-1.5">
            <label class="text-xs font-semibold uppercase tracking-wider text-muted-foreground leading-none">{{ $t('login.email') }}</label>
            <input 
              v-model="form.email" 
              type="email" 
              required
              class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-all focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            >
          </div>
          <div class="space-y-1.5">
            <label class="text-xs font-semibold uppercase tracking-wider text-muted-foreground leading-none">{{ $t('register.username') }}</label>
            <input 
              v-model="form.username" 
              type="text" 
              required
              class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-all focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            >
          </div>
          <div class="space-y-1.5">
            <label class="text-xs font-semibold uppercase tracking-wider text-muted-foreground leading-none">{{ $t('login.password') }}</label>
            <input 
              v-model="form.password" 
              type="password" 
              required
              class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-all focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            >
          </div>
          <div class="space-y-1.5">
            <label class="text-xs font-semibold uppercase tracking-wider text-muted-foreground leading-none">{{ $t('register.confirmPassword') }}</label>
            <input 
              v-model="form.confirmPassword" 
              type="password" 
              required
              class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-all focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            >
          </div>
          
          <button 
            type="submit" 
            :disabled="loading"
            class="inline-flex w-full items-center justify-center rounded-md text-sm font-semibold transition-all focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 bg-primary text-primary-foreground shadow hover:bg-primary/90 h-9 px-4 py-2 mt-4"
          >
            <span v-if="loading" class="w-4 h-4 border-2 border-primary-foreground border-t-transparent rounded-full animate-spin mr-2"></span>
            {{ $t('register.register') }}
          </button>
        </form>

        <div class="mt-8 text-center text-sm">
          <span class="text-muted-foreground">{{ $t('register.hasAccount') }} </span>
          <router-link to="/login" class="text-foreground font-medium hover:underline underline-offset-4">{{ $t('register.signIn') }}</router-link>
        </div>
      </div>
    </div>

    <!-- Alert Dialog Modal -->
    <Dialog 
      v-model:open="alertDialog.open"
      :title="alertDialog.title"
      :description="alertDialog.description"
      :confirmText="$t('common.ok')"
      hideCancel
    />
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '@/utils/request'
import Dialog from '@/components/Dialog.vue'

const router = useRouter()
const { t } = useI18n()

const form = reactive({
  email: '',
  username: '',
  password: '',
  confirmPassword: ''
})
const loading = ref(false)

const alertDialog = reactive({
  open: false,
  title: '',
  description: ''
})

const showAlert = (title: string, description: string) => {
  alertDialog.title = title
  alertDialog.description = description
  alertDialog.open = true
}

const handleRegister = async () => {
  if (form.password !== form.confirmPassword) {
    showAlert(t('settings.passwordMismatch'), '')
    return
  }
  loading.value = true
  try {
    const res: any = await request.post('/auth/register', {
      email: form.email,
      username: form.username,
      password: form.password
    })
    if (res.code === 0) {
      router.push('/login')
    } else {
      showAlert(t('register.registerFailed'), res.message || t('register.tryAgain'))
    }
  } catch (error: any) {
    showAlert(t('register.registerFailed'), error.response?.data?.message || t('register.connectionError'))
  } finally {
    loading.value = false
  }
}
</script>
