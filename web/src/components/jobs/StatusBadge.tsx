const STATUS_STYLES: Record<string, { bg: string; color: string; label: string }> = {
  queued:    { bg: '#161b22', color: '#6e7681', label: 'Queued' },
  running:   { bg: '#1c2d24', color: '#3fb950', label: 'Running' },
  completed: { bg: '#1c2d24', color: '#3fb950', label: 'Completed' },
  failed:    { bg: '#3d1f1f', color: '#f85149', label: 'Failed' },
}

interface StatusBadgeProps {
  status: string
}

export default function StatusBadge({ status }: StatusBadgeProps) {
  const style = STATUS_STYLES[status] ?? { bg: '#21262d', color: '#8b949e', label: status }
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: '5px',
      padding: '2px 8px', borderRadius: '12px',
      background: style.bg, color: style.color,
      fontSize: '11px', fontWeight: 500,
    }}>
      {status === 'running' && (
        <span style={{
          width: '6px', height: '6px', borderRadius: '50%',
          background: '#3fb950', display: 'inline-block',
          animation: 'pulse 1.5s ease-in-out infinite',
        }} />
      )}
      {style.label}
      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>
    </span>
  )
}
