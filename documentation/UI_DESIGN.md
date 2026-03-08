# UI Design Specification

## Results Dashboard

### Page Route
`/results/:jobId`

---

## Layout

### Desktop (>1024px)
```
┌────────────────────────────────────────────────────────┐
│ NAVBAR                                                 │
│ [Logo] [Strategies] [Backtests] [Live] [Profile ▼]   │
└────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────┐
│ JOB HEADER                                             │
│ Strategy: MA Crossover   Status: ● Completed           │
│ Runtime: 2026-03-08 10:00 → 10:15 (15 min)           │
│ Orders: 127  |  Logs: 1,234                           │
└────────────────────────────────────────────────────────┘

┌──────────────┬──────────────┬──────────────┬──────────┐
│ Net Profit   │ Sharpe Ratio │ Max Drawdown │ Win Rate │
│   +16.92%    │     8.85     │    -2.20%    │  45.2%   │
│   (green)    │              │   (3 days)   │          │
└──────────────┴──────────────┴──────────────┴──────────┘
┌─────────────────────────────────────────────────────┐
│                 Total Fees: $3.44                   │
└─────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────┐
│ EQUITY CURVE (interactive, zoomable)                  │
│ [Chart with OHLC candlesticks]                        │
└────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────┐
│ [Risk Metrics] [Trade Stats] [Portfolio Details]      │
│                                                        │
│ (Tab content - see sections below)                    │
└────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────┐
│ RUNTIME STATISTICS (sidebar or bottom panel)          │
│ Equity: $101,691.92  |  Unrealized: +$1,656.23       │
│ Holdings: $101,305.19  |  Fees: -$3.44               │
└────────────────────────────────────────────────────────┘

[Download JSON] [Export CSV] [View Logs] [Clone Strategy]
```

### Mobile (<768px)
- Single column layout
- Collapsible sections (accordions)
- Horizontal scrolling for equity curve
- Sticky header with key metrics

---

## Components

### 1. Job Header
**Data**: `state`, `algorithmConfiguration`

```jsx
<JobHeader>
  <StrategyName>{strategy.name}</StrategyName>
  <StatusBadge status={state.Status}>
    {status === 'Completed' && '● Completed'}
    {status === 'Running' && '⟳ Running'}
    {status === 'Failed' && '✕ Failed'}
  </StatusBadge>
  <Runtime>
    {state.StartTime} → {state.EndTime} ({duration})
  </Runtime>
  <Counts>
    Orders: {state.OrderCount} | Logs: {state.LogCount}
  </Counts>
</JobHeader>
```

### 2. Hero Metrics (5 Cards)
**Data**: `statistics`, `totalPerformance.portfolioStatistics`

```jsx
<HeroMetrics>
  <MetricCard
    label="Net Profit"
    value={statistics['Net Profit']}
    format="percentage"
    color={value > 0 ? 'green' : 'red'}
  />
  <MetricCard
    label="Sharpe Ratio"
    value={portfolioStatistics.sharpeRatio}
    format="decimal"
    tooltip="Risk-adjusted return (higher is better)"
  />
  <MetricCard
    label="Max Drawdown"
    value={statistics.Drawdown}
    format="percentage"
    subtext={`Recovery: ${portfolioStatistics.drawdownRecovery} days`}
  />
  <MetricCard
    label="Win Rate"
    value={statistics['Win Rate']}
    format="percentage"
  />
  <MetricCard
    label="Total Fees"
    value={statistics['Total Fees']}
    format="currency"
    color="red"
  />
</HeroMetrics>
```

### 3. Equity Curve Chart
**Library**: Lightweight Charts (better performance for financial data)

**Data**: `charts['Strategy Equity'].series.Equity.values`

```jsx
import { createChart } from 'lightweight-charts';

function EquityCurve({ data }) {
  // Transform LEAN data: [[timestamp, O, H, L, C], ...]
  const candlestickData = data.map(([time, open, high, low, close]) => ({
    time: time,  // Unix timestamp
    open,
    high,
    low,
    close
  }));

  // Create chart
  const chart = createChart(containerRef.current, {
    width: 800,
    height: 400,
    layout: {
      backgroundColor: '#ffffff',
      textColor: '#333',
    },
    grid: {
      vertLines: { color: '#e1e1e1' },
      horzLines: { color: '#e1e1e1' },
    },
  });

  const candlestickSeries = chart.addCandlestickSeries();
  candlestickSeries.setData(candlestickData);

  // Interactions
  chart.timeScale().fitContent();
  // Zoom with mouse wheel, pan with drag
}
```

