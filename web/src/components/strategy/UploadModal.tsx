import { useState, useCallback, DragEvent, ChangeEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '../../lib/api'

interface UploadModalProps {
  onClose: () => void
  strategyId?: string  // if set, uploading a new version (skip name step)
}

type Step = 'file' | 'name' | 'scanning' | 'success' | 'error'

export default function UploadModal({ onClose, strategyId }: UploadModalProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [step, setStep] = useState<Step>('file')
  const [file, setFile] = useState<File | null>(null)
  const [name, setName] = useState('')
  const [dragging, setDragging] = useState(false)
  const [resultId, setResultId] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState('')

  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!file) throw new Error('No file selected')
      setStep('scanning')
      const form = new FormData()
      form.append('file', file)
      if (!strategyId) form.append('name', name)
      const endpoint = strategyId
        ? `/api/strategies/${strategyId}/versions`
        : '/api/strategies'
      const { data } = await apiClient.post(endpoint, form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      return data
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['strategies'] })
      setResultId(data.strategyId ?? strategyId ?? null)
      setStep('success')
    },
    onError: (err: unknown) => {
      const axiosErr = err as { response?: { data?: { error?: string } } }
      setErrorMsg(axiosErr.response?.data?.error ?? 'Upload failed')
      setStep('error')
    },
  })

  const handleDrop = useCallback((e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    const f = e.dataTransfer.files[0]
    if (f?.name.endsWith('.py')) setFile(f)
  }, [])

  const handleFileInput = (e: ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    if (f?.name.endsWith('.py')) setFile(f)
  }

  const overlay: React.CSSProperties = {
    position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)',
    display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000,
  }
  const card: React.CSSProperties = {
    background: '#161b22', border: '1px solid #30363d', borderRadius: '8px',
    padding: '24px', width: '440px', fontFamily: 'system-ui', fontSize: '13px', color: '#e6edf3',
  }
  const btn = (variant: 'primary' | 'secondary' | 'danger'): React.CSSProperties => ({
    padding: '7px 16px', borderRadius: '6px', border: 'none', cursor: 'pointer', fontSize: '13px',
    fontWeight: 500,
    background: variant === 'primary' ? '#388bfd' : variant === 'danger' ? '#3d1f1f' : '#21262d',
    color: variant === 'danger' ? '#f85149' : '#e6edf3',
  })

  const title = strategyId ? 'Upload New Version' : 'Upload Strategy'

  return (
    <div style={overlay} onClick={onClose}>
      <div style={card} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
          <span style={{ fontWeight: 600, fontSize: '15px' }}>{title}</span>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: '#6e7681', cursor: 'pointer', fontSize: '18px' }}>x</button>
        </div>

        {step === 'file' && (
          <>
            <div
              onDragOver={e => { e.preventDefault(); setDragging(true) }}
              onDragLeave={() => setDragging(false)}
              onDrop={handleDrop}
              style={{
                border: `2px dashed ${dragging ? '#388bfd' : '#30363d'}`,
                borderRadius: '6px', padding: '40px', textAlign: 'center',
                background: dragging ? 'rgba(56,139,253,0.05)' : 'transparent',
                cursor: 'pointer', marginBottom: '16px',
              }}
            >
              {file ? (
                <div>
                  <div style={{ color: '#3fb950', marginBottom: '4px' }}>+ {file.name}</div>
                  <div style={{ color: '#6e7681' }}>{(file.size / 1024).toFixed(1)} KB</div>
                </div>
              ) : (
                <>
                  <div style={{ color: '#6e7681', marginBottom: '8px' }}>Drag & drop your .py strategy file</div>
                  <label style={{ color: '#388bfd', cursor: 'pointer' }}>
                    Browse
                    <input data-testid="upload-file-input" type="file" accept=".py" onChange={handleFileInput} style={{ display: 'none' }} />
                  </label>
                </>
              )}
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button style={btn('secondary')} onClick={onClose}>Cancel</button>
              <button
                style={{ ...btn('primary'), opacity: file ? 1 : 0.5 }}
                disabled={!file}
                onClick={() => {
                  if (strategyId) uploadMutation.mutate()
                  else setStep('name')
                }}
              >
                {strategyId ? 'Upload' : 'Next'}
              </button>
            </div>
          </>
        )}

        {step === 'name' && (
          <>
            <div style={{ marginBottom: '16px' }}>
              <label style={{ display: 'block', marginBottom: '6px', color: '#c9d1d9' }}>Strategy name</label>
              <input
                data-testid="strategy-name-input"
                value={name}
                onChange={e => setName(e.target.value)}
                autoFocus
                placeholder="My Momentum Strategy"
                style={{
                  width: '100%', padding: '8px 12px', boxSizing: 'border-box',
                  background: '#0d1117', border: '1px solid #30363d',
                  borderRadius: '6px', color: '#e6edf3', fontSize: '13px', outline: 'none',
                }}
              />
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button style={btn('secondary')} onClick={() => setStep('file')}>Back</button>
              <button
                data-testid="upload-submit"
                style={{ ...btn('primary'), opacity: name.trim() ? 1 : 0.5 }}
                disabled={!name.trim()}
                onClick={() => uploadMutation.mutate()}
              >
                Upload
              </button>
            </div>
          </>
        )}

        {step === 'scanning' && (
          <div style={{ textAlign: 'center', padding: '24px 0' }}>
            <div style={{ marginBottom: '12px', color: '#8b949e' }}>Scanning strategy for security violations...</div>
            <div style={{
              width: '24px', height: '24px', margin: '0 auto',
              border: '3px solid #21262d', borderTopColor: '#388bfd',
              borderRadius: '50%', animation: 'spin 0.7s linear infinite',
            }} />
            <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
          </div>
        )}

        {step === 'success' && (
          <div data-testid="upload-success" style={{ textAlign: 'center', padding: '16px 0' }}>
            <div style={{ fontSize: '32px', marginBottom: '12px', color: '#3fb950' }}>+</div>
            <div style={{ color: '#3fb950', marginBottom: '8px', fontWeight: 600 }}>
              {strategyId ? 'New version uploaded' : 'Strategy uploaded (v1)'}
            </div>
            <div style={{ display: 'flex', justifyContent: 'center', gap: '8px', marginTop: '16px' }}>
              <button style={btn('secondary')} onClick={onClose}>Close</button>
              {resultId && !strategyId && (
                <button style={btn('primary')} onClick={() => { onClose(); navigate(`/strategies/${resultId}`) }}>
                  View Strategy
                </button>
              )}
            </div>
          </div>
        )}

        {step === 'error' && (
          <div>
            <div
              data-testid="upload-error"
              style={{
                padding: '12px', background: '#3d1f1f', border: '1px solid #f85149',
                borderRadius: '6px', color: '#f85149', marginBottom: '16px',
              }}
            >
              {errorMsg}
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button style={btn('secondary')} onClick={() => { setStep('file'); setFile(null); setErrorMsg('') }}>Back</button>
              <button style={btn('secondary')} onClick={onClose}>Cancel</button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
