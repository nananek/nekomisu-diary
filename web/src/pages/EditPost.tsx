import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api } from '../api'
import ImageUploader from '../components/ImageUploader'
import { renderMarkdown } from '../lib/markdown'
import Icon from '../components/Icon'
import './NewPost.css'

export default function EditPost() {
  const { id } = useParams()
  const nav = useNavigate()
  const postId = Number(id)
  const [title, setTitle] = useState('')
  const [bodyMd, setBodyMd] = useState<string | null>(null) // null = no MD, edit HTML directly
  const [bodyHtml, setBodyHtml] = useState('')
  const [visibility, setVisibility] = useState('public')
  const [submitting, setSubmitting] = useState(false)
  const [loading, setLoading] = useState(true)
  const [preview, setPreview] = useState(false)

  useEffect(() => {
    api.post(postId).then(p => {
      setTitle(p.title)
      setBodyHtml(p.body_html)
      setBodyMd(p.body_md ?? null)
      setVisibility(p.visibility)
    }).catch(() => nav('/')).finally(() => setLoading(false))
  }, [postId, nav])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      if (bodyMd !== null) {
        // Markdown mode: regenerate HTML on save
        const html = renderMarkdown(bodyMd)
        await api.updatePost(postId, { title, body: html, body_md: bodyMd, visibility })
      } else {
        // Legacy HTML mode (pre-Markdown posts like migrated WP posts)
        await api.updatePost(postId, { title, body: bodyHtml, visibility })
      }
      nav(`/posts/${postId}`)
    } finally {
      setSubmitting(false)
    }
  }

  const convertToMarkdown = () => {
    if (!confirm('HTMLをプレーンテキストとして取り込みます。元のHTMLは失われます。続行しますか？')) return
    // Naive conversion: just use body_html as starting point
    setBodyMd(bodyHtml)
  }

  if (loading) return <div className="loading">読み込み中...</div>

  return (
    <div className="new-post">
      <h2><Icon name="edit" size={20} /> 日記を編集</h2>
      <form className="card post-form" onSubmit={submit}>
        <input value={title} onChange={e => setTitle(e.target.value)} required />

        {bodyMd !== null ? (
          <>
            <div className="editor-tabs">
              <button type="button" className={!preview ? 'active' : ''} onClick={() => setPreview(false)}>
                <Icon name="edit" size={14} />書く (Markdown)
              </button>
              <button type="button" className={preview ? 'active' : ''} onClick={() => setPreview(true)}>
                <Icon name="eye" size={14} />プレビュー
              </button>
            </div>
            {preview ? (
              <div className="post-body preview-box" dangerouslySetInnerHTML={{ __html: renderMarkdown(bodyMd) }} />
            ) : (
              <textarea value={bodyMd} onChange={e => setBodyMd(e.target.value)} rows={14} required />
            )}
            <ImageUploader onInsert={(url) => setBodyMd(b => (b ?? '') + `\n![画像](${url})`)} />
          </>
        ) : (
          <>
            <p className="meta"><Icon name="shield" size={12} /> この日記はHTML形式で保存されています（WordPressから移行済み）</p>
            <textarea value={bodyHtml} onChange={e => setBodyHtml(e.target.value)} rows={14} required />
            <div style={{ display: 'flex', gap: 8 }}>
              <ImageUploader onInsert={(url) => setBodyHtml(b => b + `\n<img src="${url}" />`)} />
              <button type="button" onClick={convertToMarkdown} className="ghost">
                Markdownに変換
              </button>
            </div>
          </>
        )}

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
