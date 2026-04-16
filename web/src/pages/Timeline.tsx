import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Post } from '../api'
import Icon from '../components/Icon'
import { useUnread } from '../hooks/useUnread'
import './Timeline.css'

export default function Timeline() {
  const [posts, setPosts] = useState<Post[]>([])
  const [page, setPage] = useState(1)
  const [pages, setPages] = useState(1)
  const [loaded, setLoaded] = useState(false)
  const { markSeen } = useUnread()

  useEffect(() => {
    let active = true
    api.posts(page)
      .then(d => {
        if (!active) return
        setPosts(d.posts)
        setPages(d.pages)
        setLoaded(true)
      })
      .then(() => markSeen())
    return () => { active = false }
  }, [page, markSeen])

  const loading = !loaded

  if (loading) return <div className="loading">読み込み中...</div>

  return (
    <div className="timeline">
      {posts.length === 0 && <p className="empty">まだ日記がありません</p>}
      {posts.map(p => (
        <article key={p.id} className="post-card card">
          <div className="post-header">
            <Link to={`/members/${p.author_id}`} className="post-author">
              {p.author_avatar
                ? <img className="avatar" src={p.author_avatar} alt="" />
                : <span className="avatar placeholder">{p.author_name[0]}</span>
              }
              <span className="author-name">{p.author_name}</span>
            </Link>
            <span className="meta">
              {p.published_at ? new Date(p.published_at).toLocaleDateString('ja-JP') : '下書き'}
              {p.visibility === 'private' && ' (自分のみ)'}
            </span>
          </div>
          <Link to={`/posts/${p.id}`} className="post-title-link">
            <h2>{p.title}</h2>
          </Link>
          <div className="post-excerpt" dangerouslySetInnerHTML={{ __html: p.body_html.slice(0, 300) }} />
          <div className="post-footer">
            <Link to={`/posts/${p.id}`} className="meta footer-link">
              <Icon name="comment" size={14} />
              {p.comment_count > 0 ? `${p.comment_count}件のコメント` : 'つづきを読む'}
            </Link>
          </div>
        </article>
      ))}
      {pages > 1 && (
        <div className="pagination">
          <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>前へ</button>
          <span className="meta">{page} / {pages}</span>
          <button disabled={page >= pages} onClick={() => setPage(p => p + 1)}>次へ</button>
        </div>
      )}
    </div>
  )
}
