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

  // Single cleanup effect — avoids the race where re-renders reset mountedRef to true
  useEffect(() => () => { mountedRef.current = false }, [])

  useEffect(() => {
    if (!jobId || !accessToken) return

    function connect() {
      if (!mountedRef.current) return

      const ws = new WebSocket(`${WS_BASE}/api/stream/portfolio/${jobId}`)
      wsRef.current = ws

      ws.onopen = () => {
        // First message: authenticate
        ws.send(JSON.stringify({ type: 'auth', token: accessToken }))
        if (mountedRef.current) setState((s) => ({ ...s, connected: true }))
      }

      ws.onmessage = (event) => {
        if (!mountedRef.current) return
        try {
          const raw: unknown = JSON.parse(event.data as string)
          const msg = raw as Record<string, unknown>
          if (msg['type'] === 'auth_ok') return
          if (msg['type'] === 'snapshot') {
            const snapshot = raw as SnapshotEntry
            setState((s) => ({
              snapshots: [...s.snapshots.slice(-499), snapshot],
              latestEquity: snapshot.equity,
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
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      wsRef.current?.close(1000, 'unmount')
    }
  }, [jobId, accessToken])

  return state
}
