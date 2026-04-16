import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api } from '../api'
import ImageUploader from '../components/ImageUploader'
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

  if (loading) return <div className="loading">Loading...</div>

  return (
    <div className="new-post">
      <h2>Edit Post</h2>
      <form className="card post-form" onSubmit={submit}>
        <input value={title} onChange={e => setTitle(e.target.value)} required />
        <textarea value={body} onChange={e => setBody(e.target.value)} rows={10} required />
        <ImageUploader onInsert={(url) => setBody(b => b + `\n<img src="${url}" />`)} />
        <div className="form-footer">
          <select value={visibility} onChange={e => setVisibility(e.target.value)}>
            <option value="public">Public</option>
            <option value="private">Private</option>
            <option value="draft">Draft</option>
          </select>
          <button type="submit" className="primary" disabled={submitting}>
            {submitting ? 'Saving...' : 'Save'}
          </button>
        </div>
      </form>
    </div>
  )
}

