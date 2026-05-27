import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '../lib/api'
import Topbar from '../components/layout/Topbar'
import EquityCurve from '../components/results/EquityCurve'
import HeroMetrics from '../components/results/HeroMetrics'
import MetricsTabs from '../components/results/MetricsTabs'
import LogStream from '../components/results/LogStream'
import StatusBadge from '../components/jobs/StatusBadge'
import { useJobStatus } from '../hooks/useJobStatus'

interface Job {
  id: string
  strategyName: string
  versionNumber: number
  status: string
  errorMessage?: string
  createdAt: string
}

interface PortfolioPoint {
  time: number
  open: number
  high: number
  low: number
  close: number
}

interface PortfolioResponse {
  points: PortfolioPoint[]
}

export default function Results() {
  const { jobId } = useParams<{ jobId: string }>()
  const { status: wsStatus, logs } = useJobStatus(jobId ?? null)

  const { data: job } = useQuery<Job>({
    queryKey: ['job', jobId],
    queryFn: () => apiClient.get<Job>(`/api/jobs/${jobId}`).then(r => r.data),
    refetchInterval: (query) => {
      const data = query.state.data
      if (!data) return 2000
      return ['completed', 'failed'].includes(data.status) ? false : 2000
    },
  })

  const jobStatus = wsStatus ?? job?.status ?? 'queued'
  const isRunning = jobStatus === 'running' || jobStatus === 'queued'
  const isFailed = jobStatus === 'failed'
  const isCompleted = jobStatus === 'completed'

  const { data: metrics } = useQuery<Record<string, unknown>>({
    queryKey: ['job-metrics', jobId],
    queryFn: () =>
      apiClient.get<Record<string, unknown>>(`/api/jobs/${jobId}/metrics`).then(r => r.data),
    enabled: isCompleted,
  })

  const { data: portfolioData } = useQuery<PortfolioResponse>({
    queryKey: ['job-portfolio', jobId],
    queryFn: () =>
      apiClient.get<PortfolioResponse>(`/api/jobs/${jobId}/portfolio`).then(r => r.data),
    enabled: isCompleted || isRunning,
    refetchInterval: isRunning ? 10000 : false,
  })

  const equityData = portfolioData?.points ?? []

  const handleExportJSON = () => {
    const blob = new Blob(
      [JSON.stringify({ metrics, portfolio: portfolioData }, null, 2)],
      { type: 'application/json' }
    )
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `backtest-${jobId}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div style={{
      background: '#0d1117',
      minHeight: '100vh',
      color: '#e6edf3',
      fontFamily: 'system-ui',
      fontSize: '13px',
    }}>
      <Topbar
        title={job ? `${job.strategyName} v${job.versionNumber}` : 'Backtest Results'}
        actions={
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <StatusBadge status={jobStatus} />
            {isCompleted && (
              <button
                onClick={handleExportJSON}
                style={{
                  padding: '5px 12px',
                  background: '#21262d',
                  border: '1px solid #30363d',
                  borderRadius: '5px',
                  color: '#e6edf3',
                  cursor: 'pointer',
                  fontSize: '12px',
                }}
              >
                Export JSON
              </button>
            )}
          </div>
        }
      />

      <div style={{ padding: '24px', display: 'flex', flexDirection: 'column', gap: '20px' }}>
        {isFailed && (
          <div style={{
            padding: '12px 16px',
            background: '#3d1f1f',
            border: '1px solid #f85149',
            borderRadius: '6px',
            color: '#f85149',
            fontWeight: 500,
          }}>
            Backtest failed: {job?.errorMessage ?? 'Unknown error'}
          </div>
        )}

        <div>
          <div style={{ color: '#6e7681', fontSize: '11px', fontWeight: 500, marginBottom: '8px' }}>
            EQUITY CURVE
          </div>
          {isRunning && !equityData.length ? (
            <div style={{
              height: '300px',
              background: '#161b22',
              border: '1px solid #21262d',
              borderRadius: '6px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#6e7681',
            }}>
              <div style={{ textAlign: 'center' }}>
                <div style={{
                  width: '20px',
                  height: '20px',
                  border: '2px solid #21262d',
                  borderTopColor: '#388bfd',
                  borderRadius: '50%',
                  animation: 'spin 0.7s linear infinite',
                  margin: '0 auto 8px',
                }} />
                Running...
              </div>
            </div>
          ) : (
            <EquityCurve data={equityData} />
          )}
        </div>

        <HeroMetrics metrics={metrics ?? null} isRunning={isRunning} />

        {(isCompleted || metrics != null) && <MetricsTabs metrics={metrics ?? null} />}

        <LogStream logs={logs} />
      </div>

      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}
