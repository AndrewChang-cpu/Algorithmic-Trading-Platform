import { useState } from 'react'

interface MetricsTabsProps {
  metrics: Record<string, unknown> | null
}

type TabName = 'risk' | 'trades' | 'portfolio'

const RISK_FIELDS = [
  'alpha', 'beta', 'sharpe_ratio', 'sortino_ratio',
  'value_at_risk_99', 'value_at_risk_95', 'information_ratio',
  'tracking_error', 'treynor_ratio', 'volatility_annual',
]
const TRADE_FIELDS = [
  'total_trades', 'winning_trades', 'losing_trades', 'win_rate_pct',
  'avg_win_pct', 'avg_loss_pct', 'profit_loss_ratio', 'expectancy',
  'total_fees', 'largest_win', 'largest_loss',
]
const PORTFOLIO_FIELDS = [
  'start_equity', 'end_equity', 'compounding_annual_return', 'total_return_pct',
  'max_drawdown_pct', 'volatility_annual', 'portfolio_turnover', 'drawdown_recovery_days',
]

const TABS: { name: TabName; label: string; fields: string[] }[] = [
  { name: 'risk',      label: 'Risk Metrics',      fields: RISK_FIELDS },
  { name: 'trades',   label: 'Trade Stats',        fields: TRADE_FIELDS },
  { name: 'portfolio', label: 'Portfolio Details', fields: PORTFOLIO_FIELDS },
]

function formatValue(key: string, v: unknown): string {
  if (v == null) return '--'
  const n = typeof v === 'number' ? v : parseFloat(String(v))
  if (isNaN(n)) return String(v)
  if (key.endsWith('_pct')) return `${(n * 100).toFixed(2)}%`
  if (
    key.includes('equity') ||
    key.includes('fees') ||
    key === 'largest_win' ||
    key === 'largest_loss'
  ) {
    return `$${n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
  }
  return n.toFixed(4)
}

function MetricRow({ label, value }: { label: string; value: string }) {
  return (
    <tr>
      <td style={{
        padding: '8px 16px',
        color: '#8b949e',
        borderBottom: '1px solid #21262d',
        fontSize: '12px',
      }}>
        {label.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())}
      </td>
      <td style={{
        padding: '8px 16px',
        color: '#e6edf3',
        borderBottom: '1px solid #21262d',
        fontFamily: 'monospace',
        fontSize: '12px',
        textAlign: 'right',
      }}>
        {value}
      </td>
    </tr>
  )
}

export default function MetricsTabs({ metrics }: MetricsTabsProps) {
  const [tab, setTab] = useState<TabName>('risk')

  const tabStyle = (active: boolean): React.CSSProperties => ({
    padding: '7px 16px',
    background: 'none',
    border: 'none',
    borderBottom: active ? '2px solid #388bfd' : '2px solid transparent',
    color: active ? '#e6edf3' : '#6e7681',
    cursor: 'pointer',
    fontSize: '12px',
    fontFamily: 'system-ui',
  })

  const activeFields = TABS.find(t => t.name === tab)?.fields ?? []

  return (
    <div style={{ background: '#161b22', border: '1px solid #21262d', borderRadius: '6px', overflow: 'hidden' }}>
      <div style={{ display: 'flex', borderBottom: '1px solid #21262d' }}>
        {TABS.map(t => (
          <button key={t.name} style={tabStyle(tab === t.name)} onClick={() => setTab(t.name)}>
            {t.label}
          </button>
        ))}
      </div>
      {!metrics ? (
        <div style={{ padding: '24px', color: '#6e7681', textAlign: 'center', fontSize: '12px' }}>
          No metrics available.
        </div>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <tbody>
            {activeFields.map(field => (
              <MetricRow
                key={field}
                label={field}
                value={formatValue(field, metrics[field])}
              />
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
