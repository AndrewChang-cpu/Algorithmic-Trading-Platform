import { ReactNode } from 'react'

interface TopbarProps {
  title: string
  actions?: ReactNode
}

export default function Topbar({ title, actions }: TopbarProps) {
  return (
    <div style={{
      height: '46px',
      background: '#0d1117',
      borderBottom: '1px solid #21262d',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      padding: '0 24px',
      fontFamily: '-apple-system, BlinkMacSystemFont, "Inter", system-ui, sans-serif',
      fontSize: '13px',
      color: '#e6edf3',
    }}>
      <span style={{ fontWeight: 600 }}>{title}</span>
      {actions && <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>{actions}</div>}
    </div>
  )
}
