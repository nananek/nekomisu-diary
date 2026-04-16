import { test, expect } from '@playwright/test'

/**
 * Layout tests
 * Run against dev env (localhost:3000).
 * Uses known dev credentials (nananek / changeme123).
 */

const LOGIN = 'nananek'
const PASSWORD = 'changeme123'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByPlaceholder('ログインID').fill(LOGIN)
  await page.getByPlaceholder('パスワード').fill(PASSWORD)
  await page.getByRole('button', { name: 'ログイン' }).click()
  await page.waitForURL('/')
}

test.describe('Mobile layout', () => {
  test.skip(({ viewport }) => (viewport?.width ?? 0) >= 900, 'mobile only')

  test('bottom nav stays at bottom after scroll', async ({ page }) => {
    await login(page)

    const bottomNav = page.locator('.bottom-nav')
    const mainContent = page.locator('.main-content')

    await expect(bottomNav).toBeVisible()

    const viewportHeight = page.viewportSize()!.height

    // Before scroll: bottom-nav bottom edge is at viewport bottom
    const rect1 = await bottomNav.boundingBox()
    expect(rect1).not.toBeNull()
    expect(rect1!.y + rect1!.height).toBeCloseTo(viewportHeight, 0)

    // topbar should be at top (y near 0)
    const topbar = page.locator('.topbar')
    const topRect = await topbar.boundingBox()
    expect(topRect!.y).toBeLessThan(50)

    // Scroll within main-content
    await mainContent.evaluate(el => el.scrollTop = 500)
    await page.waitForTimeout(100)

    // Bottom nav still at bottom
    const rect2 = await bottomNav.boundingBox()
    expect(rect2!.y + rect2!.height).toBeCloseTo(viewportHeight, 0)

    // Topbar still at top
    const topRect2 = await topbar.boundingBox()
    expect(topRect2!.y).toBeLessThan(50)
  })

  test('bottom nav is BELOW main content in DOM order', async ({ page }) => {
    await login(page)
    const order = await page.evaluate(() => {
      const main = document.querySelector('.main-content')
      const nav = document.querySelector('.bottom-nav')
      if (!main || !nav) return 'missing'
      // compareDocumentPosition: 4 = FOLLOWING
      return (main.compareDocumentPosition(nav) & Node.DOCUMENT_POSITION_FOLLOWING) ? 'nav-after-main' : 'main-after-nav'
    })
    expect(order).toBe('nav-after-main')
  })

  test('login page renders title', async ({ page }) => {
    await page.goto('/login')
    await expect(page.getByRole('heading', { name: 'ねこのみすきー交換日記' })).toBeVisible()
  })

  test('theme toggle works', async ({ page }) => {
    // Theme button only shows inside Layout (needs login)
    await login(page)
    const theme = page.locator('button[title^="テーマ"]')
    await expect(theme).toBeVisible()

    const htmlEl = page.locator('html')
    await theme.click()
    // after one click, should change to 'light' and set data-theme
    await page.waitForTimeout(100)
    const attr1 = await htmlEl.getAttribute('data-theme')
    expect(['light', 'dark']).toContain(attr1)

    await theme.click()
    await page.waitForTimeout(100)
    const attr2 = await htmlEl.getAttribute('data-theme')
    expect(attr2).not.toBe(attr1)
  })
})

test.describe('Desktop layout', () => {
  test.skip(({ viewport }) => (viewport?.width ?? 0) < 900, 'desktop only')

  test('topbar renders on a single row with reasonable height', async ({ page }) => {
    await login(page)
    const topbar = page.locator('.topbar')
    const box = await topbar.boundingBox()
    // A healthy topbar is ~50px tall. A broken one (previous bug:
    // site-title wrapping one Japanese char per line) would be 300px+.
    expect(box!.height).toBeLessThan(120)

    // site-title should not be vertically-wrapped (was 10×318 in the bug)
    const titleBox = await page.locator('.site-title').boundingBox()
    expect(titleBox!.width).toBeGreaterThan(80)
    expect(titleBox!.height).toBeLessThan(50)

    // Every topnav child sits on the same horizontal row
    const ys = await page.locator('.topnav > *').evaluateAll(els =>
      els.map(e => Math.round(e.getBoundingClientRect().y))
    )
    expect(new Set(ys).size).toBe(1)
  })

  test('bottom nav hidden, topnav visible', async ({ page }) => {
    await login(page)
    await expect(page.locator('.bottom-nav')).toBeHidden()
    await expect(page.locator('.topnav').first()).toBeVisible()
  })
})
