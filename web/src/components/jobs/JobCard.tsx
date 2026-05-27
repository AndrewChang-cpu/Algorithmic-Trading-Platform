import StatusBadge from './StatusBadge'

interface JobCardProps {
  id: string
  strategyName: string
  status: string
  latestEquity?: number | null
  runtimeSeconds?: number
  onClick?: () => void
  selected?: boolean
}

export default function JobCard({ strategyName, status, latestEquity, runtimeSeconds, onClick, selected }: JobCardProps) {
  const formatRuntime = (s: number) => {
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    const sec = s % 60
    return `${h.toString().padStart(2,'0')}:${m.toString().padStart(2,'0')}:${sec.toString().padStart(2,'0')}`
  }

  return (
    <div
      onClick={onClick}
      style={{
        padding: '12px 16px',
        background: selected ? '#21262d' : 'transparent',
        borderBottom: '1px solid #21262d',
        cursor: 'pointer',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        fontFamily: 'system-ui',
        fontSize: '13px',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span style={{ color: '#e6edf3', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '140px' }}>
          {strategyName}
        </span>
        <StatusBadge status={status} />
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', color: '#6e7681', fontSize: '12px' }}>
        <span style={{ fontFamily: 'monospace' }}>
          {latestEquity != null ? `$${latestEquity.toLocaleString()}` : '—'}
        </span>
        {runtimeSeconds != null && (
          <span style={{ fontFamily: 'monospace' }}>{formatRuntime(runtimeSeconds)}</span>
        )}
      </div>
    </div>
  )
}
