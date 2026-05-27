import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '../lib/api'
import Topbar from '../components/layout/Topbar'
import UploadModal from '../components/strategy/UploadModal'

interface Strategy {
  id: string
  name: string
  latestVersion: number
  runCount: number
  bestSharpe: number | null
  createdAt: string
}

export default function Strategies() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [showUpload, setShowUpload] = useState(false)

  const { data: strategies, isLoading } = useQuery<Strategy[]>({
    queryKey: ['strategies'],
    queryFn: () => apiClient.get('/api/strategies').then(r => r.data),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/api/strategies/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['strategies'] }),
  })

  const handleDelete = (id: string, name: string) => {
    if (window.confirm(`Delete "${name}" and all its runs? This cannot be undone.`)) {
      deleteMutation.mutate(id)
    }
  }

  const cell: React.CSSProperties = {
    padding: '10px 16px', borderBottom: '1px solid #21262d', color: '#c9d1d9', verticalAlign: 'middle',
  }
  const headerCell: React.CSSProperties = {
    padding: '10px 16px', textAlign: 'left', color: '#6e7681', fontWeight: 500,
    borderBottom: '1px solid #21262d', fontSize: '12px',
  }

  return (
    <div style={{ background: '#0d1117', minHeight: '100vh', color: '#e6edf3', fontFamily: 'system-ui', fontSize: '13px' }}>
      <Topbar
        title="Strategies"
        actions={
          <button
            onClick={() => setShowUpload(true)}
            style={{
              padding: '6px 14px', background: '#388bfd', border: 'none',
              borderRadius: '6px', color: '#fff', fontSize: '13px', cursor: 'pointer', fontWeight: 500,
            }}
          >
            + Upload Strategy
          </button>
        }
      />

      <div style={{ padding: '24px' }}>
        {isLoading && (
          <div style={{ color: '#6e7681', textAlign: 'center', padding: '48px' }}>Loading...</div>
        )}

        {!isLoading && (!strategies || strategies.length === 0) && (
          <div style={{
            textAlign: 'center', padding: '64px 24px',
            background: '#161b22', border: '1px solid #21262d', borderRadius: '8px',
          }}>
            <div style={{ color: '#6e7681', marginBottom: '16px' }}>No strategies yet. Upload your first strategy.</div>
            <button
              onClick={() => setShowUpload(true)}
              style={{
                padding: '8px 18px', background: '#388bfd', border: 'none',
                borderRadius: '6px', color: '#fff', cursor: 'pointer', fontWeight: 500,
              }}
            >
              + Upload Strategy
            </button>
          </div>
        )}

        {strategies && strategies.length > 0 && (
          <div style={{ background: '#161b22', border: '1px solid #21262d', borderRadius: '6px', overflow: 'hidden' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr>
                  <th style={headerCell}>Name</th>
                  <th style={headerCell}>Version</th>
                  <th style={headerCell}>Runs</th>
                  <th style={headerCell}>Best Sharpe</th>
                  <th style={headerCell}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {strategies.map(s => (
                  <tr
                    key={s.id}
                    style={{ cursor: 'pointer' }}
                    onClick={() => navigate(`/strategies/${s.id}`)}
                  >
                    <td style={{ ...cell, color: '#e6edf3', fontWeight: 500 }}>{s.name}</td>
                    <td style={cell}>
                      <span style={{
                        background: '#21262d', padding: '2px 6px', borderRadius: '4px',
                        fontSize: '11px', fontFamily: 'monospace', color: '#8b949e',
                      }}>v{s.latestVersion}</span>
                    </td>
                    <td style={{ ...cell, fontFamily: 'monospace' }}>{s.runCount}</td>
                    <td style={{ ...cell, fontFamily: 'monospace' }}>
                      {s.bestSharpe != null ? s.bestSharpe.toFixed(2) : '-'}
                    </td>
                    <td style={cell} onClick={e => e.stopPropagation()}>
                      <div style={{ display: 'flex', gap: '8px' }}>
                        <button
                          onClick={() => navigate(`/strategies/${s.id}`)}
                          style={{ padding: '4px 10px', background: '#21262d', border: '1px solid #30363d', borderRadius: '5px', color: '#c9d1d9', cursor: 'pointer', fontSize: '12px' }}
                        >
                          View
                        </button>
                        <button
                          onClick={() => handleDelete(s.id, s.name)}
                          style={{ padding: '4px 10px', background: 'transparent', border: '1px solid #3d1f1f', borderRadius: '5px', color: '#f85149', cursor: 'pointer', fontSize: '12px' }}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {showUpload && <UploadModal onClose={() => setShowUpload(false)} />}
    </div>
  )
}
