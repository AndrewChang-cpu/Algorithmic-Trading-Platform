import { useEffect, useRef, useState } from 'react'
import { useAuthStore } from '../lib/store'

export interface LogEntry {
  level: 'INFO' | 'ERROR' | 'WARN' | string
  message: string
  timestamp: string
}

interface JobStatusState {
  status: string | null
  logs: LogEntry[]
  connected: boolean
}

const WS_BASE = import.meta.env.VITE_WS_URL ?? 'ws://localhost:8080'

export function useJobStatus(jobId: string | null): JobStatusState {
  const accessToken = useAuthStore((s) => s.accessToken)
  const [state, setState] = useState<JobStatusState>({
    status: null,
    logs: [],
    connected: false,
  })
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    if (!jobId || !accessToken) return

    const TERMINAL_STATUSES = new Set(['completed', 'failed'])

    function connect() {
      if (!mountedRef.current) return

      const ws = new WebSocket(
        `${WS_BASE}/api/stream/jobs/${jobId}?token=${encodeURIComponent(accessToken!)}`
      )
      wsRef.current = ws

      ws.onopen = () => {
        if (mountedRef.current) setState((s) => ({ ...s, connected: true }))
      }

      ws.onmessage = (event) => {
        if (!mountedRef.current) return
        try {
          const msg = JSON.parse(event.data as string)
          if (msg.type === 'status') {
            setState((s) => ({ ...s, status: msg.status as string }))
            if (TERMINAL_STATUSES.has(msg.status as string)) {
              ws.close(1000, 'job complete')
            }
          } else if (msg.type === 'log') {
            setState((s) => ({
              ...s,
              logs: [...s.logs.slice(-199), {
                level: msg.level as string,
                message: msg.message as string,
                timestamp: msg.timestamp as string,
              }],
            }))
          }
        } catch {
          // ignore parse errors
        }
      }

      ws.onclose = (event) => {
        if (!mountedRef.current) return
        setState((s) => ({ ...s, connected: false }))
        if (event.code !== 1000 && mountedRef.current) {
          reconnectTimer.current = setTimeout(connect, 2000)
        }
      }

      ws.onerror = () => {
        ws.close()
      }
    }

    connect()

    return () => {
      mountedRef.current = false
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      wsRef.current?.close(1000, 'unmount')
    }
  }, [jobId, accessToken])

  return state
}
