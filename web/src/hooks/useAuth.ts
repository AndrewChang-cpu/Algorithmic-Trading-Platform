import { useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { apiClient } from '../lib/api'
import { useAuthStore } from '../lib/store'

interface AuthResponse {
  accessToken: string
  userId: string
}

export function useLogin() {
  const setAuth = useAuthStore((s) => s.setAuth)
  const navigate = useNavigate()

  return useMutation({
    mutationFn: (req: { email: string; password: string }) =>
      apiClient.post<AuthResponse>('/api/auth/login', req).then((r) => r.data),
    onSuccess: (data, req) => {
      setAuth({ id: data.userId, email: req.email }, data.accessToken)
      navigate('/overview')
    },
  })
}

export function useRegister() {
  const setAuth = useAuthStore((s) => s.setAuth)
  const navigate = useNavigate()

  return useMutation({
    mutationFn: (req: { email: string; password: string }) =>
      apiClient.post<AuthResponse>('/api/auth/register', req).then((r) => r.data),
    onSuccess: (data, req) => {
      setAuth({ id: data.userId, email: req.email }, data.accessToken)
      navigate('/overview')
    },
  })
}

export function useLogout() {
  const clearAuth = useAuthStore((s) => s.clearAuth)
  const navigate = useNavigate()

  return useMutation({
    mutationFn: () =>
      apiClient.post('/api/auth/logout').then((r) => r.data),
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
