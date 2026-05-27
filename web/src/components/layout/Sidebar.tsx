import { NavLink } from 'react-router-dom'
import { useAuthStore } from '../../lib/store'
import { useLogout } from '../../hooks/useAuth'

const NAV_ITEMS = [
  { path: '/overview', label: 'Overview' },
  { path: '/strategies', label: 'Strategies' },
  { path: '/backtests', label: 'Backtests' },
  { path: '/live', label: 'Live Trading' },
]

export default function Sidebar() {
  const user = useAuthStore((s) => s.user)
  const { mutate: logout } = useLogout()

  return (
    <div style={{
      width: '220px',
      minWidth: '220px',
      background: '#161b22',
      borderRight: '1px solid #21262d',
      display: 'flex',
      flexDirection: 'column',
      height: '100vh',
      position: 'fixed',
      left: 0,
      top: 0,
      fontFamily: '-apple-system, BlinkMacSystemFont, "Inter", system-ui, sans-serif',
      fontSize: '13px',
    }}>
      {/* Logo */}
      <div style={{
        padding: '16px',
        borderBottom: '1px solid #21262d',
        fontWeight: 600,
        color: '#e6edf3',
        letterSpacing: '-0.3px',
      }}>
        ATP
      </div>

      {/* Nav */}
      <nav style={{ flex: 1, padding: '8px 0' }}>
        {NAV_ITEMS.map(({ path, label }) => (
          <NavLink
            key={path}
            to={path}
            style={({ isActive }) => ({
              display: 'block',
              padding: '8px 16px',
              color: isActive ? '#e6edf3' : '#8b949e',
              textDecoration: 'none',
              background: isActive ? '#21262d' : 'transparent',
              borderLeft: isActive ? '2px solid #388bfd' : '2px solid transparent',
              transition: 'all 0.1s',
            })}
          >
            {label}
          </NavLink>
        ))}
      </nav>

      {/* User section */}
      <div style={{
        padding: '12px 16px',
        borderTop: '1px solid #21262d',
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
      }}>
        <span style={{ color: '#6e7681', fontSize: '12px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {user?.email ?? ''}
        </span>
        <button
          onClick={() => logout()}
          style={{
            background: 'transparent',
            border: '1px solid #30363d',
            borderRadius: '6px',
            color: '#8b949e',
            fontSize: '12px',
            padding: '4px 8px',
            cursor: 'pointer',
            textAlign: 'left',
          }}
        >
          Sign out
        </button>
      </div>
    </div>
  )
}
