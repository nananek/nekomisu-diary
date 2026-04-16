import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../api'
import type { Post, Member } from '../api'
import Icon from '../components/Icon'

export default function MemberPosts() {
  const { userId } = useParams()
  const uid = Number(userId)
  const [posts, setPosts] = useState<Post[]>([])
  const [member, setMember] = useState<Member | null>(null)
  const [page, setPage] = useState(1)
  const [pages, setPages] = useState(1)
  const [loading, setLoading] = useState(true)

  useEffect(() => { api.members().then(d => { const m = d.members.find(m => m.id === uid); if (m) setMember(m) }) }, [uid])
  useEffect(() => { setLoading(true); api.userPosts(uid, page).then(d => { setPosts(d.posts); setPages(d.pages) }).finally(() => setLoading(false)) }, [uid, page])

  return (
    <div className="timeline">
      {member && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
          {member.avatar_path
            ? <img className="avatar" src={member.avatar_path} alt="" style={{ width: 48, height: 48 }} />
            : <span className="avatar placeholder" style={{ width: 48, height: 48, fontSize: 20 }}>{member.display_name[0]}</span>
          }
          <div>
            <h2 style={{ margin: 0 }}>{member.display_name}</h2>
            <span className="meta"><Icon name="draft" size={12} /> {member.post_count}件の日記</span>
          </div>
        </div>
      )}
      {loading ? <div className="loading">読み込み中...</div> : (
        <>
          {posts.length === 0 && <p className="empty">まだ日記がありません</p>}
          {posts.map(p => (
            <article key={p.id} className="post-card card">
              <div className="post-header">
                <span className="meta">
                  {p.published_at && new Date(p.published_at).toLocaleDateString('ja-JP')}
                  {p.visibility === 'private' && ' (自分のみ)'}
                </span>
              </div>
              <Link to={`/posts/${p.id}`} className="post-title-link"><h3>{p.title}</h3></Link>
              <div className="post-excerpt" dangerouslySetInnerHTML={{ __html: p.body_html.slice(0, 200) }} />
              <span className="meta footer-link">
                {p.comment_count > 0 && <><Icon name="comment" size={12} /> {p.comment_count}件</>}
              </span>
            </article>
          ))}
          {pages > 1 && (
            <div className="pagination">
              <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>前へ</button>
              <span className="meta">{page} / {pages}</span>
              <button disabled={page >= pages} onClick={() => setPage(p => p + 1)}>次へ</button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
