import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import ImageUploader from '../components/ImageUploader'
import { renderMarkdown } from '../lib/markdown'
import './NewPost.css'

export default function NewPost() {
  const nav = useNavigate()
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [visibility, setVisibility] = useState('public')
  const [submitting, setSubmitting] = useState(false)
  const [preview, setPreview] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!title.trim() || !body.trim()) return
    setSubmitting(true)
    try {
      const html = renderMarkdown(body)
      const { id } = await api.createPost(title, html, visibility)
      nav(`/posts/${id}`)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="new-post">
      <h2>New Post</h2>
      <form className="card post-form" onSubmit={submit}>
        <input
          placeholder="Title"
          value={title}
          onChange={e => setTitle(e.target.value)}
          required
          autoFocus
        />
        <div className="editor-tabs">
          <button type="button" className={!preview ? 'active' : ''} onClick={() => setPreview(false)}>Write</button>
          <button type="button" className={preview ? 'active' : ''} onClick={() => setPreview(true)}>Preview</button>
        </div>
        {preview ? (
          <div className="post-body preview-box" dangerouslySetInnerHTML={{ __html: renderMarkdown(body) }} />
        ) : (
          <textarea
            placeholder="Write in Markdown..."
            value={body}
            onChange={e => setBody(e.target.value)}
            rows={10}
            required
          />
        )}
        <ImageUploader onInsert={(url) => setBody(b => b + `\n![image](${url})`)} />
        <div className="form-footer">
          <select value={visibility} onChange={e => setVisibility(e.target.value)}>
            <option value="public">Public</option>
            <option value="private">Private</option>
            <option value="draft">Draft</option>
          </select>
          <button type="submit" className="primary" disabled={submitting}>
            {submitting ? 'Posting...' : 'Post'}
          </button>
        </div>
      </form>
    </div>
  )
}
