import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Member } from '../api'

export default function Members() {
  const [members, setMembers] = useState<Member[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.members().then(d => setMembers(d.members)).finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="loading">Loading...</div>

  return (
    <div className="timeline">
      <h2>Members</h2>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {members.map(m => (
          <Link key={m.id} to={`/members/${m.id}`} className="card" style={{ textDecoration: 'none', display: 'flex', alignItems: 'center', gap: 16 }}>
            {m.avatar_path
              ? <img className="avatar" src={m.avatar_path} alt="" style={{ width: 48, height: 48 }} />
              : <span className="avatar placeholder" style={{ width: 48, height: 48, fontSize: 20 }}>{m.display_name[0]}</span>
            }
            <div>
              <div style={{ fontWeight: 600, color: 'var(--text-h)' }}>{m.display_name}</div>
              <div className="meta">@{m.login} &middot; {m.post_count} posts &middot; {m.comment_count} comments</div>
              <div className="meta">Joined {new Date(m.created_at).toLocaleDateString('ja-JP')}</div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}
