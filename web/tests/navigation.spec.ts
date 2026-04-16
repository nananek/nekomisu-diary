import { test, expect } from '@playwright/test'
import { loginUI } from './helpers'

test.describe('Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await loginUI(page)
  })

  test('mobile bottom nav links work', async ({ page }) => {
    test.skip(page.viewportSize()!.width >= 900, 'mobile only')

    await page.locator('.bottom-nav').getByText('検索').click()
    await expect(page).toHaveURL('/search')

    await page.locator('.bottom-nav').getByText('メンバー').click()
    await expect(page).toHaveURL('/members')

    await page.locator('.bottom-nav').getByText('ホーム').click()
    await expect(page).toHaveURL('/')
  })

  test('desktop top nav links work', async ({ page }) => {
    test.skip(page.viewportSize()!.width < 900, 'desktop only')

    await page.getByRole('link', { name: /検索/ }).click()
    await expect(page).toHaveURL('/search')

    await page.getByRole('link', { name: /メンバー/ }).click()
    await expect(page).toHaveURL('/members')

    await page.getByRole('link', { name: /下書き/ }).click()
    await expect(page).toHaveURL('/drafts')
  })

  test('members page lists users', async ({ page }) => {
    await page.goto('/members')
    await expect(page.getByRole('heading', { name: /メンバー/ })).toBeVisible()
    // At least one member card
    await expect(page.locator('.card').first()).toBeVisible()
  })

  test('clicking member goes to their post list', async ({ page }) => {
    await page.goto('/members')
    const firstMember = page.locator('.member-card').first()
    await firstMember.click()
    await expect(page).toHaveURL(/\/members\/\d+/)
  })

  test('theme preference persists across page reloads', async ({ page }) => {
    const themeBtn = page.locator('button[title^="テーマ"]')
    const html = page.locator('html')

    await themeBtn.click() // -> light
    await page.waitForTimeout(50)
    const theme1 = await html.getAttribute('data-theme')
    expect(['light', 'dark']).toContain(theme1)

    await page.reload()
    await page.waitForTimeout(200)
    const theme2 = await html.getAttribute('data-theme')
    expect(theme2).toBe(theme1)
  })
})

test.describe('PWA assets', () => {
  test('manifest.json served with expected fields', async ({ request }) => {
    const resp = await request.get('/manifest.json')
    expect(resp.ok()).toBe(true)
    const m = await resp.json()
    expect(m.name).toBe('ねこのみすきー交換日記')
    expect(m.display).toBe('standalone')
    expect(m.icons.length).toBeGreaterThanOrEqual(2)
  })

  test('service worker served', async ({ request }) => {
    const resp = await request.get('/sw.js')
    expect(resp.ok()).toBe(true)
    const body = await resp.text()
    expect(body).toContain('CACHE_NAME')
  })

  test('icons served', async ({ request }) => {
    const resp = await request.get('/icons/icon-192.png')
    expect(resp.ok()).toBe(true)
  })
})
