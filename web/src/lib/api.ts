import axios from 'axios'
import { useAuthStore } from './store'

const BASE_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

export const apiClient = axios.create({
  baseURL: BASE_URL,
})

// Inject access token on every request
apiClient.interceptors.request.use((config) => {
  const token = useAuthStore.getState().accessToken
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

let isRefreshing = false
let failedQueue: Array<{ resolve: (v: string) => void; reject: (e: unknown) => void }> = []

function processQueue(error: unknown, token: string | null) {
  failedQueue.forEach(({ resolve, reject }) => {
    if (error) reject(error)
    else if (token) resolve(token)
    else reject(new Error('No token available'))
  })
  failedQueue = []
}

// Silent token refresh on 401
apiClient.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config
    if (error.response?.status !== 401 || originalRequest._retry) {
      return Promise.reject(error)
    }
    // Auth endpoints return 401 for expected reasons (wrong password, expired token).
    // Don't intercept their 401s — let them propagate to the mutation's error handler.
    if (originalRequest.url?.includes('/api/auth/')) {
      return Promise.reject(error)
    }

    const { refreshToken, setAuth, clearAuth } = useAuthStore.getState()
    if (!refreshToken) {
      clearAuth()
      window.location.href = '/login'
      return Promise.reject(error)
    }

    if (isRefreshing) {
      return new Promise((resolve, reject) => {
        failedQueue.push({ resolve, reject })
      }).then((token) => {
        originalRequest.headers.Authorization = `Bearer ${token}`
        return apiClient(originalRequest)
      })
    }

    originalRequest._retry = true
    isRefreshing = true

    try {
      const { data } = await axios.post(`${BASE_URL}/api/auth/refresh`, {
        refreshToken,
      })
      const { accessToken: newAccess, refreshToken: newRefresh } = data
      const currentUser = useAuthStore.getState().user
      if (!currentUser) {
        processQueue(new Error('Not authenticated'), null)
        clearAuth()
        window.location.href = '/login'
        return Promise.reject(new Error('Not authenticated'))
      }
      setAuth(currentUser, newAccess, newRefresh)
      processQueue(null, newAccess)
      originalRequest.headers.Authorization = `Bearer ${newAccess}`
      return apiClient(originalRequest)
    } catch (refreshError) {
      processQueue(refreshError, null)
      clearAuth()
      window.location.href = '/login'
      return Promise.reject(refreshError)
    } finally {
      isRefreshing = false
    }
  }
)
