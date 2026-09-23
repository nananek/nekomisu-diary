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

test.describe('Security', () => {
  test('uploaded files require a session', async ({ request }) => {
    const resp = await request.get('/uploads/avatars/1.png')
    expect(resp.status()).toBe(401)
  })

  test('baseline security headers are set', async ({ request }) => {
    const resp = await request.get('/manifest.json')
    expect(resp.headers()['x-content-type-options']).toBe('nosniff')
    expect(resp.headers()['x-frame-options']).toBe('DENY')
    const csp = resp.headers()['content-security-policy']
    expect(csp).toContain("default-src 'self'")
    expect(csp).toContain("script-src 'self'")
    expect(csp).toContain("script-src-attr 'none'")
    expect(csp).toContain("connect-src 'self'")
    expect(csp).toContain("frame-ancestors 'none'")
  })

  test('app pages load without CSP violations', async ({ page }) => {
    await page.addInitScript(() => {
      const w = window as unknown as { __cspViolations: string[] }
      w.__cspViolations = []
      document.addEventListener('securitypolicyviolation', e => {
        w.__cspViolations.push(`${e.violatedDirective} ${e.blockedURI}`)
      })
    })
    const collect = () =>
      page.evaluate(() => (window as unknown as { __cspViolations?: string[] }).__cspViolations ?? [])

    // Login page first (its document is replaced once we log in).
    await page.goto('/login')
    await expect(page.getByPlaceholder('ログインID')).toBeVisible()
    const violations: string[] = [...await collect()]

    await loginUI(page)
    for (const path of ['/', '/new', '/search', '/drafts', '/members', '/media', '/settings']) {
      await page.goto(path)
      await expect(page.locator('.layout')).toBeVisible()
      violations.push(...await collect())
    }

    // Post detail page (sanitized body + comments section).
    const created = await page.context().request.post('/api/posts', {
      data: { title: `CSP_${Date.now()}`, body: '<p>x</p>', visibility: 'public' },
    })
    const postID = (await created.json()).id
    await page.goto(`/posts/${postID}`)
    await expect(page.locator('.post-view')).toBeVisible()
    violations.push(...await collect())
    await page.context().request.delete(`/api/posts/${postID}`)

    expect(violations).toEqual([])
  })

  test('CSP blocks injected inline script', async ({ page }) => {
    await loginUI(page)
    const ran = await page.evaluate(() => {
      const w = window as unknown as { __cspInlineRan?: boolean }
      const s = document.createElement('script')
      s.textContent = 'window.__cspInlineRan = true'
      document.body.appendChild(s)
      s.remove()
      return w.__cspInlineRan === true
    })
    expect(ran).toBe(false)
  })
})
