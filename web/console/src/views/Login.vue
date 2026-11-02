<template>
  <AuthLayout :title="$t('login.welcome')" :subtitle="$t('login.subtitle')">
    <form @submit.prevent="handleLogin" class="space-y-4">
      <AuthField
        id="identifier"
        v-model="form.identifier"
        :label="$t('login.identifier')"
        type="text"
        autocomplete="username"
        :placeholder="$t('login.identifierPlaceholder')"
        required
      />
      <AuthField
        id="password"
        v-model="form.password"
        :label="$t('login.password')"
        type="password"
        autocomplete="current-password"
        required
      />

      <div class="pt-2">
        <button
          type="submit"
          :disabled="loading"
          class="inline-flex h-10 w-full items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50"
        >
          <span v-if="loading" class="w-4 h-4 border-2 border-primary-foreground border-t-transparent rounded-full animate-spin mr-2"></span>
          {{ $t('login.signIn') }}
        </button>
      </div>
    </form>

    <template #footer>
      {{ $t('login.noAccount') }}
      <router-link to="/register" class="font-medium text-foreground underline-offset-4 hover:underline">{{ $t('login.createOne') }}</router-link>
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
import { useAuthStore } from '@/store/auth'
import request from '@/utils/request'
import Dialog from '@/components/Dialog.vue'
import AuthLayout from '@/layout/AuthLayout.vue'
import AuthField from '@/components/AuthField.vue'

const router = useRouter()
const auth = useAuthStore()
const { t } = useI18n()

const form = reactive({
  identifier: '',
  password: ''
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

const handleLogin = async () => {
  loading.value = true
  try {
    const res: any = await request.post('/auth/login', form)
    if (res.code === 0 && res.data?.token) {
      auth.setToken(res.data.token)
      if (res.data.refreshToken) {
        auth.setRefreshToken(res.data.refreshToken)
      }
      router.push('/')
    } else {
      showAlert(t('login.loginFailed'), res.message || t('login.checkCredentials'))
    }
  } catch (error: any) {
    showAlert(t('login.loginFailed'), error.response?.data?.message || t('login.connectionError'))
  } finally {
    loading.value = false
  }
}
</script>
