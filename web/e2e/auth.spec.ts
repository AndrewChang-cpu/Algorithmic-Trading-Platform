import { test, expect } from '@playwright/test'
import { mockOverviewAPIs } from './helpers'

test('login page shows email and password form', async ({ page }) => {
  await page.goto('/login')
  await expect(page.getByTestId('email-input')).toBeVisible()
  await expect(page.getByTestId('password-input')).toBeVisible()
  await expect(page.getByTestId('login-submit')).toBeVisible()
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
  await page.getByTestId('email-input').fill('user@example.com')
  await page.getByTestId('password-input').fill('password123')
  await page.getByTestId('login-submit').click()

  await page.waitForURL('**/overview')
  expect(page.url()).toContain('/overview')
})

test('wrong password shows inline error', async ({ page }) => {
  await page.route('**/api/auth/login', route =>
    route.fulfill({ status: 401, json: { error: 'invalid credentials' } })
  )

  await page.goto('/login')
  await page.getByTestId('email-input').fill('user@example.com')
  await page.getByTestId('password-input').fill('wrongpass')
  await page.getByTestId('login-submit').click()

  await expect(page.getByTestId('auth-error')).toBeVisible()
  await expect(page.getByTestId('auth-error')).toContainText('invalid credentials')
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
  await page.getByTestId('register-email-input').fill('new@example.com')
  await page.getByTestId('register-password-input').fill('password123')
  await page.getByTestId('register-submit').click()

  await page.waitForURL('**/overview')
  expect(page.url()).toContain('/overview')
})
