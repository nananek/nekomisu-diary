export interface User {
  id: number
  login: string
  display_name: string
  avatar_path: string | null
  has_2fa: boolean
}

export interface Post {
  id: number
  author_id: number
  author_name: string
  author_avatar: string | null
  title: string
  body_html: string
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

export const api = {
  login: (login: string, password: string) =>
    request('/api/auth/login', { method: 'POST', body: JSON.stringify({ login, password }) }),

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

  posts: (page = 1) =>
    request<{ posts: Post[]; total: number; page: number; pages: number }>(
      `/api/posts?page=${page}`,
    ),

  post: (id: number) => request<Post>(`/api/posts/${id}`),

  createPost: (title: string, body: string, visibility = 'public') =>
    request<{ id: number }>('/api/posts', {
      method: 'POST',
      body: JSON.stringify({ title, body, visibility }),
    }),

  updatePost: (id: number, data: { title?: string; body?: string; visibility?: string }) =>
    request(`/api/posts/${id}`, { method: 'PUT', body: JSON.stringify(data) }),

  deletePost: (id: number) => request(`/api/posts/${id}`, { method: 'DELETE' }),

  comments: (postId: number) =>
    request<{ comments: Comment[] }>(`/api/posts/${postId}/comments`),

  createComment: (postId: number, body: string, parentId?: number) =>
    request<{ id: number }>(`/api/posts/${postId}/comments`, {
      method: 'POST',
      body: JSON.stringify({ body, parent_id: parentId }),
    }),
}
