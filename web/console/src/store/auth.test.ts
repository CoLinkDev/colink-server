import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from './auth'

const { getProfile } = vi.hoisted(() => ({ getProfile: vi.fn() }))

vi.mock('@/utils/request', () => ({
  default: { get: getProfile },
}))

describe('auth profile loading', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    getProfile.mockReset()
  })

  it('updates cached profile after a successful response', async () => {
    const auth = useAuthStore()
    auth.setToken('access-token')
    getProfile.mockResolvedValue({
      code: 0,
      data: { email: 'alice@example.test', username: 'alice' },
    })

    await expect(auth.fetchProfile()).resolves.toBe(true)

    expect(auth.email).toBe('alice@example.test')
    expect(auth.username).toBe('alice')
    expect(auth.isAuthenticated).toBe(true)
  })

  it('clears the full session when the profile request fails', async () => {
    const auth = useAuthStore()
    auth.setToken('invalid-access')
    auth.setRefreshToken('invalid-refresh')
    auth.setEmail('stale@example.test')
    auth.setUsername('stale-user')
    getProfile.mockRejectedValue(new Error('profile unavailable'))

    await expect(auth.fetchProfile()).resolves.toBe(false)

    expect(auth.token).toBe('')
    expect(auth.refreshToken).toBe('')
    expect(auth.email).toBe('')
    expect(auth.username).toBe('')
    expect(auth.isAuthenticated).toBe(false)
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('refreshToken')).toBeNull()
  })
})
