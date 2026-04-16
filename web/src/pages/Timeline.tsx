import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Post } from '../api'
import './Timeline.css'

export default function Timeline() {
  const [posts, setPosts] = useState<Post[]>([])
  const [page, setPage] = useState(1)
  const [pages, setPages] = useState(1)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    api.posts(page).then(d => {
      setPosts(d.posts)
      setPages(d.pages)
    }).finally(() => setLoading(false))
  }, [page])

  if (loading) return <div className="loading">Loading...</div>

  return (
    <div className="timeline">
      {posts.length === 0 && <p>No posts yet.</p>}
      {posts.map(p => (
        <article key={p.id} className="post-card card">
          <div className="post-header">
            <div className="post-author">
              {p.author_avatar
                ? <img className="avatar" src={p.author_avatar} alt="" />
                : <span className="avatar placeholder">{p.author_name[0]}</span>
              }
              <span className="author-name">{p.author_name}</span>
            </div>
            <span className="meta">
              {p.published_at ? new Date(p.published_at).toLocaleDateString('ja-JP') : 'draft'}
              {p.visibility === 'private' && ' (private)'}
            </span>
          </div>
          <Link to={`/posts/${p.id}`} className="post-title-link">
            <h2>{p.title}</h2>
          </Link>
          <div
            className="post-excerpt"
            dangerouslySetInnerHTML={{ __html: p.body_html.slice(0, 300) }}
          />
          <div className="post-footer meta">
            <Link to={`/posts/${p.id}`}>
              {p.comment_count > 0 ? `${p.comment_count} comments` : 'Read more'}
            </Link>
          </div>
        </article>
      ))}
      {pages > 1 && (
        <div className="pagination">
          <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Prev</button>
          <span className="meta">{page} / {pages}</span>
          <button disabled={page >= pages} onClick={() => setPage(p => p + 1)}>Next</button>
        </div>
      )}
    </div>
  )
}