**Features**:
- Zoomable (mouse wheel)
- Pannable (drag)
- Crosshair on hover
- Tooltip showing exact OHLC values
- Responsive (auto-resize)

---

## Tab Content

### Tab 1: Risk Metrics
**Data**: `statistics`, `totalPerformance.portfolioStatistics`

**Layout**: 3-column grid

```jsx
<RiskMetricsTab>
  <Section title="Risk-Adjusted Returns">
    <MetricCard label="Sortino Ratio" value={statistics['Sortino Ratio']} tooltip="..." />
    <MetricCard label="Prob. Sharpe Ratio" value={statistics['Probabilistic Sharpe Ratio']} />
    <MetricCard label="Information Ratio" value={statistics['Information Ratio']} />
  </Section>

  <Section title="Market Sensitivity">
    <MetricCard label="Beta" value={statistics.Beta} />
    <MetricCard label="Alpha" value={statistics.Alpha} />
    <MetricCard label="Tracking Error" value={statistics['Tracking Error']} />
  </Section>

  <Section title="Volatility">
    <MetricCard label="Annual Std Dev" value={statistics['Annual Standard Deviation']} />
    <MetricCard label="Annual Variance" value={statistics['Annual Variance']} />
    <MetricCard label="Treynor Ratio" value={statistics['Treynor Ratio']} />
  </Section>

  <Section title="Value at Risk">
    <MetricCard label="VaR 99%" value={portfolioStatistics.valueAtRisk99} />
    <MetricCard label="VaR 95%" value={portfolioStatistics.valueAtRisk95} />
  </Section>
</RiskMetricsTab>
```

**Total**: 11 risk metrics

### Tab 2: Trade Statistics
**Data**: `totalPerformance.tradeStatistics`

**Conditional**: Only show if `totalNumberOfTrades > 0`

```jsx
<TradeStatsTab>
  {tradeStatistics.totalNumberOfTrades === 0 ? (
    <EmptyState>No closed trades yet</EmptyState>
  ) : (
    <>
      <Section title="Summary">
        <MetricCard label="Total Trades" value={tradeStatistics.totalNumberOfTrades} />
        <MetricCard label="Winning Trades" value={tradeStatistics.numberOfWinningTrades} />
        <MetricCard label="Losing Trades" value={tradeStatistics.numberOfLosingTrades} />
      </Section>

      <Section title="Profitability">
        <MetricCard label="Total P&L" value={tradeStatistics.totalProfitLoss} format="currency" />
        <MetricCard label="Average Win" value={tradeStatistics.averageProfit} format="currency" />
        <MetricCard label="Average Loss" value={tradeStatistics.averageLoss} format="currency" />
        <MetricCard label="Profit Factor" value={tradeStatistics.profitFactor} />
        <MetricCard label="Largest Win" value={tradeStatistics.largestProfit} format="currency" />
        <MetricCard label="Largest Loss" value={tradeStatistics.largestLoss} format="currency" />
      </Section>

      <Section title="Duration">
        <MetricCard label="Avg Duration" value={tradeStatistics.averageTradeDuration} format="interval" />
        <MetricCard label="Avg Win Duration" value={tradeStatistics.averageWinningTradeDuration} format="interval" />
        <MetricCard label="Avg Loss Duration" value={tradeStatistics.averageLosingTradeDuration} format="interval" />
        <MetricCard label="Median Duration" value={tradeStatistics.medianTradeDuration} format="interval" />
      </Section>

      <Section title="Excursion">
        <MetricCard label="Avg MAE" value={tradeStatistics.averageMAE} tooltip="Maximum Adverse Excursion" />
        <MetricCard label="Avg MFE" value={tradeStatistics.averageMFE} tooltip="Maximum Favorable Excursion" />
        <MetricCard label="Largest MAE" value={tradeStatistics.largestMAE} />
        <MetricCard label="Largest MFE" value={tradeStatistics.largestMFE} />
      </Section>

      <Section title="Streaks">
        <MetricCard label="Max Consec Wins" value={tradeStatistics.maxConsecutiveWinningTrades} />
        <MetricCard label="Max Consec Losses" value={tradeStatistics.maxConsecutiveLosingTrades} />
      </Section>
    </>
  )}
</TradeStatsTab>
```

**Total**: 30+ trade metrics

### Tab 3: Portfolio Details
**Data**: `statistics`, `totalPerformance.portfolioStatistics`, `algorithmConfiguration`

