import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthUser {
  id: string
  email: string
}

interface AuthState {
  user: AuthUser | null
  accessToken: string | null
  refreshToken: string | null
  refreshTimer: ReturnType<typeof setTimeout> | null
  setAuth: (user: AuthUser, accessToken: string, refreshToken: string) => void
  clearAuth: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      accessToken: null,
      refreshToken: null,
      refreshTimer: null,
      setAuth: (user, accessToken, refreshToken) => {
        // Clear any existing proactive refresh timer
        const existing = get().refreshTimer
        if (existing) clearTimeout(existing)

        // Schedule proactive refresh 12 minutes from now (tokens expire in 15 min)
        const timer = setTimeout(async () => {
          try {
            const res = await fetch('/api/auth/refresh', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ refreshToken: get().refreshToken }),
            })
            if (!res.ok) {
              get().clearAuth()
              return
            }
            const data = await res.json()
            const currentUser = get().user
            if (currentUser) {
              get().setAuth(currentUser, data.accessToken, data.refreshToken)
              // Broadcast to other tabs
              const ch = new BroadcastChannel('auth')
              ch.postMessage({ type: 'token_refresh', accessToken: data.accessToken, refreshToken: data.refreshToken })
              ch.close()
            }
          } catch {
            get().clearAuth()
          }
        }, 12 * 60 * 1000)

        set({ user, accessToken, refreshToken, refreshTimer: timer })
      },
      clearAuth: () => {
        clearTimeout(get().refreshTimer ?? undefined)
        set({ user: null, accessToken: null, refreshToken: null, refreshTimer: null })
      },
    }),
    {
      name: 'atp-auth',
      partialize: (state) => ({
        user: state.user,
        accessToken: state.accessToken,
        refreshToken: state.refreshToken,
      }),
    }
  )
)
