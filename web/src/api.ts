export interface User {
  id: number
  login: string
  display_name: string
  avatar_path: string | null
  has_2fa: boolean
  has_totp: boolean
  has_webauthn: boolean
}

export interface Post {
  id: number
  author_id: number
  author_name: string
  author_avatar: string | null
  title: string
  /** Full body HTML. Empty on list endpoints; populated only on GET /api/posts/:id. */
  body_html: string
  body_md?: string | null
  /** Plain-text excerpt (no HTML). Populated on list endpoints. */
  excerpt: string
  visibility: string
  published_at: string | null
  created_at: string
  comment_count: number
}

export interface Comment {
  id: number
  post_id: number
  author_id: number | null
  author_name: string
  author_avatar: string | null
  body: string
  parent_id: number | null
  created_at: string
}

export interface LoginResult {
  ok?: boolean
  requires_2fa?: boolean
  has_totp?: boolean
  has_webauthn?: boolean
}

export interface WebAuthnCredential {
  id: string
  name: string
  created_at: string
}

export interface Member {
  id: number
  login: string
  display_name: string
  avatar_path: string | null
  created_at: string
  post_count: number
  comment_count: number
}

async function request<T>(url: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    ...opts,
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Request failed')
  return data
}

async function uploadFile<T>(url: string, file: File, extra?: Record<string, string>): Promise<T> {
  const form = new FormData()
  form.append('file', file)
  if (extra) {
    for (const [k, v] of Object.entries(extra)) form.append(k, v)
  }
  const res = await fetch(url, { method: 'POST', credentials: 'same-origin', body: form })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Upload failed')
  return data
}

export const api = {
  // Auth
  login: (login: string, password: string) =>
    request<LoginResult>('/api/auth/login', { method: 'POST', body: JSON.stringify({ login, password }) }),

  register: (login: string, email: string, display_name: string, password: string) =>
    request('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify({ login, email, display_name, password }),
    }),

  logout: () => request('/api/auth/logout', { method: 'POST' }),
  me: () => request<User>('/api/auth/me'),

  changePassword: (old_password: string, new_password: string) =>
    request('/api/auth/password', {
      method: 'PUT',
      body: JSON.stringify({ old_password, new_password }),
    }),

  updateProfile: (data: { display_name?: string; email?: string }) =>
    request('/api/auth/profile', { method: 'PUT', body: JSON.stringify(data) }),

  uploadAvatar: (file: File) =>
    uploadFile<{ avatar_path: string }>('/api/auth/avatar', file),

  // TOTP
  totpSetup: () => request<{ secret: string; url: string }>('/api/auth/totp/setup', { method: 'POST' }),
  totpConfirm: (code: string) =>
    request('/api/auth/totp/confirm', { method: 'POST', body: JSON.stringify({ code }) }),
  totpDisable: () => request('/api/auth/totp', { method: 'DELETE' }),
  totpVerifyLogin: (code: string) =>
    request('/api/auth/totp/verify-login', { method: 'POST', body: JSON.stringify({ code }) }),

  // WebAuthn
  webauthnRegisterBegin: () =>
    request<import('./lib/webauthn').RegistrationOptionsJSON>('/api/auth/webauthn/register/begin', { method: 'POST' }),
  webauthnRegisterFinish: (credential: unknown): Promise<{ ok?: boolean; error?: string }> =>
    fetch('/api/auth/webauthn/register/finish', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(credential),
    }).then(r => r.json()),
  webauthnCredentials: () =>
    request<{ credentials: WebAuthnCredential[] }>('/api/auth/webauthn/credentials'),
  webauthnDeleteCredential: (id: string) =>
    request(`/api/auth/webauthn/credentials/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  webauthnLoginBegin: () =>
    request<import('./lib/webauthn').LoginOptionsJSON>('/api/auth/webauthn/login/begin', { method: 'POST' }),
  webauthnLoginFinish: (credential: unknown): Promise<{ ok?: boolean; error?: string }> =>
    fetch('/api/auth/webauthn/login/finish', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(credential),
    }).then(r => r.json()),

  // Posts
  posts: (page = 1) =>
    request<{ posts: Post[]; total: number; page: number; pages: number }>(`/api/posts?page=${page}`),
  post: (id: number) => request<Post>(`/api/posts/${id}`),
  createPost: (title: string, body: string, visibility = 'public', bodyMd?: string) =>
    request<{ id: number }>('/api/posts', {
      method: 'POST',
      body: JSON.stringify({ title, body, body_md: bodyMd, visibility }),
    }),
  updatePost: (id: number, data: { title?: string; body?: string; body_md?: string | null; visibility?: string }) =>
    request(`/api/posts/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deletePost: (id: number) => request(`/api/posts/${id}`, { method: 'DELETE' }),
  drafts: () => request<{ posts: Post[] }>('/api/posts/drafts'),
  search: (q: string) => request<{ posts: Post[]; total: number }>(`/api/posts/search?q=${encodeURIComponent(q)}`),
  userPosts: (userId: number, page = 1) =>
    request<{ posts: Post[]; total: number; page: number; pages: number }>(`/api/users/${userId}/posts?page=${page}`),

  // Members
  members: () => request<{ members: Member[] }>('/api/members'),
  user: (id: number) => request<Member>(`/api/users/${id}`),

  // Unread
  unread: () => request<{ unread: number; last_seen: string }>('/api/unread'),
  markSeen: () => request('/api/unread/mark-seen', { method: 'POST' }),

  // Comments
  comments: (postId: number) =>
    request<{ comments: Comment[] }>(`/api/posts/${postId}/comments`),
  createComment: (postId: number, body: string, parentId?: number) =>
    request<{ id: number }>(`/api/posts/${postId}/comments`, {
      method: 'POST',
      body: JSON.stringify({ body, parent_id: parentId }),
    }),
  deleteComment: (commentId: number) =>
    request(`/api/comments/${commentId}`, { method: 'DELETE' }),

  // Media
  uploadMedia: (file: File, postId?: number) =>
    uploadFile<{ id: number; url: string; path: string }>(
      '/api/media/upload', file, postId ? { post_id: String(postId) } : undefined,
    ),
  listMedia: () => request<{ items: MediaItem[] }>('/api/media'),
  deleteMedia: (id: number) => request(`/api/media/${id}`, { method: 'DELETE' }),
}

export interface MediaItem {
  id: number
  filename: string
  url: string
  thumbnail_url?: string
  mime_type: string
  byte_size?: number
  width?: number
  height?: number
  created_at: string
}
