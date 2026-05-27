import { Page } from '@playwright/test'

// Simulate being logged in by setting the Zustand persist state in localStorage.
// Must be called after navigating to any page so the browser context is initialized.
export async function setLoggedIn(page: Page, userId = 'test-user-id', email = 'test@example.com') {
  await page.evaluate(({ userId, email }) => {
    localStorage.setItem('atp-auth', JSON.stringify({
      state: {
        user: { id: userId, email },
        accessToken: 'fake-access-token',
        refreshToken: 'fake-refresh-token',
      },
      version: 0,
    }))
  }, { userId, email })
}

// Mock all Overview page API calls so post-login redirect doesn't error.
export function mockOverviewAPIs(page: Page) {
  page.route('**/api/health', route =>
    route.fulfill({ status: 200, json: { kafka: 'ok', redis: 'ok', db: 'ok' } })
  )
  page.route('**/api/jobs?*', route =>
    route.fulfill({ status: 200, json: { jobs: [], total: 0 } })
  )
  page.route('**/api/strategies', route =>
    route.fulfill({ status: 200, json: [] })
  )
}
