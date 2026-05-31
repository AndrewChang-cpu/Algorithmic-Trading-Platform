import { create } from 'zustand'
import { BASE_URL } from './api'

interface AuthUser {
  id: string
  email: string
}

interface AuthState {
  user: AuthUser | null
  accessToken: string | null
  refreshTimer: ReturnType<typeof setTimeout> | null
  setAuth: (user: AuthUser, accessToken: string) => void
  clearAuth: () => void
}

export const useAuthStore = create<AuthState>()((set, get) => ({
  user: null,
  accessToken: null,
  refreshTimer: null,
  setAuth: (user, accessToken) => {
    // Clear any existing proactive refresh timer
    const existing = get().refreshTimer
    if (existing) clearTimeout(existing)

    // Schedule proactive refresh 12 minutes from now (tokens expire in 15 min)
    const timer = setTimeout(async () => {
      try {
        const res = await fetch(`${BASE_URL}/api/auth/refresh`, {
          method: 'POST',
          credentials: 'include',
        })
        if (!res.ok) {
          get().clearAuth()
          return
        }
        const data = await res.json()
        const currentUser = get().user
        if (currentUser) {
          get().setAuth(currentUser, data.accessToken)
          // Broadcast to other tabs
          const ch = new BroadcastChannel('auth')
          ch.postMessage({ type: 'token_refresh', accessToken: data.accessToken })
          ch.close()
        }
      } catch {
        get().clearAuth()
      }
    }, 12 * 60 * 1000)

    set({ user, accessToken, refreshTimer: timer })
  },
  clearAuth: () => {
    clearTimeout(get().refreshTimer ?? undefined)
    set({ user: null, accessToken: null, refreshTimer: null })
  },
}))
