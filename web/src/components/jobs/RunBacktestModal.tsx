import { useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { apiClient } from '../../lib/api'

interface RunBacktestModalProps {
  strategyVersionId: string
  onClose: () => void
}

type Step = 'params' | 'csv' | 'queued'

export default function RunBacktestModal({ strategyVersionId, onClose }: RunBacktestModalProps) {
  const navigate = useNavigate()
  const [step, setStep] = useState<Step>('params')
  const [symbols, setSymbols] = useState('')
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')
  const [resolution, setResolution] = useState('1d')
  const [dataSource, setDataSource] = useState<'alpaca' | 'csv'>('alpaca')
  const [csvFile, setCsvFile] = useState<File | null>(null)
  const [jobId, setJobId] = useState<string | null>(null)
  const [error, setError] = useState('')

  const submitMutation = useMutation({
    mutationFn: async () => {
      const symbolList = symbols.split(',').map(s => s.trim()).filter(Boolean)
      if (dataSource === 'csv' && csvFile) {
        const form = new FormData()
        form.append('strategyVersionId', strategyVersionId)
        form.append('type', 'backtest')
        form.append('dataSource', 'csv')
        form.append('symbols', JSON.stringify(symbolList))
        form.append('startDate', startDate)
        form.append('endDate', endDate)
        form.append('resolution', resolution)
        form.append('csvFile', csvFile)
        const { data } = await apiClient.post('/api/jobs', form, { headers: { 'Content-Type': 'multipart/form-data' } })
        return data
      }
      const { data } = await apiClient.post('/api/jobs', {
        strategyVersionId, type: 'backtest', dataSource: 'alpaca',
        symbols: symbolList, startDate, endDate, resolution,
      })
      return data
    },
    onSuccess: (data) => { setJobId(data.jobId); setStep('queued') },
    onError: (err: any) => setError(err.response?.data?.error ?? 'Submission failed'),
  })

  const handleParamsSubmit = (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!symbols.trim()) { setError('Symbols are required'); return }
    if (!startDate || !endDate) { setError('Start and end date are required'); return }
    if (new Date(startDate) >= new Date(endDate)) { setError('Start date must be before end date'); return }
    if (dataSource === 'csv') { setStep('csv'); return }
    submitMutation.mutate()
  }

  const overlay: React.CSSProperties = {
    position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)',
    display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000,
  }
  const card: React.CSSProperties = {
    background: '#161b22', border: '1px solid #30363d', borderRadius: '8px',
    padding: '24px', width: '460px', fontFamily: 'system-ui', fontSize: '13px', color: '#e6edf3',
  }
  const inputStyle: React.CSSProperties = {
    width: '100%', padding: '7px 10px', boxSizing: 'border-box',
    background: '#0d1117', border: '1px solid #30363d',
    borderRadius: '6px', color: '#e6edf3', fontSize: '13px', outline: 'none',
  }
  const label = (text: string) => <label style={{ display: 'block', marginBottom: '5px', color: '#c9d1d9' }}>{text}</label>
  const fieldWrap: React.CSSProperties = { marginBottom: '14px' }

  return (
    <div style={overlay} onClick={onClose}>
      <div style={card} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
          <span style={{ fontWeight: 600, fontSize: '15px' }}>Run Backtest</span>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: '#6e7681', cursor: 'pointer', fontSize: '18px' }}>×</button>
        </div>

        {step === 'params' && (
          <form onSubmit={handleParamsSubmit}>
            <div style={fieldWrap}>
              {label('Symbols')}
              <input data-testid="symbols-input" style={inputStyle} value={symbols} onChange={e => setSymbols(e.target.value)} placeholder="SPY, QQQ" />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', marginBottom: '14px' }}>
              <div>
                {label('Start Date')}
                <input data-testid="start-date-input" type="date" style={inputStyle} value={startDate} onChange={e => setStartDate(e.target.value)} />
              </div>
              <div>
                {label('End Date')}
                <input data-testid="end-date-input" type="date" style={inputStyle} value={endDate} onChange={e => setEndDate(e.target.value)} />
              </div>
            </div>
            <div style={fieldWrap}>
              {label('Resolution')}
              <select data-testid="resolution-select" style={inputStyle} value={resolution} onChange={e => setResolution(e.target.value)}>
                <option value="1d">Daily</option>
                <option value="1h">Hourly</option>
                <option value="1m">Minute</option>
              </select>
            </div>
            <div style={{ marginBottom: '16px' }}>
              {label('Data Source')}
              <div style={{ display: 'flex', gap: '10px' }}>
                {(['alpaca', 'csv'] as const).map(src => (
                  <div
                    key={src}
                    onClick={() => setDataSource(src)}
                    style={{
                      flex: 1, padding: '10px', borderRadius: '6px', cursor: 'pointer',
                      border: `1px solid ${dataSource === src ? '#388bfd' : '#30363d'}`,
                      background: dataSource === src ? 'rgba(56,139,253,0.05)' : 'transparent',
                    }}
                  >
                    <div style={{ fontWeight: 500, marginBottom: '2px' }}>{src === 'alpaca' ? 'Alpaca' : 'CSV Upload'}</div>
                    <div style={{ color: '#6e7681', fontSize: '11px' }}>
                      {src === 'alpaca' ? 'Use Alpaca historical data' : 'Upload OHLCV CSV file'}
                    </div>
                  </div>
                ))}
              </div>
            </div>
            {error && <div style={{ color: '#f85149', marginBottom: '12px', fontSize: '12px' }}>{error}</div>}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button type="button" onClick={onClose} style={{ padding: '7px 14px', background: '#21262d', border: 'none', borderRadius: '6px', color: '#e6edf3', cursor: 'pointer' }}>Cancel</button>
              <button data-testid="submit-backtest" type="submit" disabled={submitMutation.isPending} style={{ padding: '7px 14px', background: '#388bfd', border: 'none', borderRadius: '6px', color: '#fff', cursor: 'pointer', fontWeight: 500 }}>
                {dataSource === 'csv' ? 'Next →' : submitMutation.isPending ? 'Submitting...' : 'Run Backtest'}
              </button>
            </div>
          </form>
        )}

        {step === 'csv' && (
          <div>
            <div style={{ marginBottom: '16px', color: '#8b949e' }}>Upload OHLCV CSV (max 50MB)</div>
            <input type="file" accept=".csv" onChange={e => setCsvFile(e.target.files?.[0] ?? null)} style={{ color: '#e6edf3', marginBottom: '16px' }} />
            {error && <div style={{ color: '#f85149', marginBottom: '12px', fontSize: '12px' }}>{error}</div>}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button onClick={() => setStep('params')} style={{ padding: '7px 14px', background: '#21262d', border: 'none', borderRadius: '6px', color: '#e6edf3', cursor: 'pointer' }}>← Back</button>
              <button onClick={() => submitMutation.mutate()} disabled={!csvFile || submitMutation.isPending} style={{ padding: '7px 14px', background: '#388bfd', border: 'none', borderRadius: '6px', color: '#fff', cursor: 'pointer', fontWeight: 500 }}>
                Run Backtest
              </button>
            </div>
          </div>
        )}

        {step === 'queued' && (
          <div data-testid="job-queued-confirmation" style={{ textAlign: 'center', padding: '16px 0' }}>
            <div style={{ color: '#3fb950', marginBottom: '8px', fontWeight: 600 }}>Job queued!</div>
            <div style={{ color: '#6e7681', marginBottom: '16px', fontSize: '12px' }}>Job ID: {jobId}</div>
            <div style={{ display: 'flex', justifyContent: 'center', gap: '8px' }}>
              <button onClick={onClose} style={{ padding: '7px 14px', background: '#21262d', border: 'none', borderRadius: '6px', color: '#e6edf3', cursor: 'pointer' }}>Close</button>
              {jobId && <button onClick={() => { onClose(); navigate(`/results/${jobId}`) }} style={{ padding: '7px 14px', background: '#388bfd', border: 'none', borderRadius: '6px', color: '#fff', cursor: 'pointer', fontWeight: 500 }}>View job status →</button>}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
