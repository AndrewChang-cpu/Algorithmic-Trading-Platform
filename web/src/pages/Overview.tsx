import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { apiClient } from '../lib/api'
import Topbar from '../components/layout/Topbar'
import StatusBadge from '../components/jobs/StatusBadge'

interface HealthStatus {
  kafka: string
  redis: string
  db: string
}

interface Job {
  id: string
  strategyName: string
  type: string
  status: string
  createdAt: string
}

interface JobList {
  jobs: Job[]
  total: number
}

function HealthDot({ status }: { status: string }) {
  const color = status === 'ok' ? '#3fb950' : status === 'down' ? '#f85149' : '#d29922'
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: '6px',
      padding: '4px 10px', background: '#161b22', borderRadius: '12px',
      border: `1px solid ${color}22`, fontSize: '12px', color: '#c9d1d9',
    }}>
      <span style={{ width: '8px', height: '8px', borderRadius: '50%', background: color, display: 'inline-block' }} />
      {status}
    </span>
  )
}

export default function Overview() {
  const navigate = useNavigate()

  const { data: health } = useQuery<HealthStatus>({
    queryKey: ['health'],
    queryFn: () => apiClient.get('/api/health').then(r => r.data),
    refetchInterval: 30000,
    retry: false,
  })

  const { data: allJobs } = useQuery<JobList>({
    queryKey: ['jobs', 'all', 'recent'],
    queryFn: () => apiClient.get('/api/jobs', { params: { limit: 10 } }).then(r => r.data),
  })

  const { data: strategies } = useQuery<{ total: number }>({
    queryKey: ['strategies-count'],
    queryFn: () => apiClient.get('/api/strategies').then(r => ({ total: r.data.length ?? 0 })),
  })

  const activeJobs = allJobs?.jobs.filter(j => j.status === 'running').length ?? 0

  const card: React.CSSProperties = {
    background: '#161b22', border: '1px solid #21262d', borderRadius: '8px', padding: '16px 20px',
  }
  const cell: React.CSSProperties = {
    padding: '10px 16px', borderBottom: '1px solid #21262d', color: '#c9d1d9', fontSize: '13px', verticalAlign: 'middle',
  }

  return (
    <div style={{ background: '#0d1117', minHeight: '100vh', color: '#e6edf3', fontFamily: 'system-ui', fontSize: '13px' }}>
      <Topbar title="Overview" />

      <div style={{ padding: '24px', display: 'flex', flexDirection: 'column', gap: '24px' }}>
        {/* System health bar */}
        <div style={{ ...card, display: 'flex', alignItems: 'center', gap: '16px' }}>
          <span style={{ color: '#6e7681', fontSize: '12px', fontWeight: 500 }}>System Health</span>
          <div style={{ display: 'flex', gap: '8px' }}>
            <span style={{ color: '#6e7681', fontSize: '12px' }}>Kafka</span>
            <HealthDot status={health?.kafka ?? 'unknown'} />
          </div>
          <div style={{ display: 'flex', gap: '8px' }}>
            <span style={{ color: '#6e7681', fontSize: '12px' }}>Redis</span>
            <HealthDot status={health?.redis ?? 'unknown'} />
          </div>
          <div style={{ display: 'flex', gap: '8px' }}>
            <span style={{ color: '#6e7681', fontSize: '12px' }}>Database</span>
            <HealthDot status={health?.db ?? 'unknown'} />
          </div>
        </div>

        {/* Summary cards */}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '16px' }}>
          <div
            style={{ ...card, cursor: 'pointer' }}
            onClick={() => navigate('/backtests?status=running')}
          >
            <div style={{ color: '#6e7681', fontSize: '11px', marginBottom: '6px' }}>Active Jobs</div>
            <div style={{ fontSize: '28px', fontWeight: 600, fontFamily: 'monospace', color: activeJobs > 0 ? '#3fb950' : '#e6edf3' }}>
              {activeJobs}
            </div>
          </div>
          <div style={card}>
            <div style={{ color: '#6e7681', fontSize: '11px', marginBottom: '6px' }}>Strategies</div>
            <div style={{ fontSize: '28px', fontWeight: 600, fontFamily: 'monospace' }}>
              {strategies?.total ?? '—'}
            </div>
          </div>
          <div style={card}>
            <div style={{ color: '#6e7681', fontSize: '11px', marginBottom: '6px' }}>Total Backtests</div>
            <div style={{ fontSize: '28px', fontWeight: 600, fontFamily: 'monospace' }}>
              {allJobs?.total ?? '—'}
            </div>
          </div>
        </div>

        {/* Recent jobs table */}
        <div>
          <div style={{ color: '#6e7681', fontSize: '12px', fontWeight: 500, marginBottom: '12px' }}>Recent Jobs</div>
          {!allJobs?.jobs?.length ? (
            <div style={{ ...card, color: '#6e7681', textAlign: 'center', padding: '32px' }}>No jobs yet.</div>
          ) : (
            <div style={{ background: '#161b22', border: '1px solid #21262d', borderRadius: '6px', overflow: 'hidden' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    {['Strategy', 'Type', 'Status', 'Date'].map(h => (
                      <th key={h} style={{ ...cell, color: '#6e7681', fontWeight: 500, fontSize: '12px', background: '#161b22', textAlign: 'left' }}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {allJobs.jobs.map(job => (
                    <tr
                      key={job.id}
                      onClick={() => navigate(job.type === 'live' ? '/live' : `/results/${job.id}`)}
                      style={{ cursor: 'pointer' }}
                      onMouseEnter={e => (e.currentTarget.style.background = '#1c2128')}
                      onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
                    >
                      <td style={{ ...cell, color: '#e6edf3', fontWeight: 500 }}>{job.strategyName}</td>
                      <td style={cell}>{job.type}</td>
                      <td style={cell}><StatusBadge status={job.status} /></td>
                      <td style={{ ...cell, color: '#6e7681' }}>{new Date(job.createdAt).toLocaleDateString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
