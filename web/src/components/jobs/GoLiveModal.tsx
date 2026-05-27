import { useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { apiClient } from '../../lib/api'

interface GoLiveModalProps {
  strategyVersionId: string
  onClose: () => void
}

export default function GoLiveModal({ strategyVersionId, onClose }: GoLiveModalProps) {
  const navigate = useNavigate()
  const [symbols, setSymbols] = useState('')
  const [resolution, setResolution] = useState('1d')
  const [warmupDays, setWarmupDays] = useState(365)
  const [jobId, setJobId] = useState<string | null>(null)
  const [error, setError] = useState('')

  const submitMutation = useMutation({
    mutationFn: async () => {
      const symbolList = symbols.split(',').map(s => s.trim()).filter(Boolean)
      if (!symbolList.length) throw new Error('Symbols are required')
      const { data } = await apiClient.post('/api/jobs', {
        strategyVersionId, type: 'live',
        symbols: symbolList, resolution, warmupDays,
      })
      return data
    },
    onSuccess: (data) => setJobId(data.jobId),
    onError: (err: any) => setError(err.response?.data?.error ?? 'Failed to start live trading'),
  })

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!symbols.trim()) { setError('Symbols are required'); return }
    submitMutation.mutate()
  }

  const overlay: React.CSSProperties = {
    position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)',
    display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000,
  }
  const card: React.CSSProperties = {
    background: '#161b22', border: '1px solid #30363d', borderRadius: '8px',
    padding: '24px', width: '420px', fontFamily: 'system-ui', fontSize: '13px', color: '#e6edf3',
  }
  const inputStyle: React.CSSProperties = {
    width: '100%', padding: '7px 10px', boxSizing: 'border-box',
    background: '#0d1117', border: '1px solid #30363d',
    borderRadius: '6px', color: '#e6edf3', fontSize: '13px', outline: 'none',
  }

  return (
    <div style={overlay} onClick={onClose}>
      <div style={card} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
          <span style={{ fontWeight: 600, fontSize: '15px' }}>Start Live Trading</span>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: '#6e7681', cursor: 'pointer', fontSize: '18px' }}>×</button>
        </div>

        {!jobId ? (
          <form onSubmit={handleSubmit}>
            <div style={{ marginBottom: '14px' }}>
              <label style={{ display: 'block', marginBottom: '5px', color: '#c9d1d9' }}>Symbols</label>
              <input style={inputStyle} value={symbols} onChange={e => setSymbols(e.target.value)} placeholder="SPY, QQQ" />
            </div>
            <div style={{ marginBottom: '14px' }}>
              <label style={{ display: 'block', marginBottom: '5px', color: '#c9d1d9' }}>Resolution</label>
              <select style={inputStyle} value={resolution} onChange={e => setResolution(e.target.value)}>
                <option value="1d">Daily</option>
                <option value="1h">Hourly</option>
                <option value="1m">Minute</option>
              </select>
            </div>
            <div style={{ marginBottom: '16px' }}>
              <label style={{ display: 'block', marginBottom: '5px', color: '#c9d1d9' }}>Warmup period (days)</label>
              <input type="number" style={inputStyle} value={warmupDays} min={1}
                onChange={e => setWarmupDays(parseInt(e.target.value) || 365)} />
              <div style={{ color: '#6e7681', fontSize: '11px', marginTop: '4px' }}>
                Days of historical data to pre-fetch before live trading starts
              </div>
            </div>
            {error && <div style={{ color: '#f85149', marginBottom: '12px', fontSize: '12px' }}>{error}</div>}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button type="button" onClick={onClose} style={{ padding: '7px 14px', background: '#21262d', border: 'none', borderRadius: '6px', color: '#e6edf3', cursor: 'pointer' }}>Cancel</button>
              <button type="submit" disabled={submitMutation.isPending} style={{ padding: '7px 14px', background: '#3fb950', border: 'none', borderRadius: '6px', color: '#fff', cursor: 'pointer', fontWeight: 500 }}>
                {submitMutation.isPending ? 'Starting...' : 'Start Live Trading'}
              </button>
            </div>
          </form>
        ) : (
          <div style={{ textAlign: 'center', padding: '16px 0' }}>
            <div style={{ color: '#3fb950', marginBottom: '8px', fontWeight: 600 }}>Live job queued!</div>
            <div style={{ color: '#6e7681', marginBottom: '16px', fontSize: '12px' }}>Job ID: {jobId}</div>
            <div style={{ display: 'flex', justifyContent: 'center', gap: '8px' }}>
              <button onClick={onClose} style={{ padding: '7px 14px', background: '#21262d', border: 'none', borderRadius: '6px', color: '#e6edf3', cursor: 'pointer' }}>Close</button>
              <button onClick={() => { onClose(); navigate('/live') }} style={{ padding: '7px 14px', background: '#3fb950', border: 'none', borderRadius: '6px', color: '#fff', cursor: 'pointer', fontWeight: 500 }}>View live monitor →</button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
