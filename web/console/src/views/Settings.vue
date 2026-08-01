<template>
  <div class="space-y-6">
    <div class="space-y-1">
      <h3 class="text-xl font-semibold tracking-tight">{{ $t('settings.title') }}</h3>
      <p class="text-muted-foreground text-sm">{{ $t('settings.subtitle') }}</p>
    </div>
    
    <div class="h-[1px] bg-border my-6"></div>

    <div class="grid gap-8">
      <div class="flex flex-col gap-6 md:flex-row md:items-start md:justify-between py-2">
        <div class="space-y-1 md:w-1/3">
          <h4 class="text-base font-semibold">{{ $t('settings.changeUsername') }}</h4>
          <p class="text-xs text-muted-foreground max-w-[240px]">
            {{ $t('settings.usernameHint') }}
          </p>
        </div>

        <form @submit.prevent="handleUpdateUsername" class="space-y-4 md:w-2/3 max-w-3xl w-full">
          <div class="grid gap-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
            <div class="space-y-1.5">
              <label class="text-xs font-semibold uppercase tracking-wider text-muted-foreground leading-none">{{ $t('register.username') }}</label>
              <input 
                v-model="usernameForm.username" 
                type="text" 
                required
                class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              >
            </div>
            <button 
              type="submit" 
              :disabled="usernameLoading"
              class="inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 bg-primary text-primary-foreground shadow hover:bg-primary/90 h-9 px-4 py-2"
            >
              <span v-if="usernameLoading" class="w-4 h-4 border-2 border-primary-foreground border-t-transparent rounded-full animate-spin mr-2"></span>
              {{ $t('settings.updateUsername') }}
            </button>
          </div>
        </form>
      </div>

      <div class="flex flex-col gap-6 md:flex-row md:items-start md:justify-between py-2">
        <div class="space-y-1 md:w-1/3">
          <h4 class="text-base font-semibold">{{ $t('settings.changePassword') }}</h4>
          <p class="text-xs text-muted-foreground max-w-[240px]">
            {{ $t('settings.changePasswordHint') }}
          </p>
        </div>
        
        <form @submit.prevent="handleChangePassword" class="space-y-4 md:w-2/3 max-w-3xl w-full">
          <div class="grid gap-4 sm:grid-cols-3">
            <div class="space-y-1.5">
              <label class="text-xs font-semibold uppercase tracking-wider text-muted-foreground leading-none">{{ $t('settings.currentPassword') }}</label>
              <input 
                v-model="pwdForm.oldPassword" 
                type="password" 
                required
                class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              >
            </div>
            <div class="space-y-1.5">
              <label class="text-xs font-semibold uppercase tracking-wider text-muted-foreground leading-none">{{ $t('settings.newPassword') }}</label>
              <input 
                v-model="pwdForm.newPassword" 
                type="password" 
                required
                class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              >
            </div>
            <div class="space-y-1.5">
              <label class="text-xs font-semibold uppercase tracking-wider text-muted-foreground leading-none">{{ $t('settings.confirmNewPassword') }}</label>
              <input 
                v-model="pwdForm.confirmPassword" 
                type="password" 
                required
                class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              >
            </div>
          </div>
          
          <div class="flex justify-end pt-4 border-t border-border/40">
            <button 
              type="submit" 
              :disabled="passwordLoading"
              class="inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 bg-primary text-primary-foreground shadow hover:bg-primary/90 h-9 px-4 py-2"
            >
              <span v-if="passwordLoading" class="w-4 h-4 border-2 border-primary-foreground border-t-transparent rounded-full animate-spin mr-2"></span>
              {{ $t('settings.updatePassword') }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Alert / Message Dialog Modal -->
    <Dialog 
      v-model:open="alertDialog.open"
      :title="alertDialog.title"
      :description="alertDialog.description"
      :confirmText="$t('common.ok')"
      hideCancel
      @confirm="handleAlertConfirm"
    />
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/store/auth'
import request from '@/utils/request'
import Dialog from '@/components/Dialog.vue'

const router = useRouter()
const auth = useAuthStore()
const { t } = useI18n()

const pwdForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})
const usernameForm = reactive({
  username: auth.username
})
const usernameLoading = ref(false)
const passwordLoading = ref(false)

const alertDialog = reactive({
  open: false,
  title: '',
  description: '',
  onConfirm: () => {}
})

const showAlert = (title: string, description: string, onConfirm = () => {}) => {
  alertDialog.title = title
  alertDialog.description = description
  alertDialog.onConfirm = onConfirm
  alertDialog.open = true
}

const handleAlertConfirm = () => {
  alertDialog.onConfirm()
}

watch(
  () => auth.username,
  (value) => {
    usernameForm.username = value
  },
  { immediate: true }
)

const handleChangePassword = async () => {
  if (pwdForm.newPassword !== pwdForm.confirmPassword) {
    showAlert(t('settings.passwordMismatch'), '')
    return
  }
  passwordLoading.value = true
  try {
    const res: any = await request.post('/auth/change-password', {
      oldPassword: pwdForm.oldPassword,
      newPassword: pwdForm.newPassword
    })
    if (res.code === 0) {
      showAlert(t('settings.passwordUpdated'), '', () => {
        auth.clearToken()
        router.push('/login')
      })
    } else {
      showAlert(t('settings.passwordUpdateFailed'), res.message || '')
    }
  } catch (error: any) {
    showAlert(t('settings.passwordUpdateFailed'), error.response?.data?.message || '')
  } finally {
    passwordLoading.value = false
  }
}

const handleUpdateUsername = async () => {
  usernameLoading.value = true
  try {
    const res: any = await request.put('/me/username', {
      username: usernameForm.username
    })
    if (res.code === 0) {
      auth.setUsername(usernameForm.username)
      showAlert(t('settings.usernameUpdated'), '')
    } else {
      showAlert(t('settings.usernameUpdateFailed'), res.message || '')
    }
  } catch (error: any) {
    showAlert(t('settings.usernameUpdateFailed'), error.response?.data?.message || '')
  } finally {
    usernameLoading.value = false
  }
}
</script>