```jsx
<PortfolioTab>
  <Section title="Returns">
    <MetricCard label="CAGR" value={statistics['Compounding Annual Return']} />
    <MetricCard label="Total Return" value={statistics['Net Profit']} />
    <MetricCard label="Expectancy" value={portfolioStatistics.expectancy} />
  </Section>

  <Section title="Equity">
    <MetricCard label="Start Equity" value={portfolioStatistics.startEquity} format="currency" />
    <MetricCard label="End Equity" value={portfolioStatistics.endEquity} format="currency" />
  </Section>

  <Section title="Strategy Capacity">
    <MetricCard label="Estimated Capacity" value={statistics['Estimated Strategy Capacity']} format="currency" />
    <MetricCard label="Lowest Capacity Asset" value={statistics['Lowest Capacity Asset']} />
    <MetricCard label="Portfolio Turnover" value={statistics['Portfolio Turnover']} />
  </Section>

  <Section title="Configuration">
    <MetricCard label="Start Date" value={algorithmConfiguration.startDate} format="date" />
    <MetricCard label="End Date" value={algorithmConfiguration.endDate} format="date" />
    <MetricCard label="Trading Days/Year" value={algorithmConfiguration.tradingDaysPerYear} />
    <MetricCard label="Account Currency" value={algorithmConfiguration.accountCurrency} />
  </Section>
</PortfolioTab>
```

**Total**: 12 portfolio metrics

---

## Real-Time Updates

### Polling Strategy
```jsx
const { data: status } = useQuery(
  ['job-status', jobId],
  () => fetchJobStatus(jobId),
  {
    refetchInterval: (data) =>
      data?.status === 'running' || data?.status === 'queued' ? 2000 : false,
  }
);

const { data: metrics } = useQuery(
  ['job-metrics', jobId],
  () => fetchJobMetrics(jobId),
  {
    enabled: status?.status === 'completed',
  }
);
```

### Loading States
```jsx
{status === 'running' && (
  <div>
    <Spinner />
    <p>Backtest running... {progress}%</p>
    <RuntimeStats data={runtimeStatistics} /> {/* Updates every 2s */}
  </div>
)}

{status === 'completed' && (
  <ComprehensiveResults data={metrics} />
)}

{status === 'failed' && (
  <ErrorBanner error={state.RuntimeError} stackTrace={state.StackTrace} />
)}
```

---

## Number Formatting

```jsx
const formatters = {
  currency: (value) => `$${Number(value).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`,

  percentage: (value) => `${(Number(value) * 100).toFixed(2)}%`,

  decimal: (value) => Number(value).toFixed(3),

  interval: (value) => {
    // Parse "2d 4h 15m" format
    // Return human-readable duration
  },

  date: (value) => new Date(value).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  })
};
```

---

## Color Coding

```jsx
const getColor = (value, type) => {
  if (type === 'profit') {
    return value > 0 ? 'text-green-600' : 'text-red-600';
  }
  if (type === 'drawdown') {
    return 'text-red-600';
  }
  return 'text-gray-900';
};
```

---

## Component Structure

```
src/pages/Results.tsx
├── components/results/
│   ├── JobHeader.tsx
│   ├── HeroMetrics.tsx
│   │   └── MetricCard.tsx
│   ├── EquityCurve.tsx
│   ├── TabContainer.tsx
│   │   ├── RiskMetricsTab.tsx
│   │   ├── TradeStatsTab.tsx
│   │   └── PortfolioTab.tsx
│   ├── RuntimeStats.tsx
│   └── ActionsToolbar.tsx
└── hooks/
    ├── useJobStatus.ts
    ├── useJobMetrics.ts
    └── useFormatters.ts
```

---

## API Integration

### Fetch Job Metrics
```typescript
GET /api/jobs/:id/metrics

Response: {
  statistics: { ... },           // Display metrics
  runtimeStatistics: { ... },    // Live values
  totalPerformance: {
    portfolioStatistics: { ... },
    tradeStatistics: { ... }
  },
  charts: {
    "Strategy Equity": {
      series: {
        Equity: {
          values: [[timestamp, O, H, L, C], ...]
        }
      }
    }
  },
  state: {
    Status: "Completed",
    StartTime: "...",
    EndTime: "...",
    OrderCount: 127,
    LogCount: 1234
  },
  algorithmConfiguration: { ... }
}
```

---

## Responsive Design

### Breakpoints
- **Mobile** (<768px): 1 column, collapsible sections
- **Tablet** (768-1024px): 2 columns
- **Desktop** (>1024px): 3 columns

### Mobile Optimizations
- Tabs become accordion
- Chart: horizontal scroll, touch gestures
- Sticky header with key metric (Net Profit only)
- Bottom sheet for detailed metrics
