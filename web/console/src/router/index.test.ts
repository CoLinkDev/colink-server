import { describe, expect, it } from 'vitest'

import { resolveAuthRedirect } from './index'

describe('authentication route guard', () => {
  it('redirects unauthenticated users away from protected routes', () => {
    expect(resolveAuthRedirect({ requiresAuth: true }, false)).toEqual({ name: 'Login' })
  })

  it('redirects authenticated users away from guest routes', () => {
    expect(resolveAuthRedirect({ requiresGuest: true }, true)).toEqual({ name: 'Devices' })
  })

  it('allows routes matching the current authentication state', () => {
    expect(resolveAuthRedirect({ requiresAuth: true }, true)).toBeUndefined()
    expect(resolveAuthRedirect({ requiresGuest: true }, false)).toBeUndefined()
  })
})
