import { useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { apiClient } from '../lib/api'
import { useAuthStore } from '../lib/store'

interface AuthResponse {
  accessToken: string
  refreshToken: string
  userId: string
}

export function useLogin() {
  const setAuth = useAuthStore((s) => s.setAuth)
  const navigate = useNavigate()

  return useMutation({
    mutationFn: (data: { email: string; password: string }) =>
      apiClient.post<AuthResponse>('/api/auth/login', data).then((r) => r.data),
    onSuccess: (data) => {
      setAuth({ id: data.userId, email: '' }, data.accessToken, data.refreshToken)
      navigate('/overview')
    },
  })
}

export function useRegister() {
  const setAuth = useAuthStore((s) => s.setAuth)
  const navigate = useNavigate()

  return useMutation({
    mutationFn: (data: { email: string; password: string }) =>
      apiClient.post<AuthResponse>('/api/auth/register', data).then((r) => r.data),
    onSuccess: (data) => {
      setAuth({ id: data.userId, email: '' }, data.accessToken, data.refreshToken)
      navigate('/overview')
    },
  })
}

export function useLogout() {
  const { refreshToken, clearAuth } = useAuthStore()
  const navigate = useNavigate()

  return useMutation({
    mutationFn: () =>
      apiClient.post('/api/auth/logout', { refreshToken }).then((r) => r.data),
    onSuccess: () => {
      clearAuth()
      navigate('/login')
    },
    onError: () => {
      clearAuth()
      navigate('/login')
    },
  })
}
