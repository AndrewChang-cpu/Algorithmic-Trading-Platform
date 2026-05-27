import { useState, FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useLogin } from '../hooks/useAuth'

export default function Login() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const { mutate: login, isPending, error } = useLogin()

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    login({ email, password })
  }

  const errorMsg = error
    ? (error as { response?: { data?: { error?: string } } }).response?.data?.error ?? 'Login failed'
    : null

  return (
    <div style={{
      minHeight: '100vh',
      background: '#0d1117',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontFamily: '-apple-system, BlinkMacSystemFont, "Inter", system-ui, sans-serif',
      fontSize: '13px',
      color: '#e6edf3',
    }}>
      <div style={{
        background: '#161b22',
        border: '1px solid #21262d',
        borderRadius: '6px',
        padding: '32px',
        width: '340px',
      }}>
        <h1 style={{ margin: '0 0 24px', fontSize: '20px', fontWeight: 600, textAlign: 'center' }}>
          Algorithmic Trading Platform
        </h1>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: '16px' }}>
            <label style={{ display: 'block', marginBottom: '6px', color: '#c9d1d9' }}>
              Email
            </label>
            <input
              type="email"
              value={email}
              onChange={e => setEmail(e.target.value)}
              disabled={isPending}
              required
              style={{
                width: '100%',
                padding: '8px 12px',
                background: '#0d1117',
                border: '1px solid #30363d',
                borderRadius: '6px',
                color: '#e6edf3',
                fontSize: '13px',
                boxSizing: 'border-box',
                outline: 'none',
              }}
            />
          </div>

          <div style={{ marginBottom: '16px' }}>
            <label style={{ display: 'block', marginBottom: '6px', color: '#c9d1d9' }}>
              Password
            </label>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              disabled={isPending}
              required
              style={{
                width: '100%',
                padding: '8px 12px',
                background: '#0d1117',
                border: '1px solid #30363d',
                borderRadius: '6px',
                color: '#e6edf3',
                fontSize: '13px',
                boxSizing: 'border-box',
                outline: 'none',
              }}
            />
          </div>

          {errorMsg && (
            <div style={{
              marginBottom: '16px',
              padding: '8px 12px',
              background: '#3d1f1f',
              border: '1px solid #f85149',
              borderRadius: '6px',
              color: '#f85149',
              fontSize: '12px',
            }}>
              {errorMsg}
            </div>
          )}

          <button
            type="submit"
            disabled={isPending}
            style={{
              width: '100%',
              padding: '8px',
              background: isPending ? '#1f6feb' : '#388bfd',
              border: 'none',
              borderRadius: '6px',
              color: '#fff',
              fontSize: '13px',
              fontWeight: 500,
              cursor: isPending ? 'not-allowed' : 'pointer',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: '8px',
            }}
          >
            {isPending ? (
              <>
                <span style={{
                  width: '14px', height: '14px',
                  border: '2px solid rgba(255,255,255,0.3)',
                  borderTopColor: '#fff',
                  borderRadius: '50%',
                  display: 'inline-block',
                  animation: 'spin 0.7s linear infinite',
                }} />
                Signing in...
              </>
            ) : 'Sign in'}
          </button>
        </form>

        <p style={{ marginTop: '16px', textAlign: 'center', color: '#6e7681' }}>
          Don't have an account?{' '}
          <Link to="/register" style={{ color: '#388bfd', textDecoration: 'none' }}>
            Create one
          </Link>
        </p>
      </div>

      <style>{`
        @keyframes spin { to { transform: rotate(360deg); } }
        input:focus { border-color: #388bfd !important; }
      `}</style>
    </div>
  )
}
