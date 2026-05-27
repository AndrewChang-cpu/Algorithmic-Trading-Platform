import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '../lib/api'
import Topbar from '../components/layout/Topbar'
import StatusBadge from '../components/jobs/StatusBadge'

interface Job {
  id: string
  strategyName: string
  versionNumber: number
  type: string
  status: string
  dataSource: string
  symbols: string[]
  resolution: string
  createdAt: string
  startedAt?: string
  completedAt?: string
  errorMessage?: string
}

interface JobList {
  jobs: Job[]
  total: number
}

const STATUSES = ['all', 'queued', 'running', 'completed', 'failed']
const PAGE_SIZE = 20

export default function Backtests() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const [page, setPage] = useState(1)

  const statusFilter = searchParams.get('status') ?? 'all'

  const { data, isLoading } = useQuery<JobList>({
    queryKey: ['jobs', 'backtest', statusFilter, page],
    queryFn: () => {
      const params: Record<string, string | number> = {
        type: 'backtest',
        page,
        limit: PAGE_SIZE,
      }
      if (statusFilter !== 'all') params.status = statusFilter
      return apiClient.get('/api/jobs', { params }).then(r => r.data)
    },
  })

  const totalPages = Math.ceil((data?.total ?? 0) / PAGE_SIZE)

  const cell: React.CSSProperties = {
    padding: '10px 16px',
    borderBottom: '1px solid #21262d',
    color: '#c9d1d9',
    verticalAlign: 'middle',
    fontSize: '13px',
  }
  const hCell: React.CSSProperties = {
    ...cell,
    color: '#6e7681',
    fontWeight: 500,
    fontSize: '12px',
    background: '#161b22',
  }

  return (
    <div style={{ background: '#0d1117', minHeight: '100vh', color: '#e6edf3', fontFamily: 'system-ui', fontSize: '13px' }}>
      <Topbar title="Backtests" />

      <div style={{ padding: '24px' }}>
        {/* Status filter tabs */}
        <div style={{ display: 'flex', gap: '4px', marginBottom: '20px', background: '#161b22', padding: '4px', borderRadius: '6px', border: '1px solid #21262d', width: 'fit-content' }}>
          {STATUSES.map(s => (
            <button
              key={s}
              onClick={() => { setSearchParams(s === 'all' ? {} : { status: s }); setPage(1) }}
              style={{
                padding: '5px 14px',
                background: statusFilter === s ? '#21262d' : 'transparent',
                border: 'none',
                borderRadius: '4px',
                color: statusFilter === s ? '#e6edf3' : '#6e7681',
                cursor: 'pointer',
                fontSize: '12px',
                fontWeight: statusFilter === s ? 500 : 400,
                textTransform: 'capitalize',
              }}
            >
              {s === 'all' ? 'All' : s.charAt(0).toUpperCase() + s.slice(1)}
            </button>
          ))}
        </div>

        {isLoading && (
          <div style={{ color: '#6e7681', padding: '48px', textAlign: 'center' }}>Loading...</div>
        )}

        {!isLoading && (!data?.jobs || data.jobs.length === 0) && (
          <div style={{
            background: '#161b22', border: '1px solid #21262d', borderRadius: '8px',
            padding: '64px', textAlign: 'center', color: '#6e7681',
          }}>
            No backtests yet. Select a strategy and run your first backtest.
          </div>
        )}

        {data?.jobs && data.jobs.length > 0 && (
          <>
            <div style={{ background: '#161b22', border: '1px solid #21262d', borderRadius: '6px', overflow: 'hidden' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    {['Strategy', 'Version', 'Status', 'Symbols', 'Resolution', 'Data Source', 'Date', 'Duration'].map(h => (
                      <th key={h} style={hCell}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {data.jobs.map(job => {
                    const duration = job.startedAt && job.completedAt
                      ? Math.round((new Date(job.completedAt).getTime() - new Date(job.startedAt).getTime()) / 1000)
                      : null

                    return (
                      <tr
                        key={job.id}
                        onClick={() => navigate(`/results/${job.id}`)}
                        style={{ cursor: 'pointer' }}
                        onMouseEnter={e => (e.currentTarget.style.background = '#1c2128')}
                        onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
                      >
                        <td style={{ ...cell, color: '#e6edf3', fontWeight: 500 }}>{job.strategyName}</td>
                        <td style={cell}>
                          <span style={{ fontFamily: 'monospace', fontSize: '11px', background: '#21262d', padding: '2px 6px', borderRadius: '4px', color: '#8b949e' }}>
                            v{job.versionNumber}
                          </span>
                        </td>
                        <td style={cell}><StatusBadge status={job.status} /></td>
                        <td style={{ ...cell, fontFamily: 'monospace', fontSize: '11px' }}>{job.symbols?.join(', ')}</td>
                        <td style={{ ...cell, fontFamily: 'monospace', fontSize: '11px' }}>{job.resolution}</td>
                        <td style={{ ...cell, color: '#8b949e' }}>{job.dataSource}</td>
                        <td style={{ ...cell, color: '#6e7681', whiteSpace: 'nowrap' }}>
                          {new Date(job.createdAt).toLocaleDateString()}
                        </td>
                        <td style={{ ...cell, fontFamily: 'monospace', fontSize: '11px', color: '#6e7681' }}>
                          {duration != null ? `${duration}s` : '—'}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: '8px', marginTop: '16px' }}>
                <span style={{ color: '#6e7681', fontSize: '12px' }}>
                  Page {page} of {totalPages} ({data.total} total)
                </span>
                <button
                  disabled={page === 1}
                  onClick={() => setPage(p => p - 1)}
                  style={{ padding: '4px 10px', background: '#21262d', border: '1px solid #30363d', borderRadius: '5px', color: page === 1 ? '#6e7681' : '#e6edf3', cursor: page === 1 ? 'not-allowed' : 'pointer', fontSize: '12px' }}
                >
                  Prev
                </button>
                <button
                  disabled={page === totalPages}
                  onClick={() => setPage(p => p + 1)}
                  style={{ padding: '4px 10px', background: '#21262d', border: '1px solid #30363d', borderRadius: '5px', color: page === totalPages ? '#6e7681' : '#e6edf3', cursor: page === totalPages ? 'not-allowed' : 'pointer', fontSize: '12px' }}
                >
                  Next
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
