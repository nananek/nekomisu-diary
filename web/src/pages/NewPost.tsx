import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import ImageUploader from '../components/ImageUploader'
import { renderMarkdown } from '../lib/markdown'
import Icon from '../components/Icon'
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
      const { id } = await api.createPost(title, html, visibility, body)
      nav(`/posts/${id}`)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="new-post">
      <h2><Icon name="plus" size={20} /> 新しい日記</h2>
      <form className="card post-form" onSubmit={submit}>
        <input placeholder="タイトル" value={title} onChange={e => setTitle(e.target.value)} required autoFocus />
        <div className="editor-tabs">
          <button type="button" className={!preview ? 'active' : ''} onClick={() => setPreview(false)}>
            <Icon name="edit" size={14} />書く
          </button>
          <button type="button" className={preview ? 'active' : ''} onClick={() => setPreview(true)}>
            <Icon name="eye" size={14} />プレビュー
          </button>
        </div>
        {preview ? (
          <div className="post-body preview-box" dangerouslySetInnerHTML={{ __html: renderMarkdown(body) }} />
        ) : (
          <textarea placeholder="Markdownで日記を書こう..." value={body} onChange={e => setBody(e.target.value)} rows={10} required />
        )}
        <ImageUploader onInsert={(url) => setBody(b => b + `\n![画像](${url})`)} />
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
            <Icon name="send" size={16} />{submitting ? '投稿中...' : '投稿する'}
          </button>
        </div>
      </form>
    </div>
  )
}
