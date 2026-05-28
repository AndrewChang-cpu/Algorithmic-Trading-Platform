import { test, expect } from '@playwright/test'
import { setLoggedIn } from './helpers'

test('strategies page shows empty state when no strategies', async ({ page }) => {
  await page.goto('/login')
  await setLoggedIn(page)

  await page.route('**/api/strategies', route =>
    route.fulfill({ status: 200, json: [] })
  )

  await page.goto('/strategies')
  await expect(page.getByText(/no strategies yet/i)).toBeVisible()
})

test('strategies page shows strategy name in table', async ({ page }) => {
  await page.goto('/login')
  await setLoggedIn(page)

  await page.route('**/api/strategies', route =>
    route.fulfill({
      status: 200,
      json: [{ id: 's1', name: 'My Strategy', latestVersion: 1, runCount: 0, bestSharpe: null, createdAt: '2024-01-01' }],
    })
  )

  await page.goto('/strategies')
  await expect(page.getByText('My Strategy')).toBeVisible()
})

test('upload modal success shows uploaded confirmation', async ({ page }) => {
  await page.goto('/login')
  await setLoggedIn(page)

  await page.route('**/api/strategies', route =>
    route.fulfill({ status: 200, json: [] })
  )
  await page.route('**/api/strategies', async (route) => {
    if (route.request().method() === 'POST') {
      await route.fulfill({ status: 201, json: { strategyId: 'new-id', versionId: 'v1', versionNumber: 1 } })
    } else {
      await route.fulfill({ status: 200, json: [] })
    }
  })

  await page.goto('/strategies')
  await page.getByRole('button', { name: /upload strategy/i }).first().click()

  // Step 1: file drop zone — select file then click "Next"
  await expect(page.getByText(/drag.*drop/i)).toBeVisible()
  await page.setInputFiles('[data-testid="upload-file-input"]', {
    name: 'strategy.py',
    mimeType: 'text/plain',
    buffer: Buffer.from('class MyStrat(QCAlgorithm):\n    pass\n'),
  })
  await page.getByRole('button', { name: 'Next' }).click()

  // Step 2: strategy name
  await page.getByTestId('strategy-name-input').fill('My Strategy')
  await page.getByTestId('upload-submit').click()

  await expect(page.getByTestId('upload-success')).toBeVisible()
  await expect(page.getByTestId('upload-success')).toContainText(/strategy uploaded/i)
})

test('upload modal error shows violation text', async ({ page }) => {
  await page.goto('/login')
  await setLoggedIn(page)

  await page.route('**/api/strategies', async (route) => {
    if (route.request().method() === 'POST') {
      await route.fulfill({ status: 422, json: { error: 'import os detected on line 1' } })
    } else {
      await route.fulfill({ status: 200, json: [] })
    }
  })

  await page.goto('/strategies')
  await page.getByRole('button', { name: /upload strategy/i }).first().click()

  await page.setInputFiles('[data-testid="upload-file-input"]', {
    name: 'bad.py',
    mimeType: 'text/plain',
    buffer: Buffer.from('import os\nclass MyStrat(QCAlgorithm): pass\n'),
  })
  await page.getByRole('button', { name: 'Next' }).click()

  await page.getByTestId('strategy-name-input').fill('Bad Strategy')
  await page.getByTestId('upload-submit').click()

  await expect(page.getByTestId('upload-error')).toBeVisible()
  await expect(page.getByTestId('upload-error')).toContainText(/import os/i)
})
