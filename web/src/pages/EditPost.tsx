import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api } from '../api'
import ImageUploader from '../components/ImageUploader'
import Icon from '../components/Icon'
import './NewPost.css'

export default function EditPost() {
  const { id } = useParams()
  const nav = useNavigate()
  const postId = Number(id)
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [visibility, setVisibility] = useState('public')
  const [submitting, setSubmitting] = useState(false)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.post(postId).then(p => {
      setTitle(p.title)
      setBody(p.body_html)
      setVisibility(p.visibility)
    }).catch(() => nav('/')).finally(() => setLoading(false))
  }, [postId, nav])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      await api.updatePost(postId, { title, body, visibility })
      nav(`/posts/${postId}`)
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) return <div className="loading">読み込み中...</div>

  return (
    <div className="new-post">
      <h2><Icon name="edit" size={20} /> 日記を編集</h2>
      <form className="card post-form" onSubmit={submit}>
        <input value={title} onChange={e => setTitle(e.target.value)} required />
        <textarea value={body} onChange={e => setBody(e.target.value)} rows={10} required />
        <ImageUploader onInsert={(url) => setBody(b => b + `\n<img src="${url}" />`)} />
        <div className="form-footer">
          <div className="visibility-select">
            <Icon name={visibility === 'public' ? 'eye' : visibility === 'private' ? 'eye-off' : 'draft'} size={16} />
            <select value={visibility} onChange={e => setVisibility(e.target.value)}>
              <option value="public">全員に公開</option>
              <option value="private">自分のみ</option>
              <option value="draft">下書き</option>
            </select>
          </div>
          <button type="submit" className="primary" disabled={submitting}>
            <Icon name="check" size={16} />{submitting ? '保存中...' : '保存する'}
          </button>
        </div>
      </form>
    </div>
  )
}
