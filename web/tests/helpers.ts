import type { Page, APIRequestContext } from '@playwright/test'

export const TEST_USER = {
  login: 'nananek',
  password: 'changeme123',
}

export async function loginUI(page: Page, creds = TEST_USER) {
  await page.goto('/login')
  await page.getByPlaceholder('ログインID').fill(creds.login)
  await page.getByPlaceholder('パスワード').fill(creds.password)
  await page.getByRole('button', { name: 'ログイン' }).click()
  await page.waitForURL('/')
}

export async function loginAPI(page: Page, creds = TEST_USER) {
  const ctx = page.context()
  await ctx.request.post('/api/auth/login', { data: creds })
}

/** Create a post via API and return its id. */
export async function createPost(
  req: APIRequestContext,
  title: string,
  body: string,
  visibility = 'public',
): Promise<number> {
  const resp = await req.post('/api/posts', { data: { title, body, visibility } })
  if (!resp.ok()) throw new Error(`createPost failed: ${resp.status()}`)
  const j = await resp.json()
  return j.id
}

/** Delete a post via API (idempotent cleanup). */
export async function deletePost(req: APIRequestContext, id: number) {
  await req.delete(`/api/posts/${id}`)
}

/** Create a comment via API and return its id. */
export async function createComment(req: APIRequestContext, postId: number, body: string): Promise<number> {
  const resp = await req.post(`/api/posts/${postId}/comments`, { data: { body } })
  if (!resp.ok()) throw new Error(`createComment failed: ${resp.status()}`)
  return (await resp.json()).id
}
