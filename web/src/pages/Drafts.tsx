import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Post } from '../api'
import Icon from '../components/Icon'

export default function Drafts() {
  const [drafts, setDrafts] = useState<Post[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => { api.drafts().then(d => setDrafts(d.posts)).finally(() => setLoading(false)) }, [])

  if (loading) return <div className="loading">読み込み中...</div>

  return (
    <div className="timeline">
      <h2><Icon name="draft" size={20} /> 下書き</h2>
      {drafts.length === 0 && <p className="empty">下書きはありません</p>}
      {drafts.map(p => (
        <article key={p.id} className="post-card card">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Link to={`/posts/${p.id}/edit`} className="post-title-link" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <Icon name="edit" size={16} /><h3>{p.title || '(無題)'}</h3>
            </Link>
            <span className="meta">{new Date(p.created_at).toLocaleDateString('ja-JP')}</span>
          </div>
        </article>
      ))}
    </div>
  )
}
