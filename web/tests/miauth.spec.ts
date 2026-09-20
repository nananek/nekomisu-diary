import { test, expect } from '@playwright/test'

// Exercises the real MiAuth ("Misskeyでログイン") flow end to end through a
// browser: click the button on /login, follow the redirect out to a mock
// Misskey instance (cmd/mockmisskey) that auto-approves, follow the
// redirect back to /login?session=..., and land signed in. Requires the
// diary server under test to be started with
// -misskey-instance pointing at a running mockmisskey, with the seeded
// TEST_USER linked to mockmisskey's -username via cmd/miauthlink (see
// .github/workflows/ci.yml's e2e job).
test.describe('MiAuth (Misskey) login', () => {
  test('signs in and lands on the timeline', async ({ page }) => {
    await page.goto('/login')
    const misskeyBtn = page.getByRole('button', { name: /Misskeyでログイン/ })
    await expect(misskeyBtn).toBeVisible()
    await misskeyBtn.click()

    // Two cross-origin redirects happen here (diary -> mockmisskey ->
    // diary), so this is a real navigation, not SPA routing.
    await page.waitForURL(/\/$/, { timeout: 10_000 })
    await expect(page.locator('.site-title')).toContainText('ねこのみすきー交換日記')

    const me = await page.context().request.get('/api/auth/me')
    expect(me.ok()).toBe(true)
  })

  // The suspicious part: while POST /api/auth/miauth/finish is in flight
  // after landing back on /login?session=..., does the visitor see the
  // full password-entry form for that window (indistinguishable from
  // being bounced back to a fresh manual login), or something that makes
  // clear a Misskey login is already in progress? Gate the request on a
  // promise this test controls, instead of a fixed delay, so the
  // mid-flight assertion can't race the network and can hold the state
  // open for as long as needed to inspect it.
  test('does not show the password form while the session token resolves', async ({ page }) => {
    let releaseFinish = () => {}
    const finishGate = new Promise<void>(resolve => { releaseFinish = resolve })
    await page.route('**/api/auth/miauth/finish', async route => {
      await finishGate
      await route.continue()
    })

    await page.goto('/login')
    await page.getByRole('button', { name: /Misskeyでログイン/ }).click()
    await page.waitForURL(/\/login\?session=/, { timeout: 10_000 })

    // The finish request is now parked on finishGate, so the page is
    // guaranteed to still be in the mid-flight state — no timing luck
    // needed for this to be the right moment to assert.
    await expect(page.getByPlaceholder('パスワード')).not.toBeVisible()
    await expect(page.getByPlaceholder('ログインID')).not.toBeVisible()

    releaseFinish()
    await page.waitForURL(/\/$/, { timeout: 10_000 })
    await expect(page.locator('.site-title')).toContainText('ねこのみすきー交換日記')
  })
})
