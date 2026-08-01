import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import request from '@/utils/request'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')
  const refreshToken = ref(localStorage.getItem('refreshToken') || '')
  const email = ref(localStorage.getItem('email') || '')
  const username = ref(localStorage.getItem('username') || '')

  const isAuthenticated = computed(() => !!token.value)

  function setToken(newToken: string) {
    token.value = newToken
    localStorage.setItem('token', newToken)
  }

  function setRefreshToken(newRefreshToken: string) {
    refreshToken.value = newRefreshToken
    localStorage.setItem('refreshToken', newRefreshToken)
  }

  function setEmail(newEmail: string) {
    email.value = newEmail
    localStorage.setItem('email', newEmail)
  }

  function setUsername(newUsername: string) {
    username.value = newUsername
    localStorage.setItem('username', newUsername)
  }

  function clearToken() {
    token.value = ''
    refreshToken.value = ''
    email.value = ''
    username.value = ''
    localStorage.removeItem('token')
    localStorage.removeItem('refreshToken')
    localStorage.removeItem('email')
    localStorage.removeItem('username')
  }

  async function fetchProfile() {
    if (!token.value) return false
    try {
      const res: any = await request.get('/me')
      if (res.code !== 0 || !res.data?.email) {
        throw new Error(res.message || 'Invalid profile response')
      }
      setEmail(res.data.email)
      setUsername(res.data.username || '')
      return true
    } catch {
      clearToken()
      return false
    }
  }

  return { 
    token, 
    refreshToken, 
    email, 
    username, 
    isAuthenticated, 
    setToken, 
    setRefreshToken, 
    setEmail, 
    setUsername, 
    clearToken, 
    fetchProfile 
  }
})
