import { test, expect } from '@playwright/test'
import { setLoggedIn } from './helpers'

test('backtests page shows empty state when no jobs', async ({ page }) => {
  await page.goto('/login')
  await setLoggedIn(page)

  await page.route('**/api/jobs**', route =>
    route.fulfill({ status: 200, json: { jobs: [], total: 0 } })
  )

  await page.goto('/backtests')
  await expect(page.getByText(/no backtests yet/i)).toBeVisible()
})

test('run backtest modal shows job queued confirmation', async ({ page }) => {
  await page.goto('/login')
  await setLoggedIn(page)

  const strategy = {
    id: 'strat-1',
    name: 'Test Strategy',
    latestVersion: 1,
    runCount: 0,
    bestSharpe: null,
    createdAt: '2024-01-01',
    versions: [{ id: 'v1', versionNumber: 1, createdAt: '2024-01-01' }],
  }

  await page.route('**/api/strategies/strat-1', route =>
    route.fulfill({ status: 200, json: strategy })
  )
  await page.route('**/api/strategies/strat-1/versions/v1/code', route =>
    route.fulfill({ status: 200, json: { code: 'class MyStrat(QCAlgorithm): pass' } })
  )
  await page.route(/\/api\/jobs/, async (route) => {
    if (route.request().method() === 'POST') {
      await route.fulfill({ status: 202, json: { jobId: 'test-job-123' } })
    } else {
      await route.fulfill({ status: 200, json: { jobs: [], total: 0 } })
    }
  })

  await page.goto('/strategies/strat-1')

  // Wait for strategy to load and "Run Backtest" button to appear
  await expect(page.getByRole('button', { name: /run backtest/i })).toBeVisible()
  await page.getByRole('button', { name: /run backtest/i }).click()

  // Wait for modal to open
  await expect(page.getByText('Run Backtest').first()).toBeVisible()

  // Fill form using data-testid selectors
  await page.getByTestId('symbols-input').fill('SPY')
  await page.getByTestId('start-date-input').fill('2024-01-02')
  await page.getByTestId('end-date-input').fill('2024-03-31')

  // Submit the form
  await page.getByTestId('submit-backtest').click()

  await expect(page.getByTestId('job-queued-confirmation')).toBeVisible()
  await expect(page.getByText('Job queued!')).toBeVisible()
  await expect(page.getByText('test-job-123')).toBeVisible()
})

test('backtests page shows queued job in list', async ({ page }) => {
  await page.goto('/login')
  await setLoggedIn(page)

  await page.route('**/api/jobs**', route =>
    route.fulfill({
      status: 200,
      json: {
        jobs: [{
          id: 'test-job-123',
          status: 'queued',
          type: 'backtest',
          strategyName: 'Test Strategy',
          versionNumber: 1,
          dataSource: 'alpaca',
          createdAt: '2024-01-01T00:00:00Z',
          startedAt: null,
          completedAt: null,
        }],
        total: 1,
      },
    })
  )

  await page.goto('/backtests')
  // Status badge in the table (not the filter button, which also says "Queued")
  await expect(page.getByRole('cell').getByText('Queued')).toBeVisible()
})
