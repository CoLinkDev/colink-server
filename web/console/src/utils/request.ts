import axios from 'axios'
import { useAuthStore } from '@/store/auth'
import router from '@/router'

const request = axios.create({
  baseURL: '/api/v1',
  timeout: 10000
})

request.interceptors.request.use(
  config => {
    const auth = useAuthStore()
    if (auth.token) {
      config.headers.Authorization = `Bearer ${auth.token}`
    }
    return config
  },
  error => Promise.reject(error)
)

let isRefreshing = false
let failedQueue: any[] = []

const processQueue = (error: any, token: string | null = null) => {
  failedQueue.forEach(prom => {
    if (error) {
      prom.reject(error)
    } else {
      prom.resolve(token)
    }
  })
  failedQueue = []
}

request.interceptors.response.use(
  response => response.data,
  async error => {
    const originalRequest = error.config

    if (error.response?.status === 401 && !originalRequest._retry) {
      if (originalRequest.url?.includes('/auth/refresh')) {
        const auth = useAuthStore()
        auth.clearToken()
        router.push({ name: 'Login' })
        return Promise.reject(error)
      }

      const auth = useAuthStore()
      if (!auth.refreshToken) {
        auth.clearToken()
        router.push({ name: 'Login' })
        return Promise.reject(error)
      }

      if (isRefreshing) {
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject })
        })
          .then(token => {
            originalRequest.headers.Authorization = `Bearer ${token}`
            return request(originalRequest)
          })
          .catch(err => {
            return Promise.reject(err)
          })
      }

      originalRequest._retry = true
      isRefreshing = true

      try {
        const res = await axios.post('/api/v1/auth/refresh', {
          refreshToken: auth.refreshToken
        })

        if (res.data && res.data.code === 0 && res.data.data?.token) {
          const { token, refreshToken } = res.data.data
          auth.setToken(token)
          if (refreshToken) {
            auth.setRefreshToken(refreshToken)
          }

          processQueue(null, token)

          originalRequest.headers.Authorization = `Bearer ${token}`
          return request(originalRequest)
        } else {
          throw new Error('Refresh token invalid')
        }
      } catch (refreshError) {
        processQueue(refreshError, null)
        auth.clearToken()
        router.push({ name: 'Login' })
        return Promise.reject(refreshError)
      } finally {
        isRefreshing = false
      }
    }

    return Promise.reject(error)
  }
)

export default request
