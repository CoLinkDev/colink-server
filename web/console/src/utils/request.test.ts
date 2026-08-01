import axios from 'axios'
import MockAdapter from 'axios-mock-adapter'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { routerPush } = vi.hoisted(() => ({ routerPush: vi.fn() }))

vi.mock('@/router', () => ({
  default: { push: routerPush },
}))

describe('request token refresh interceptor', () => {
  beforeEach(() => {
    vi.resetModules()
    setActivePinia(createPinia())
    localStorage.clear()
    routerPush.mockReset()
  })

  it('retries concurrent 401 responses after a single refresh request', async () => {
    const { request, auth, requestMock, axiosMock } = await setup()
    auth.setToken('old-access')
    auth.setRefreshToken('refresh-token')

    requestMock.onGet('/protected').reply((config) =>
      config.headers?.Authorization === 'Bearer new-access'
        ? [200, { code: 0, data: 'ok' }]
        : [401],
    )
    axiosMock.onPost('/api/v1/auth/refresh').reply(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10))
      return [200, {
        code: 0,
        data: { token: 'new-access', refreshToken: 'new-refresh' },
      }]
    })

    const responses = await Promise.all([
      request.get('/protected'),
      request.get('/protected'),
      request.get('/protected'),
    ])

    expect(responses).toEqual([
      { code: 0, data: 'ok' },
      { code: 0, data: 'ok' },
      { code: 0, data: 'ok' },
    ])
    expect(axiosMock.history.post).toHaveLength(1)
    expect(auth.token).toBe('new-access')
    expect(auth.refreshToken).toBe('new-refresh')
    requestMock.restore()
    axiosMock.restore()
  })

  it('clears the session and redirects when refresh fails', async () => {
    const { request, auth, requestMock, axiosMock } = await setup()
    auth.setToken('expired-access')
    auth.setRefreshToken('expired-refresh')
    requestMock.onGet('/protected').reply(401)
    axiosMock.onPost('/api/v1/auth/refresh').reply(401)

    await expect(request.get('/protected')).rejects.toBeDefined()

    expect(auth.token).toBe('')
    expect(auth.refreshToken).toBe('')
    expect(routerPush).toHaveBeenCalledWith({ name: 'Login' })
    requestMock.restore()
    axiosMock.restore()
  })
})

async function setup() {
  const [{ default: request }, { useAuthStore }] = await Promise.all([
    import('./request'),
    import('@/store/auth'),
  ])
  const auth = useAuthStore()
  return {
    request,
    auth,
    requestMock: new MockAdapter(request),
    axiosMock: new MockAdapter(axios),
  }
}
