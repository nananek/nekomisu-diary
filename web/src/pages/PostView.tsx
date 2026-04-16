import { useState, useEffect } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { api } from '../api'
import type { Post, Comment } from '../api'
import { useAuth } from '../auth'
import Icon from '../components/Icon'
import './PostView.css'

export default function PostView() {
  const { id } = useParams()
  const { user } = useAuth()
  const nav = useNavigate()
  const [post, setPost] = useState<Post | null>(null)
  const [comments, setComments] = useState<Comment[]>([])
  const [newComment, setNewComment] = useState('')
  const [replyTo, setReplyTo] = useState<number | null>(null)
  const postId = Number(id)

  useEffect(() => {
    api.post(postId).then(setPost).catch(() => nav('/'))
    api.comments(postId).then(d => setComments(d.comments))
  }, [postId, nav])

  const submitComment = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newComment.trim()) return
    await api.createComment(postId, newComment, replyTo ?? undefined)
    setNewComment('')
    setReplyTo(null)
    const d = await api.comments(postId)
    setComments(d.comments)
  }

  const deleteComment = async (commentId: number) => {
    await api.deleteComment(commentId)
    const d = await api.comments(postId)
    setComments(d.comments)
  }

  const deletePost = async () => {
    if (!confirm('この日記を削除しますか？')) return
    await api.deletePost(postId)
    nav('/')
  }

  if (!post) return <div className="loading">読み込み中...</div>

  const topLevel = comments.filter(c => !c.parent_id)
  const replies = (parentId: number) => comments.filter(c => c.parent_id === parentId)

  return (
    <div className="post-view">
      <article className="card">
        <div className="pv-header">
          <Link to={`/members/${post.author_id}`} className="post-author">
            {post.author_avatar
              ? <img className="avatar" src={post.author_avatar} alt="" />
              : <span className="avatar placeholder">{post.author_name[0]}</span>
            }
            <span className="author-name">{post.author_name}</span>
            <span className="meta">
              {post.published_at && new Date(post.published_at).toLocaleDateString('ja-JP')}
            </span>
          </Link>
          {user?.id === post.author_id && (
            <div style={{ display: 'flex', gap: 4 }}>
              <Link to={`/posts/${post.id}/edit`} className="btn"><Icon name="edit" size={16} />編集</Link>
              <button className="danger" onClick={deletePost}><Icon name="trash" size={16} /></button>
            </div>
          )}
        </div>
        <h1>{post.title}</h1>
        <div className="post-body" dangerouslySetInnerHTML={{ __html: post.body_html }} />
      </article>

      <section className="comments-section">
        <h3><Icon name="comment" size={18} /> コメント ({comments.length})</h3>
        {topLevel.map(c => (
          <div key={c.id} className="comment-thread">
            <CommentItem c={c} onReply={setReplyTo} onDelete={deleteComment} activeReply={replyTo} currentUserId={user?.id ?? 0} />
            {replies(c.id).map(r => (
              <div key={r.id} className="comment-reply">
                <CommentItem c={r} onReply={setReplyTo} onDelete={deleteComment} activeReply={replyTo} currentUserId={user?.id ?? 0} />
              </div>
            ))}
          </div>
        ))}
        <form className="comment-form" onSubmit={submitComment}>
          {replyTo && (
            <p className="meta">
              返信中 <a href="#" onClick={e => { e.preventDefault(); setReplyTo(null) }}>キャンセル</a>
            </p>
          )}
          <textarea placeholder="コメントを書く..." value={newComment} onChange={e => setNewComment(e.target.value)} rows={3} />
          <button type="submit" className="primary"><Icon name="send" size={16} />送信</button>
        </form>
      </section>
    </div>
  )
}

function CommentItem({ c, onReply, onDelete, activeReply, currentUserId }: {
  c: Comment; onReply: (id: number) => void; onDelete: (id: number) => void; activeReply: number | null; currentUserId: number
}) {
  return (
    <div className={`comment card ${activeReply === c.id ? 'replying' : ''}`}>
      <div className="comment-header">
        <div className="post-author">
          {c.author_avatar
            ? <img className="avatar" src={c.author_avatar} alt="" style={{ width: 28, height: 28 }} />
            : <span className="avatar placeholder" style={{ width: 28, height: 28, fontSize: 13 }}>{c.author_name[0]}</span>
          }
          <span className="author-name">{c.author_name}</span>
          <span className="meta">{new Date(c.created_at).toLocaleDateString('ja-JP')}</span>
        </div>
        <div style={{ display: 'flex', gap: 2 }}>
          <button className="ghost" onClick={() => onReply(c.id)} title="返信"><Icon name="reply" size={14} /></button>
          {c.author_id === currentUserId && (
            <button className="ghost" onClick={() => onDelete(c.id)} title="削除" style={{ color: 'var(--danger)' }}><Icon name="trash" size={14} /></button>
          )}
        </div>
      </div>
      <p>{c.body}</p>
    </div>
  )
}
