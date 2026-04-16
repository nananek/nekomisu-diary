import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Member } from '../api'
import Icon from '../components/Icon'

export default function Members() {
  const [members, setMembers] = useState<Member[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => { api.members().then(d => setMembers(d.members)).finally(() => setLoading(false)) }, [])

  if (loading) return <div className="loading">読み込み中...</div>

  return (
    <div className="timeline">
      <h2><Icon name="users" size={20} /> メンバー</h2>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        {members.map(m => (
          <Link key={m.id} to={`/members/${m.id}`} className="card member-card">
            {m.avatar_path
              ? <img className="avatar" src={m.avatar_path} alt="" style={{ width: 48, height: 48 }} />
              : <span className="avatar placeholder" style={{ width: 48, height: 48, fontSize: 20 }}>{m.display_name[0]}</span>
            }
            <div>
              <div style={{ fontWeight: 600, color: 'var(--text-h)' }}>{m.display_name}</div>
              <div className="meta">
                <Icon name="draft" size={12} /> {m.post_count}件 ・ <Icon name="comment" size={12} /> {m.comment_count}件
              </div>
              <div className="meta">
                {new Date(m.created_at).toLocaleDateString('ja-JP')} から参加
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}
