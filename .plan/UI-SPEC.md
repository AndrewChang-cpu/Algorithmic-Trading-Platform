# UI Spec: Algorithmic Trading Platform

## Design System

**Aesthetic:** GitHub-inspired dense data terminal. Dark, information-dense, functional. Monospace for numbers and code. Minimal chrome. Data is the hero.

**Color tokens:**
```css
--bg-primary:     #0d1117;   /* page background */
--bg-secondary:   #161b22;   /* panels, cards, sidebar */
--bg-tertiary:    #21262d;   /* hover states, borders */
--border:         #21262d;
--border-muted:   #30363d;
--text-primary:   #e6edf3;
--text-secondary: #c9d1d9;
--text-muted:     #6e7681;
--accent-blue:    #388bfd;
--accent-blue-dk: #1f6feb;
--green:          #3fb950;
--red:            #f85149;
--yellow:         #d29922;
--font-mono:      'SF Mono', 'Consolas', monospace;
```

**Typography:** System sans-serif (`-apple-system, BlinkMacSystemFont, 'Inter', system-ui`) at 13px base. Monospace for numbers, code, IDs, timestamps.

**Layout:** Fixed 220px left sidebar + scrollable main area. Topbar height 46px.

## UI States

| # | Page/State | Route | Mockup |
|---|------------|-------|--------|
| 1 | Login | `/login` | [mockup-login.html](mockup-login.html) |
| 2 | Register | `/register` | [mockup-login.html](mockup-login.html) (tab) |
| 3 | Overview / Dashboard | `/overview` | [../mockups/overview.html](../mockups/overview.html) |
| 4 | Strategies list | `/strategies` | [../mockups/strategies.html](../mockups/strategies.html) |
| 5 | Upload strategy modal (step 1: file) | modal on `/strategies` | [../mockups/upload-strategy.html](../mockups/upload-strategy.html) |
| 6 | Upload strategy modal (step 2: name + confirm) | modal | [../mockups/upload-strategy.html](../mockups/upload-strategy.html) |
| 7 | Upload strategy modal (step 3: scanning) | modal | [../mockups/upload-strategy.html](../mockups/upload-strategy.html) |
| 8 | Upload strategy modal (error: AST violation) | modal | [../mockups/upload-strategy.html](../mockups/upload-strategy.html) |
| 9 | Strategy detail — code view | `/strategies/:id` | [mockup-strategy-detail.html](mockup-strategy-detail.html) |
| 10 | Strategy detail — runs list | `/strategies/:id?tab=runs` | [mockup-strategy-detail.html](mockup-strategy-detail.html) |
| 11 | Run backtest modal — data source selection | modal on strategy detail | [mockup-run-backtest-modal.html](mockup-run-backtest-modal.html) |
| 12 | Run backtest modal — CSV upload | modal | [mockup-run-backtest-modal.html](mockup-run-backtest-modal.html) |
| 13 | Backtests list | `/backtests` | [../mockups/overview.html](../mockups/overview.html) (jobs table section) |
| 14 | Backtest results — completed | `/results/:jobId` | [../mockups/results.html](../mockups/results.html) |
| 15 | Backtest results — running (in progress) | `/results/:jobId` | (running state: equity chart shows partial data, spinner) |
| 16 | Backtest results — failed | `/results/:jobId` | (error state: red banner with error message) |
| 17 | Live trading monitor — active jobs | `/live` | [../mockups/live.html](../mockups/live.html) |
| 18 | Live trading monitor — empty (no live jobs) | `/live` | (empty state with "Go Live" prompt) |
| 19 | Go Live modal | modal on strategy detail | (see spec below) |

## Page Specifications

### Login / Register (`/login`, `/register`)
**States:** login form, register form (tab-switched), loading (submitting), error (invalid credentials)
**Components:**
- Centered card on full dark background (no sidebar)
- Logo mark + "Algorithmic Trading Platform" header
- Email + password inputs
- Submit button (full width, primary)
- Toggle between Login/Register (tab or link)
- Error message inline below the form
- Register: confirm password field is NOT required (keep it simple)

**Empty/loading states:**
- Button shows spinner on submit
- Inputs disabled during submission
- Redirect to `/overview` on success

