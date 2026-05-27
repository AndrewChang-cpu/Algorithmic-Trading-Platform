import { useEffect, useRef, useState } from 'react'
import { useAuthStore } from '../lib/store'

export interface SnapshotEntry {
  type: 'snapshot'
  time: string
  equity: number
  unrealized: number
  holdings: number
  fees: number
}

interface PortfolioState {
  snapshots: SnapshotEntry[]
  latestEquity: number | null
  connected: boolean
}

const WS_BASE = import.meta.env.VITE_WS_URL ?? 'ws://localhost:8080'

export function usePortfolio(jobId: string | null): PortfolioState {
  const accessToken = useAuthStore((s) => s.accessToken)
  const [state, setState] = useState<PortfolioState>({
    snapshots: [],
    latestEquity: null,
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

    function connect() {
      if (!mountedRef.current) return

      const ws = new WebSocket(
        `${WS_BASE}/api/stream/portfolio/${jobId}?token=${encodeURIComponent(accessToken!)}`
      )
      wsRef.current = ws

      ws.onopen = () => {
        if (mountedRef.current) setState((s) => ({ ...s, connected: true }))
      }

      ws.onmessage = (event) => {
        if (!mountedRef.current) return
        try {
          const msg = JSON.parse(event.data as string) as SnapshotEntry
          if (msg.type === 'snapshot') {
            setState((s) => ({
              snapshots: [...s.snapshots.slice(-499), msg],
              latestEquity: msg.equity,
              connected: s.connected,
            }))
          }
        } catch {
          // ignore
        }
      }

      ws.onclose = (event) => {
        if (!mountedRef.current) return
        setState((s) => ({ ...s, connected: false }))
        if (event.code !== 1000 && mountedRef.current) {
          reconnectTimer.current = setTimeout(connect, 2000)
        }
      }

      ws.onerror = () => ws.close()
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
