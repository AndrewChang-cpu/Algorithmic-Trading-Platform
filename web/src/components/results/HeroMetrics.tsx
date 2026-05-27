interface Metrics {
  total_return_pct?: number | null
  sharpe_ratio?: number | null
  max_drawdown_pct?: number | null
  win_rate_pct?: number | null
  total_fees?: number | null
  start_equity?: number | null
  end_equity?: number | null
}

interface HeroMetricsProps {
  metrics: Metrics | null
  isRunning?: boolean
}

function MetricCard({ label, value, valueColor }: { label: string; value: string; valueColor?: string }) {
  return (
    <div style={{
      background: '#161b22',
      border: '1px solid #21262d',
      borderRadius: '6px',
      padding: '14px 18px',
      minWidth: '140px',
      flex: 1,
    }}>
      <div style={{ color: '#6e7681', fontSize: '11px', marginBottom: '6px' }}>{label}</div>
      <div style={{ fontFamily: 'monospace', fontSize: '20px', fontWeight: 600, color: valueColor ?? '#e6edf3' }}>
        {value}
      </div>
    </div>
  )
}

function pct(v: number | null | undefined, decimals = 2): string {
  if (v == null) return '--'
  return `${(v * 100).toFixed(decimals)}%`
}

export default function HeroMetrics({ metrics, isRunning }: HeroMetricsProps) {
  if (isRunning || !metrics) {
    return (
      <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
        {['Net P&L', 'Sharpe Ratio', 'Max Drawdown', 'Win Rate'].map(label => (
          <MetricCard key={label} label={label} value="--" />
        ))}
      </div>
    )
  }

  const netPnL =
    metrics.end_equity != null && metrics.start_equity != null
      ? metrics.end_equity - metrics.start_equity
      : null
  const netPnLPct = metrics.total_return_pct

  return (
    <div>
      <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap', marginBottom: '8px' }}>
        <MetricCard
          label="Net P&L"
          value={
            netPnL != null
              ? `$${netPnL.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })} (${pct(netPnLPct)})`
              : '--'
          }
          valueColor={netPnL != null ? (netPnL >= 0 ? '#3fb950' : '#f85149') : undefined}
        />
        <MetricCard
          label="Sharpe Ratio"
          value={metrics.sharpe_ratio?.toFixed(3) ?? '--'}
        />
        <MetricCard
          label="Max Drawdown"
          value={pct(metrics.max_drawdown_pct)}
          valueColor="#f85149"
        />
        <MetricCard
          label="Win Rate"
          value={pct(metrics.win_rate_pct)}
        />
      </div>
      {metrics.total_fees != null && (
        <span style={{
          display: 'inline-block',
          padding: '2px 8px',
          background: '#21262d',
          borderRadius: '4px',
          fontSize: '11px',
          color: '#6e7681',
          fontFamily: 'monospace',
        }}>
          Total Fees: ${metrics.total_fees.toFixed(2)}
        </span>
      )}
    </div>
  )
}