### Overview / Dashboard (`/overview`)
Existing mockup (`../mockups/overview.html`) is the reference. Additions needed:
- System health bar must reflect: Kafka status, Redis status, PostgreSQL status (all green/warn/down)
- "Active Jobs" count in the summary card must link to `/backtests?status=running`
- Recent jobs table rows link to `/results/:jobId` (backtest) or `/live` (live)

### Strategies List (`/strategies`)
Existing mockup (`../mockups/strategies.html`) is the reference. Additions needed:
- Each strategy row shows: name, latest version badge (e.g. v3), last run P&L, validation status, run/detail/delete actions
- Empty state: centered card "No strategies yet. Upload your first strategy." with upload button
- Delete action: shows confirmation dialog before deleting

### Upload Strategy Modal (multi-step)
Existing mockup (`../mockups/upload-strategy.html`) is the reference. Steps:
1. **File drop**: Drag-drop zone for `.py` file, or click to browse. Shows file name + size on select.
2. **Name**: Text input for strategy name. Optional description. Submit triggers upload.
3. **Scanning**: Progress state — "Scanning strategy for security violations..." with spinner.
4. **Error**: If AST scan fails, red alert with specific violation: `"Violation: import os detected on line 3"`. Back button.
5. **Success**: Green checkmark, "Strategy uploaded (v1)". Close → navigates to strategy detail.

**Versioning:** Re-uploading via the strategy detail page skips name step (uses existing name). Same scan flow. On success: "Version 2 uploaded."

### Strategy Detail (`/strategies/:id`)

**Tab 1: Code**
- Full-width syntax-highlighted Python code viewer (read-only)
- Version selector dropdown top-right: "v3 (latest)" | "v2" | "v1"
- Each version shows its upload date
- Header: strategy name, version badge, "Upload New Version" button, "Delete Strategy" button (danger)

**Tab 2: Runs**
- Table: Run #, Version, Type (Backtest/Live), Status badge, Date, Net P&L, Sharpe
- Clicking a row → navigates to results page or live monitor
- "Run Backtest" button (primary) + "Go Live" button (green) in top bar
- Empty state: "No runs yet. Run your first backtest."

**Aggregate stats bar** (shown on both tabs):
- Total runs | Best Sharpe | Best Return | Avg Return

See: [mockup-strategy-detail.html](mockup-strategy-detail.html)

### Run Backtest Modal

**Step 1: Parameters**
- **Symbols**: text input, comma-separated tickers (e.g. `SPY, QQQ`). Required.
- **Start date / End date**: date pickers. Required.
- **Resolution**: dropdown — `Daily` (default) | `Hourly` | `Minute`.
- **Data source**: two radio-card options below the above fields:
  1. **Alpaca** (default selected): "Use Alpaca historical data."
  2. **CSV Upload**: "Upload an OHLCV CSV file (timestamp, open, high, low, close, volume)."
- Primary button: "Run Backtest" (if Alpaca) or "Next →" (if CSV)
- Validation: all fields required; start date must be before end date.

**Step 2 (CSV only): Upload CSV**
- Same dropzone UI as strategy upload
- File constraints: `.csv` only, max 50MB
- "Run Backtest" button

**Step 3: Confirmation / queuing**
- "Job queued. Job ID: abc123"
- Link: "View job status →" navigates to results page

See: [mockup-run-backtest-modal.html](mockup-run-backtest-modal.html)

### Go Live Modal

Opened from the "Go Live" button on the Strategy Detail page.

**Fields:**
- **Symbols**: text input, comma-separated tickers (e.g. `SPY, QQQ`). Required.
- **Resolution**: dropdown — `Daily` (default) | `Hourly` | `Minute`.
- **Warmup period**: number input, days of historical data to pre-fetch before live trading starts. Default: 365. Label: "Warmup period (days)".
- No data source selector — live trading data always flows through go-data → Kafka. go-data's data source is a deployment concern, not a per-job choice.

**Steps:**
1. **Parameters**: symbols, resolution, warmup period fields. Primary button: "Start Live Trading".
2. **Confirmation**: "Live job queued. Job ID: abc123." Link: "View live monitor →" navigates to `/live`.

**Validation:** all fields required; warmup days must be ≥ 1.

### Backtest Results (`/results/:jobId`)

Existing mockup (`../mockups/results.html`) is the reference.

