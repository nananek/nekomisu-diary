import { test, expect } from '@playwright/test'
import { loginUI, TEST_USER } from './helpers'

test.describe('Authentication', () => {
  test('invalid credentials show error', async ({ page }) => {
    await page.goto('/login')
    await page.getByPlaceholder('ログインID').fill('nonexistent')
    await page.getByPlaceholder('パスワード').fill('wrongpass')
    await page.getByRole('button', { name: 'ログイン' }).click()
    await expect(page.locator('.error')).toBeVisible()
    await expect(page).toHaveURL(/\/login/)
  })

  test('login redirects to timeline', async ({ page }) => {
    await loginUI(page)
    await expect(page).toHaveURL('/')
    await expect(page.locator('.site-title')).toContainText('ねこのみすきー交換日記')
  })

  test('logout returns to login', async ({ page }) => {
    await loginUI(page)
    // Mobile: open hamburger menu; desktop: direct click
    const isDesktop = page.viewportSize()!.width >= 900
    if (!isDesktop) {
      await page.locator('.hamburger').click()
    }
    await page.getByRole('button', { name: 'ログアウト' }).click()
    await page.waitForURL('/login')
  })

  test('me endpoint returns logged-in user after login', async ({ page, context }) => {
    await loginUI(page)
    const resp = await context.request.get('/api/auth/me')
    expect(resp.ok()).toBe(true)
    const me = await resp.json()
    expect(me.login).toBe(TEST_USER.login)
  })

  test('unauthenticated access redirects to login', async ({ page }) => {
    await page.goto('/')
    await page.waitForURL(/\/login/)
  })
})
