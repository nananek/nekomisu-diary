import { test, expect } from '@playwright/test'
import { loginAPI, createPost, deletePost, createComment } from './helpers'

test.describe('Posts E2E', () => {
  let createdIds: number[] = []

  test.beforeEach(async ({ page }) => {
    await loginAPI(page)
  })

  test.afterEach(async ({ page }) => {
    // Clean up any posts created during the test
    for (const id of createdIds) {
      await deletePost(page.context().request, id)
    }
    createdIds = []
  })

  test('create, view, edit, delete post', async ({ page }) => {
    const title = `テスト投稿_${Date.now()}`

    // Create via API (faster and deterministic)
    const id = await createPost(page.context().request, title, '<p>初期本文</p>')
    createdIds.push(id)

    // View it on timeline
    await page.goto('/')
    await expect(page.getByRole('heading', { name: title })).toBeVisible()

    // Open detail page
    await page.getByRole('heading', { name: title }).click()
    await page.waitForURL(`/posts/${id}`)
    await expect(page.getByRole('heading', { name: title, level: 1 })).toBeVisible()
    await expect(page.locator('.post-body')).toContainText('初期本文')

    // Edit via UI
    await page.getByRole('link', { name: /編集/ }).click()
    await page.waitForURL(`/posts/${id}/edit`)
    const titleInput = page.locator('input').first()
    await titleInput.fill(title + '_編集後')
    await page.getByRole('button', { name: /保存/ }).click()
    await page.waitForURL(`/posts/${id}`)
    await expect(page.getByRole('heading', { name: title + '_編集後' })).toBeVisible()

    // Delete via UI (auto-confirm)
    page.on('dialog', d => d.accept())
    await page.locator('button.danger').click()
    await page.waitForURL('/')
    createdIds = createdIds.filter(x => x !== id)

    // Should not be on timeline
    await expect(page.getByRole('heading', { name: title + '_編集後' })).toHaveCount(0)
  })

  test('timeline lists posts in reverse chronological order', async ({ page }) => {
    const title1 = `先_${Date.now()}`
    const id1 = await createPost(page.context().request, title1, '<p>x</p>')
    createdIds.push(id1)
    await new Promise(r => setTimeout(r, 50))
    const title2 = `後_${Date.now()}`
    const id2 = await createPost(page.context().request, title2, '<p>y</p>')
    createdIds.push(id2)

    await page.goto('/')
    await expect(page.getByRole('heading', { name: title2 })).toBeVisible()
    await expect(page.getByRole('heading', { name: title1 })).toBeVisible()
    const titles = await page.locator('article h2').allTextContents()
    const idx1 = titles.indexOf(title1)
    const idx2 = titles.indexOf(title2)
    expect(idx2).toBeLessThan(idx1) // newer (title2) first
  })

  test('draft post does not appear on public timeline', async ({ page }) => {
    const title = `下書き_${Date.now()}`
    const id = await createPost(page.context().request, title, '<p>x</p>', 'draft')
    createdIds.push(id)

    await page.goto('/')
    await expect(page.getByRole('heading', { name: title })).toHaveCount(0)

    // But shows in drafts
    await page.goto('/drafts')
    await expect(page.getByRole('heading', { name: title })).toBeVisible()
  })

  test('search finds post by title', async ({ page }) => {
    const marker = `search${Date.now()}`
    const id = await createPost(page.context().request, `${marker}タイトル`, '<p>body</p>')
    createdIds.push(id)

    await page.goto('/search')
    await page.getByPlaceholder('日記を検索...').fill(marker)
    await page.locator('form button[type="submit"]').click()
    await expect(page.getByRole('heading', { name: `${marker}タイトル` })).toBeVisible()
  })
})

test.describe('Comments E2E', () => {
  let createdPosts: number[] = []

  test.beforeEach(async ({ page }) => {
    await loginAPI(page)
  })

  test.afterEach(async ({ page }) => {
    for (const id of createdPosts) {
      await deletePost(page.context().request, id)
    }
    createdPosts = []
  })

  test('add and view comment', async ({ page }) => {
    const id = await createPost(page.context().request, `コメントテスト_${Date.now()}`, '<p>x</p>')
    createdPosts.push(id)

    await page.goto(`/posts/${id}`)
    const body = `テストコメント_${Date.now()}`
    await page.getByPlaceholder('コメントを書く...').fill(body)
    await page.getByRole('button', { name: /送信/ }).click()

    await expect(page.locator('.comment').filter({ hasText: body })).toBeVisible()
  })

  test('delete own comment', async ({ page }) => {
    const postId = await createPost(page.context().request, `コメント削除_${Date.now()}`, '<p>x</p>')
    createdPosts.push(postId)
    const body = `消える_${Date.now()}`
    await createComment(page.context().request, postId, body)

    await page.goto(`/posts/${postId}`)
    const comment = page.locator('.comment').filter({ hasText: body })
    await expect(comment).toBeVisible()

    // Click delete button (trash icon) inside the comment
    page.on('dialog', d => d.accept())
    await comment.locator('button[title="削除"]').click()
    await expect(comment).toHaveCount(0)
  })
})
