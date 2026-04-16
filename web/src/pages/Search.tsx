import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Post } from '../api'

export default function Search() {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Post[]>([])
  const [searched, setSearched] = useState(false)
  const [loading, setLoading] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!query.trim()) return
    setLoading(true)
    try {
      const d = await api.search(query)
      setResults(d.posts)
      setSearched(true)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="timeline">
      <h2>Search</h2>
      <form onSubmit={submit} style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        <input
          placeholder="Search posts..."
          value={query}
          onChange={e => setQuery(e.target.value)}
          style={{ flex: 1 }}
          autoFocus
        />
        <button type="submit" className="primary" disabled={loading}>Search</button>
      </form>
      {loading && <div className="loading">Searching...</div>}
      {searched && !loading && results.length === 0 && <p>No results.</p>}
      {results.map(p => (
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
              {p.published_at && new Date(p.published_at).toLocaleDateString('ja-JP')}
            </span>
          </div>
          <Link to={`/posts/${p.id}`} className="post-title-link"><h3>{p.title}</h3></Link>
        </article>
      ))}
    </div>
  )
}