**Running state (job in progress):**
- Equity chart shows partial data updating in real-time (WebSocket)
- Hero metrics show "--" with a pulsing indicator
- "Running..." status badge with spinner
- Log stream panel at the bottom shows live LEAN log output

**Failed state:**
- Red banner at top: "Backtest failed: [error_message]"
- No metrics or chart
- "View Logs" button expands log output

**Completed state:**
- Equity curve (zoomable, uses Lightweight Charts)
- Hero metrics: Net P&L (green/red), Sharpe Ratio, Max Drawdown, Win Rate
- Total Fees chip
- Tabs: Risk Metrics | Trade Stats | Portfolio Details
- Download buttons: "Export JSON" | "Export CSV"
- "Clone Strategy" button navigates to strategy detail with pre-selected version

### Live Trading Monitor (`/live`)

Existing mockup (`../mockups/live.html`) is the reference.

**Active jobs list (left panel):**
- Each card: strategy name, P&L, runtime, running status dot
- "+ Go Live" button at bottom of list → opens strategy selector

**Detail panel (right):**
- Stats bar: Equity, Unrealized P&L, Holdings, Fees (live-updating)
- Real-time equity chart (last N hours, scrolling)
- Positions table: Symbol | Quantity | Avg Cost | Current Price | P&L
- Log stream: last 50 log lines (auto-scroll)
- "Stop" button (danger) → confirmation dialog → sends cancel request

**Empty state:**
- Centered card: "No live strategies running. Select a strategy and click Go Live."

## Component Structure

```
src/
├── pages/
│   ├── Login.tsx              # email, password, submit, error display
│   ├── Register.tsx           # same fields + redirect
│   ├── Overview.tsx           # health bar, summary cards, recent jobs table
│   ├── Strategies.tsx         # filter bar, table, upload modal trigger
│   ├── StrategyDetail.tsx     # tabs: Code | Runs, version selector
│   ├── Backtests.tsx          # paginated job list (filter by status)
│   ├── Results.tsx            # equity chart + metrics (running/failed/completed)
│   └── Live.tsx               # split panel: job list + detail
├── components/
│   ├── layout/
│   │   ├── Sidebar.tsx        # nav items, live dot, user avatar
│   │   └── Topbar.tsx         # page title, action buttons
│   ├── results/
│   │   ├── EquityCurve.tsx    # Lightweight Charts, zoom, OHLC data
│   │   ├── HeroMetrics.tsx    # 4 metric cards + fees chip
│   │   ├── MetricsTabs.tsx    # Risk | Trade | Portfolio tab panels
│   │   └── LogStream.tsx      # auto-scrolling log panel
│   ├── strategy/
│   │   ├── UploadModal.tsx    # multi-step wizard (file → name → scan → result)
│   │   ├── CodeViewer.tsx     # syntax-highlighted Python (read-only)
│   │   └── VersionSelector.tsx
│   └── jobs/
│       ├── JobCard.tsx        # used in live panel left list
│       ├── StatusBadge.tsx    # queued/running/completed/failed
│       └── RunBacktestModal.tsx  # symbols, dates, resolution, data source selection + CSV upload
├── hooks/
│   ├── useAuth.ts             # Zustand auth store + token refresh logic
│   ├── useJobStatus.ts        # WebSocket /api/stream/jobs/:id
│   └── usePortfolio.ts        # WebSocket /api/stream/portfolio/:jobId
└── lib/
    ├── api.ts                 # Axios instance, baseURL, 401 interceptor for refresh
    └── store.ts               # Zustand: { user, accessToken, setAuth, clearAuth }
```

## State Management

- **Server state**: React Query (all API data: strategies, jobs, metrics, portfolio history)
- **Client state**: Zustand (auth: user, accessToken)
- **Real-time**: WebSocket hooks (`useJobStatus`, `usePortfolio`) — invalidate React Query cache on status changes
- **No Redux**

## Navigation / Routing (React Router v6)
```
/login          → Login (unauthenticated only)
/register       → Register (unauthenticated only)
/               → redirect to /overview
/overview       → Overview
/strategies     → Strategies list
/strategies/:id → Strategy detail
/backtests      → Backtest jobs list
/results/:jobId → Backtest results
/live           → Live trading monitor
```
Protected routes: all except `/login` and `/register`. Unauthenticated → redirect to `/login`.
