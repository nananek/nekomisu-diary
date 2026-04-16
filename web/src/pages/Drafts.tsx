import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Post } from '../api'

export default function Drafts() {
  const [drafts, setDrafts] = useState<Post[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.drafts().then(d => setDrafts(d.posts)).finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="loading">Loading...</div>

  return (
    <div className="timeline">
      <h2>My Drafts</h2>
      {drafts.length === 0 && <p>No drafts.</p>}
      {drafts.map(p => (
        <article key={p.id} className="post-card card">
          <h3><Link to={`/posts/${p.id}/edit`}>{p.title || '(untitled)'}</Link></h3>
          <span className="meta">{new Date(p.created_at).toLocaleDateString('ja-JP')}</span>
        </article>
      ))}
    </div>
  )
}
