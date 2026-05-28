import { useState, useEffect, lazy, Suspense } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '../lib/api'
import Topbar from '../components/layout/Topbar'
import VersionSelector from '../components/strategy/VersionSelector'
import CodeViewer from '../components/strategy/CodeViewer'
import UploadModal from '../components/strategy/UploadModal'

const RunBacktestModal = lazy(() =>
  import('../components/jobs/RunBacktestModal').catch(() => ({
    default: () => <div style={{ color: '#6e7681' }}>Loading...</div>,
  }))
)
const GoLiveModal = lazy(() =>
  import('../components/jobs/GoLiveModal').catch(() => ({
    default: () => <div style={{ color: '#6e7681' }}>Loading...</div>,
  }))
)

interface StrategyVersion {
  id: string
  versionNumber: number
  createdAt: string
}

interface StrategyDetail {
  id: string
  name: string
  description: string
  latestVersion: number
  versions: StrategyVersion[]
  stats: {
    runCount: number
    bestSharpe: number | null
    avgReturn: number | null
  }
}

interface JobRow {
  id: string
  versionNumber: number
  type: string
  status: string
  createdAt: string
  symbols: string[]
}

type Tab = 'code' | 'runs'

export default function StrategyDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [tab, setTab] = useState<Tab>('code')
  const [selectedVersionId, setSelectedVersionId] = useState<string>('')
  const [showUpload, setShowUpload] = useState(false)
  const [showBacktest, setShowBacktest] = useState(false)
  const [showLive, setShowLive] = useState(false)

  const { data: strategy, isLoading } = useQuery<StrategyDetail>({
    queryKey: ['strategy', id],
    queryFn: () => apiClient.get(`/api/strategies/${id}`).then(r => r.data),
  })

  useEffect(() => {
    if (strategy && !selectedVersionId && strategy.versions?.length) {
      setSelectedVersionId(strategy.versions[0].id)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [strategy])

  const { data: codeData } = useQuery<{ code: string }>({
    queryKey: ['strategy-code', id, selectedVersionId],
    queryFn: () =>
      apiClient.get(`/api/strategies/${id}/versions/${selectedVersionId}/code`).then(r => r.data),
    enabled: !!selectedVersionId,
  })

  const { data: jobs } = useQuery<JobRow[]>({
    queryKey: ['jobs', id],
    queryFn: () =>
      apiClient.get('/api/jobs', { params: { type: 'backtest' } }).then(r => r.data.jobs ?? []),
    enabled: tab === 'runs',
  })

  const deleteMutation = useMutation({
    mutationFn: () => apiClient.delete(`/api/strategies/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['strategies'] })
      navigate('/strategies')
    },
  })

  const handleDelete = () => {
    if (window.confirm(`Delete "${strategy?.name}" and all its runs? This cannot be undone.`)) {
      deleteMutation.mutate()
    }
  }

  const tabStyle = (active: boolean): React.CSSProperties => ({
    padding: '8px 16px',
    background: 'none',
    border: 'none',
    borderBottom: active ? '2px solid #388bfd' : '2px solid transparent',
    color: active ? '#e6edf3' : '#6e7681',
    cursor: 'pointer',
    fontSize: '13px',
    fontFamily: 'system-ui',
  })

  const cell: React.CSSProperties = {
    padding: '10px 16px', borderBottom: '1px solid #21262d',
    color: '#c9d1d9', verticalAlign: 'middle', fontSize: '13px',
  }

  if (isLoading) {
    return (
      <div style={{ background: '#0d1117', minHeight: '100vh', color: '#6e7681', padding: '48px', textAlign: 'center', fontFamily: 'system-ui' }}>
        Loading...
      </div>
    )
  }

  if (!strategy) return null

  const currentVersionId = selectedVersionId || strategy.versions?.[0]?.id || ''

  return (
    <div style={{ background: '#0d1117', minHeight: '100vh', color: '#e6edf3', fontFamily: 'system-ui', fontSize: '13px' }}>
      <Topbar
        title={strategy.name}
        actions={
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <VersionSelector
              versions={strategy.versions ?? []}
              selectedId={currentVersionId}
              onSelect={setSelectedVersionId}
            />
            <button onClick={() => setShowUpload(true)} style={{ padding: '6px 12px', background: '#21262d', border: '1px solid #30363d', borderRadius: '6px', color: '#e6edf3', cursor: 'pointer', fontSize: '12px' }}>
              Upload Version
            </button>
            <button onClick={() => setShowBacktest(true)} style={{ padding: '6px 12px', background: '#388bfd', border: 'none', borderRadius: '6px', color: '#fff', cursor: 'pointer', fontWeight: 500, fontSize: '12px' }}>
              Run Backtest
            </button>
            <button onClick={() => setShowLive(true)} style={{ padding: '6px 12px', background: '#3fb950', border: 'none', borderRadius: '6px', color: '#fff', cursor: 'pointer', fontWeight: 500, fontSize: '12px' }}>
              Go Live
            </button>
            <button onClick={handleDelete} style={{ padding: '6px 12px', background: 'transparent', border: '1px solid #3d1f1f', borderRadius: '6px', color: '#f85149', cursor: 'pointer', fontSize: '12px' }}>
              Delete
            </button>
          </div>
        }
      />

      {/* Stats bar */}
      <div style={{ padding: '12px 24px', borderBottom: '1px solid #21262d', display: 'flex', gap: '32px' }}>
        {[
          { label: 'Total Runs', value: strategy.stats?.runCount ?? 0 },
          { label: 'Best Sharpe', value: strategy.stats?.bestSharpe?.toFixed(2) ?? '—' },
          { label: 'Avg Return', value: strategy.stats?.avgReturn != null ? `${(strategy.stats.avgReturn * 100).toFixed(2)}%` : '—' },
        ].map(({ label, value }) => (
          <div key={label}>
            <div style={{ color: '#6e7681', fontSize: '11px', marginBottom: '2px' }}>{label}</div>
            <div style={{ fontFamily: 'monospace', fontWeight: 600 }}>{String(value)}</div>
          </div>
        ))}
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', borderBottom: '1px solid #21262d', padding: '0 24px' }}>
        <button style={tabStyle(tab === 'code')} onClick={() => setTab('code')}>Code</button>
        <button style={tabStyle(tab === 'runs')} onClick={() => setTab('runs')}>Runs</button>
      </div>

      <div style={{ padding: '24px' }}>
        {tab === 'code' && <CodeViewer code={codeData?.code ?? ''} />}

        {tab === 'runs' && (
          <div>
            {!jobs || jobs.length === 0 ? (
              <div style={{
                textAlign: 'center', padding: '48px',
                background: '#161b22', border: '1px solid #21262d', borderRadius: '8px',
                color: '#6e7681',
              }}>
                No runs yet. Run your first backtest.
              </div>
            ) : (
              <div style={{ background: '#161b22', border: '1px solid #21262d', borderRadius: '6px', overflow: 'hidden' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr>
                      {['Version', 'Type', 'Status', 'Symbols', 'Date'].map(h => (
                        <th key={h} style={{ padding: '10px 16px', textAlign: 'left', color: '#6e7681', fontWeight: 500, borderBottom: '1px solid #21262d', fontSize: '12px' }}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {jobs.map(job => (
                      <tr
                        key={job.id}
                        onClick={() => navigate(job.type === 'live' ? '/live' : `/results/${job.id}`)}
                        style={{ cursor: 'pointer' }}
                      >
                        <td style={cell}><span style={{ fontFamily: 'monospace', fontSize: '11px', background: '#21262d', padding: '2px 6px', borderRadius: '4px' }}>v{job.versionNumber}</span></td>
                        <td style={cell}>{job.type}</td>
                        <td style={cell}>{job.status}</td>
                        <td style={{ ...cell, fontFamily: 'monospace', fontSize: '11px' }}>{job.symbols?.join(', ')}</td>
                        <td style={{ ...cell, color: '#6e7681' }}>{new Date(job.createdAt).toLocaleDateString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </div>

      {showUpload && <UploadModal strategyId={id} onClose={() => setShowUpload(false)} />}
      {showBacktest && currentVersionId && (
        <Suspense fallback={null}>
          <RunBacktestModal strategyVersionId={currentVersionId} onClose={() => setShowBacktest(false)} />
        </Suspense>
      )}
      {showLive && currentVersionId && (
        <Suspense fallback={null}>
          <GoLiveModal strategyVersionId={currentVersionId} onClose={() => setShowLive(false)} />
        </Suspense>
      )}
    </div>
  )
}
