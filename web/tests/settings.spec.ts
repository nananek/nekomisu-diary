import { test, expect } from '@playwright/test'
import { loginUI, loginAPI, TEST_USER } from './helpers'

test.describe('Settings E2E', () => {
  test('navigate to settings page shows all sections', async ({ page }) => {
    await loginUI(page)
    await page.goto('/settings')
    await expect(page.getByRole('heading', { name: /プロフィール/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /アバター/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /パスワード/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /認証アプリ/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: /セキュリティキー/ })).toBeVisible()
  })

  test('display name update persists', async ({ page, context }) => {
    await loginUI(page)
    await page.goto('/settings')

    // Capture current display name (may differ across test runs)
    const input = page.locator('.settings-form input').first()
    await expect(input).toHaveValue(/./) // at least some value
    const original = await input.inputValue()

    const newName = `表示名_${Date.now()}`
    await input.fill(newName)
    await page.getByRole('button', { name: /保存/ }).first().click()

    // Verify via API that the server was updated
    await expect.poll(async () => {
      const resp = await context.request.get('/api/auth/me')
      const me = await resp.json()
      return me.display_name
    }).toBe(newName)

    // Restore
    await context.request.put('/api/auth/profile', { data: { display_name: original } })
  })

  test('wrong old password rejected', async ({ page }) => {
    await loginUI(page)
    await page.goto('/settings')
    await page.getByPlaceholder('現在のパスワード').fill('definitely_wrong')
    await page.getByPlaceholder(/新しいパスワード/).fill('newpassword123')
    await page.getByRole('button', { name: /変更する/ }).click()
    await expect(page.locator('.error')).toBeVisible()
  })

  test('TOTP setup starts and shows secret', async ({ page, context }) => {
    await loginAPI(page)

    // Ensure no TOTP is enabled (delete any existing for the test user)
    await context.request.delete('/api/auth/totp')

    await page.goto('/settings')
    await page.getByRole('button', { name: /設定する/ }).click()
    // Secret code input should appear
    await expect(page.getByPlaceholder(/6桁の認証コード/)).toBeVisible()

    // Clean up: don't leave a pending TOTP setup that affects other tests
    await context.request.delete('/api/auth/totp')
  })
})
