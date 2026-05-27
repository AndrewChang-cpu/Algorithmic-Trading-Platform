import { test, expect } from '@playwright/test'
import { mockOverviewAPIs } from './helpers'

test('login page shows email and password form', async ({ page }) => {
  await page.goto('/login')
  await expect(page.locator('input[type="email"]')).toBeVisible()
  await expect(page.locator('input[type="password"]')).toBeVisible()
  await expect(page.getByRole('button', { name: /sign in/i })).toBeVisible()
})

test('valid login redirects to overview', async ({ page }) => {
  mockOverviewAPIs(page)
  await page.route('**/api/auth/login', route =>
    route.fulfill({
      status: 200,
      json: { accessToken: 'tok', refreshToken: 'ref', userId: 'uid' },
    })
  )

  await page.goto('/login')
  await page.fill('input[type="email"]', 'user@example.com')
  await page.fill('input[type="password"]', 'password123')
  await page.getByRole('button', { name: /sign in/i }).click()

  await page.waitForURL('**/overview')
  expect(page.url()).toContain('/overview')
})

test('wrong password shows inline error', async ({ page }) => {
  await page.route('**/api/auth/login', route =>
    route.fulfill({ status: 401, json: { error: 'invalid credentials' } })
  )

  await page.goto('/login')
  await page.fill('input[type="email"]', 'user@example.com')
  await page.fill('input[type="password"]', 'wrongpass')
  await page.getByRole('button', { name: /sign in/i }).click()

  await expect(page.getByText('invalid credentials')).toBeVisible()
})

test('register with valid details redirects to overview', async ({ page }) => {
  mockOverviewAPIs(page)
  await page.route('**/api/auth/register', route =>
    route.fulfill({
      status: 201,
      json: { accessToken: 'tok', refreshToken: 'ref', userId: 'uid' },
    })
  )

  await page.goto('/register')
  await page.fill('input[type="email"]', 'new@example.com')
  await page.fill('input[type="password"]', 'password123')
  await page.getByRole('button', { name: /create account/i }).click()

  await page.waitForURL('**/overview')
  expect(page.url()).toContain('/overview')
})
