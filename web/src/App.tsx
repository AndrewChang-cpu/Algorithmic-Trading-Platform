import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { lazy, Suspense, useEffect } from 'react'
import { useAuthStore } from './lib/store'
import { ErrorBoundary } from './components/ErrorBoundary'
import Sidebar from './components/layout/Sidebar'
import Login from './pages/Login'
import Register from './pages/Register'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
})

function PlaceholderPage({ title }: { title: string }) {
  return (
    <div style={{ padding: '24px', color: '#e6edf3', fontFamily: 'system-ui' }}>
      <h2 style={{ margin: 0, fontSize: '18px', fontWeight: 600 }}>{title}</h2>
      <p style={{ color: '#6e7681', marginTop: '8px' }}>Coming soon</p>
    </div>
  )
}

// Lazy-load pages with fallback to placeholder for pages not yet created
const Overview = lazy(() =>
  import(/* @vite-ignore */ './pages/Overview').catch(() => ({
    default: () => <PlaceholderPage title="Overview" />,
  }))
)
const Strategies = lazy(() =>
  import(/* @vite-ignore */ './pages/Strategies').catch(() => ({
    default: () => <PlaceholderPage title="Strategies" />,
  }))
)
const StrategyDetail = lazy(() =>
  import(/* @vite-ignore */ './pages/StrategyDetail').catch(() => ({
    default: () => <PlaceholderPage title="Strategy Detail" />,
  }))
)
const Backtests = lazy(() =>
  import(/* @vite-ignore */ './pages/Backtests').catch(() => ({
    default: () => <PlaceholderPage title="Backtests" />,
  }))
)
const Results = lazy(() =>
  import(/* @vite-ignore */ './pages/Results').catch(() => ({
    default: () => <PlaceholderPage title="Results" />,
  }))
)
const Live = lazy(() =>
  import(/* @vite-ignore */ './pages/Live').catch(() => ({
    default: () => <PlaceholderPage title="Live" />,
  }))
)

const fallback = <div style={{ padding: '24px', color: '#6e7681' }}>Loading...</div>

function AppLayout() {
  return (
    <div style={{ display: 'flex', background: '#0d1117', minHeight: '100vh' }}>
      <Sidebar />
      <div style={{ marginLeft: '220px', flex: 1, display: 'flex', flexDirection: 'column' }}>
        <Outlet />
      </div>
    </div>
  )
}

function PrivateRoute() {
  const accessToken = useAuthStore((s) => s.accessToken)
  return accessToken ? <Outlet /> : <Navigate to="/login" replace />
}

export default function App() {
  useEffect(() => {
    const ch = new BroadcastChannel('auth')
    ch.onmessage = (e) => {
      if (e.data.type === 'token_refresh') {
        const { user } = useAuthStore.getState()
        if (user) {
          useAuthStore.getState().setAuth(user, e.data.accessToken, e.data.refreshToken)
        }
      }
    }
    return () => ch.close()
  }, [])

  return (
    <ErrorBoundary>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          {/* Public routes */}
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />

          {/* Protected routes */}
          <Route element={<PrivateRoute />}>
            <Route element={<AppLayout />}>
              <Route path="/" element={<Navigate to="/overview" replace />} />
              <Route path="/overview" element={<Suspense fallback={fallback}><Overview /></Suspense>} />
              <Route path="/strategies" element={<Suspense fallback={fallback}><Strategies /></Suspense>} />
              <Route path="/strategies/:id" element={<Suspense fallback={fallback}><StrategyDetail /></Suspense>} />
              <Route path="/backtests" element={<Suspense fallback={fallback}><Backtests /></Suspense>} />
              <Route path="/results/:jobId" element={<Suspense fallback={fallback}><Results /></Suspense>} />
              <Route path="/live" element={<Suspense fallback={fallback}><Live /></Suspense>} />
            </Route>
          </Route>

          {/* Catch-all */}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
    </ErrorBoundary>
  )
}
