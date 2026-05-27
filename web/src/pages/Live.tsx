import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '../lib/api'
import Topbar from '../components/layout/Topbar'
import JobCard from '../components/jobs/JobCard'
import GoLiveModal from '../components/jobs/GoLiveModal'
import { usePortfolio } from '../hooks/usePortfolio'
import { useJobStatus } from '../hooks/useJobStatus'

interface LiveJob {
  id: string
  strategyName: string
  versionNumber: number
  strategyVersionId: string
  status: string
  createdAt: string
}

interface Strategy {
  id: string
  name: string
  latestVersion: number
  versions?: { id: string; versionNumber: number }[]
}

export default function Live() {
  const queryClient = useQueryClient()
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null)
  const [showGoLive, setShowGoLive] = useState(false)
  const [selectedStrategyVersionId, setSelectedStrategyVersionId] = useState<string | null>(null)
  const [showStrategyPicker, setShowStrategyPicker] = useState(false)

  const { data: liveJobList } = useQuery<{ jobs: LiveJob[] }>({
    queryKey: ['jobs', 'live', 'running'],
    queryFn: () => apiClient.get('/api/jobs', { params: { type: 'live', status: 'running' } }).then(r => r.data),
    refetchInterval: 5000,
  })

  const { data: strategies } = useQuery<Strategy[]>({
    queryKey: ['strategies'],
    queryFn: () => apiClient.get('/api/strategies').then(r => r.data),
    enabled: showStrategyPicker,
  })

  const cancelMutation = useMutation({
    mutationFn: (jobId: string) => apiClient.post(`/api/jobs/${jobId}/cancel`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['jobs', 'live'] }),
  })

  const liveJobs = liveJobList?.jobs ?? []
  const selectedJob = liveJobs.find(j => j.id === selectedJobId) ?? liveJobs[0] ?? null

  const { snapshots, latestEquity, connected: portfolioConnected } = usePortfolio(selectedJob?.id ?? null)
  const { logs } = useJobStatus(selectedJob?.id ?? null)

  const latestSnapshot = snapshots[snapshots.length - 1]

  const statBox = (label: string, value: string) => (
    <div style={{ background: '#161b22', border: '1px solid #21262d', borderRadius: '6px', padding: '10px 14px', minWidth: '120px' }}>
      <div style={{ color: '#6e7681', fontSize: '11px', marginBottom: '3px' }}>{label}</div>
      <div style={{ fontFamily: 'monospace', fontWeight: 600, fontSize: '15px' }}>{value}</div>
    </div>
  )

  const handleGoLiveClick = () => {
    setShowStrategyPicker(true)
  }

  const handleStrategyPick = (versionId: string) => {
    setSelectedStrategyVersionId(versionId)
    setShowStrategyPicker(false)
    setShowGoLive(true)
  }

  return (
    <div style={{ background: '#0d1117', minHeight: '100vh', color: '#e6edf3', fontFamily: 'system-ui', fontSize: '13px' }}>
      <Topbar title="Live Trading" />

      <div style={{ display: 'flex', height: 'calc(100vh - 46px)' }}>
        {/* Left panel: job list */}
        <div style={{ width: '280px', minWidth: '280px', borderRight: '1px solid #21262d', display: 'flex', flexDirection: 'column' }}>
          <div style={{ flex: 1, overflowY: 'auto' }}>
            {liveJobs.length === 0 && (
              <div style={{ padding: '32px 16px', color: '#6e7681', textAlign: 'center' }}>
                No live strategies running.
              </div>
            )}
            {liveJobs.map(job => (
              <JobCard
                key={job.id}
                id={job.id}
                strategyName={job.strategyName}
                status={job.status}
                latestEquity={job.id === selectedJob?.id ? latestEquity : undefined}
                selected={job.id === selectedJobId}
                onClick={() => setSelectedJobId(job.id)}
              />
            ))}
          </div>
          <div style={{ padding: '12px', borderTop: '1px solid #21262d' }}>
            <button
              onClick={handleGoLiveClick}
              style={{ width: '100%', padding: '8px', background: '#3fb950', border: 'none', borderRadius: '6px', color: '#fff', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}
            >
              + Go Live
            </button>
          </div>
        </div>

        {/* Right panel: selected job detail */}
        <div style={{ flex: 1, overflow: 'auto', padding: '24px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {!selectedJob ? (
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#6e7681' }}>
              No live strategies running. Click &quot;+ Go Live&quot; to start one.
            </div>
          ) : (
            <>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div>
                  <span style={{ fontWeight: 600, fontSize: '16px' }}>{selectedJob.strategyName}</span>
                  <span style={{ color: '#6e7681', marginLeft: '8px', fontSize: '12px' }}>
                    {portfolioConnected ? 'Live' : 'Connecting...'}
                  </span>
                </div>
                <button
                  onClick={() => {
                    if (window.confirm('Stop this live trading job?')) {
                      cancelMutation.mutate(selectedJob.id)
                    }
                  }}
                  style={{ padding: '6px 14px', background: 'transparent', border: '1px solid #3d1f1f', borderRadius: '6px', color: '#f85149', cursor: 'pointer', fontSize: '12px' }}
                >
                  Stop
                </button>
              </div>

              {/* Stats bar */}
              <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
                {statBox('Equity', latestSnapshot ? `$${parseFloat(String(latestSnapshot.equity)).toLocaleString()}` : '—')}
                {statBox('Unrealized P&L', latestSnapshot ? `$${parseFloat(String(latestSnapshot.unrealized)).toLocaleString()}` : '—')}
                {statBox('Holdings', latestSnapshot ? `$${parseFloat(String(latestSnapshot.holdings)).toLocaleString()}` : '—')}
                {statBox('Fees', latestSnapshot ? `$${Math.abs(parseFloat(String(latestSnapshot.fees))).toLocaleString()}` : '—')}
              </div>

              {/* Positions placeholder */}
              <div style={{ background: '#161b22', border: '1px solid #21262d', borderRadius: '6px', padding: '16px', color: '#6e7681', fontSize: '12px' }}>
                Position data not available in MVP
              </div>

              {/* Log stream */}
              <div style={{ background: '#0d1117', border: '1px solid #21262d', borderRadius: '6px', overflow: 'hidden' }}>
                <div style={{ padding: '8px 12px', borderBottom: '1px solid #21262d', color: '#6e7681', fontSize: '11px', fontWeight: 500 }}>LOGS</div>
                <pre style={{
                  margin: 0, padding: '12px', maxHeight: '200px', overflow: 'auto',
                  fontFamily: 'monospace', fontSize: '11px', color: '#8b949e',
                  background: 'transparent',
                }}>
                  {logs.length === 0 ? 'No logs yet...' : logs.slice(-50).map(l =>
                    `[${l.timestamp}] ${l.level}: ${l.message}`
                  ).join('\n')}
                </pre>
              </div>
            </>
          )}
        </div>
      </div>

      {/* Strategy picker modal */}
      {showStrategyPicker && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
          <div style={{ background: '#161b22', border: '1px solid #30363d', borderRadius: '8px', padding: '24px', width: '380px', fontSize: '13px' }}>
            <div style={{ fontWeight: 600, fontSize: '15px', marginBottom: '16px' }}>Select a Strategy</div>
            {!strategies?.length ? (
              <div style={{ color: '#6e7681' }}>No strategies found. Upload one first.</div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {strategies.map(s => (
                  <div
                    key={s.id}
                    onClick={() => handleStrategyPick(s.versions?.[0]?.id ?? '')}
                    style={{ padding: '10px', background: '#0d1117', border: '1px solid #21262d', borderRadius: '6px', cursor: 'pointer', display: 'flex', justifyContent: 'space-between' }}
                  >
                    <span>{s.name}</span>
                    <span style={{ color: '#6e7681', fontSize: '11px', fontFamily: 'monospace' }}>v{s.latestVersion}</span>
                  </div>
                ))}
              </div>
            )}
            <button onClick={() => setShowStrategyPicker(false)} style={{ marginTop: '16px', padding: '6px 14px', background: '#21262d', border: 'none', borderRadius: '6px', color: '#e6edf3', cursor: 'pointer' }}>
              Cancel
            </button>
          </div>
        </div>
      )}

      {showGoLive && selectedStrategyVersionId && (
        <GoLiveModal
          strategyVersionId={selectedStrategyVersionId}
          onClose={() => { setShowGoLive(false); setSelectedStrategyVersionId(null) }}
        />
      )}
    </div>
  )
}
