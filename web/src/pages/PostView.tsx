import { useState, useEffect } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { api } from '../api'
import type { Post, Comment } from '../api'
import { useAuth } from '../App'
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
    if (!confirm('Delete this post?')) return
    await api.deletePost(postId)
    nav('/')
  }

  if (!post) return <div className="loading">Loading...</div>

  const topLevel = comments.filter(c => !c.parent_id)
  const replies = (parentId: number) => comments.filter(c => c.parent_id === parentId)

  return (
    <div className="post-view">
      <article className="card">
        <div className="pv-header">
          <div className="post-author">
            {post.author_avatar
              ? <img className="avatar" src={post.author_avatar} alt="" />
              : <span className="avatar placeholder">{post.author_name[0]}</span>
            }
            <span className="author-name">{post.author_name}</span>
            <span className="meta">
              {post.published_at && new Date(post.published_at).toLocaleDateString('ja-JP')}
            </span>
          </div>
          {user?.id === post.author_id && (
            <div style={{ display: 'flex', gap: 8 }}>
              <Link to={`/posts/${post.id}/edit`} className="btn">Edit</Link>
              <button className="danger" onClick={deletePost}>Delete</button>
            </div>
          )}
        </div>
        <h1>{post.title}</h1>
        <div className="post-body" dangerouslySetInnerHTML={{ __html: post.body_html }} />
      </article>

      <section className="comments-section">
        <h3>Comments ({comments.length})</h3>
        {topLevel.map(c => (
          <div key={c.id} className="comment-thread">
            <CommentItem c={c} onReply={setReplyTo} onDelete={deleteComment} activeReply={replyTo} currentUserId={user?.id ?? 0} />
            {replies(c.id).map(r => (
              <div key={r.id} className="comment-reply">
                <CommentItem c={r} onReply={setReplyTo} activeReply={replyTo} />
              </div>
            ))}
          </div>
        ))}
        <form className="comment-form" onSubmit={submitComment}>
          {replyTo && (
            <p className="meta">
              Replying to #{replyTo}{' '}
              <a href="#" onClick={e => { e.preventDefault(); setReplyTo(null) }}>cancel</a>
            </p>
          )}
          <textarea
            placeholder="Write a comment..."
            value={newComment}
            onChange={e => setNewComment(e.target.value)}
            rows={3}
          />
          <button type="submit" className="primary">Comment</button>
        </form>
      </section>
    </div>
  )
}

function CommentItem({
  c,
  onReply,
  onDelete,
  activeReply,
  currentUserId,
}: {
  c: Comment
  onReply: (id: number) => void
  onDelete: (id: number) => void
  activeReply: number | null
  currentUserId: number
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
        <div style={{ display: 'flex', gap: 4 }}>
          <button className="reply-btn" onClick={() => onReply(c.id)}>Reply</button>
          {c.author_id === currentUserId && (
            <button className="reply-btn danger" onClick={() => onDelete(c.id)}>Del</button>
          )}
        </div>
      </div>
      <p>{c.body}</p>
    </div>
  )
}
