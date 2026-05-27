import { useEffect, useRef } from 'react'
import type { LogEntry } from '../../hooks/useJobStatus'

interface LogStreamProps {
  logs: LogEntry[]
}

const LEVEL_COLOR: Record<string, string> = {
  ERROR: '#f85149',
  WARN:  '#d29922',
  INFO:  '#8b949e',
}

export default function LogStream({ logs }: LogStreamProps) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logs.length])

  return (
    <div style={{
      background: '#0d1117',
      border: '1px solid #21262d',
      borderRadius: '6px',
      overflow: 'hidden',
    }}>
      <div style={{
        padding: '6px 12px',
        borderBottom: '1px solid #21262d',
        color: '#6e7681',
        fontSize: '11px',
        fontWeight: 500,
      }}>
        LOGS
      </div>
      <div style={{ maxHeight: '220px', overflowY: 'auto', padding: '8px 12px' }}>
        {logs.length === 0 ? (
          <span style={{ color: '#484f58', fontSize: '11px', fontFamily: 'monospace' }}>
            No logs yet...
          </span>
        ) : (
          logs.map((log, i) => (
            <div
              key={i}
              style={{
                fontFamily: 'monospace',
                fontSize: '11px',
                lineHeight: '18px',
                display: 'flex',
                gap: '8px',
              }}
            >
              <span style={{ color: '#484f58', flexShrink: 0 }}>
                {new Date(log.timestamp).toLocaleTimeString()}
              </span>
              <span style={{ color: LEVEL_COLOR[log.level] ?? '#8b949e', flexShrink: 0, width: '40px' }}>
                {log.level}
              </span>
              <span style={{ color: '#8b949e', wordBreak: 'break-all' }}>{log.message}</span>
            </div>
          ))
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
