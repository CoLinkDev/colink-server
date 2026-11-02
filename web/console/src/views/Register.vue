<template>
  <AuthLayout :title="$t('register.title')" :subtitle="$t('register.subtitle')">
    <form @submit.prevent="handleRegister" class="space-y-4">
      <AuthField
        id="email"
        v-model="form.email"
        :label="$t('login.email')"
        type="email"
        autocomplete="email"
        required
      />
      <AuthField
        id="username"
        v-model="form.username"
        :label="$t('register.username')"
        type="text"
        autocomplete="username"
        required
      />
      <AuthField
        id="password"
        v-model="form.password"
        :label="$t('login.password')"
        type="password"
        autocomplete="new-password"
        required
      />
      <AuthField
        id="confirmPassword"
        v-model="form.confirmPassword"
        :label="$t('register.confirmPassword')"
        type="password"
        autocomplete="new-password"
        required
      />

      <div class="pt-2">
        <button
          type="submit"
          :disabled="loading"
          class="inline-flex h-10 w-full items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50"
        >
          <span v-if="loading" class="w-4 h-4 border-2 border-primary-foreground border-t-transparent rounded-full animate-spin mr-2"></span>
          {{ $t('register.register') }}
        </button>
      </div>
    </form>

    <template #footer>
      {{ $t('register.hasAccount') }}
      <router-link to="/login" class="font-medium text-foreground underline-offset-4 hover:underline">{{ $t('register.signIn') }}</router-link>
    </template>

    <Dialog
      v-model:open="alertDialog.open"
      :title="alertDialog.title"
      :description="alertDialog.description"
      :confirmText="$t('common.ok')"
      hideCancel
    />
  </AuthLayout>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '@/utils/request'
import Dialog from '@/components/Dialog.vue'
import AuthLayout from '@/layout/AuthLayout.vue'
import AuthField from '@/components/AuthField.vue'

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
