import { useState, useEffect } from 'react'
import { api } from '../api'
import type { MediaItem } from '../api'
import Icon from '../components/Icon'
import './Gallery.css'

export default function Gallery() {
  const [items, setItems] = useState<MediaItem[]>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<MediaItem | null>(null)
  const [copied, setCopied] = useState<number | null>(null)

  useEffect(() => {
    api.listMedia().then(d => setItems(d.items)).finally(() => setLoading(false))
  }, [])

  const del = async (item: MediaItem) => {
    if (!confirm(`${item.filename} を削除しますか？`)) return
    await api.deleteMedia(item.id)
    setItems(xs => xs.filter(x => x.id !== item.id))
    setSelected(null)
  }

  const copyUrl = async (item: MediaItem) => {
    await navigator.clipboard.writeText(item.url)
    setCopied(item.id)
    setTimeout(() => setCopied(null), 1500)
  }

  const copyMd = async (item: MediaItem) => {
    const md = `![${item.filename}](${item.url})`
    await navigator.clipboard.writeText(md)
    setCopied(item.id)
    setTimeout(() => setCopied(null), 1500)
  }

  if (loading) return <div className="loading">読み込み中...</div>

  return (
    <div className="timeline">
      <h2><Icon name="image" size={20} /> メディア ({items.length})</h2>
      {items.length === 0 && <p className="empty">アップロードした画像はありません</p>}
      <div className="gallery-grid">
        {items.map(item => (
          <div key={item.id} className="gallery-item" onClick={() => setSelected(item)}>
            <img src={item.thumbnail_url || item.url} alt={item.filename} loading="lazy" />
          </div>
        ))}
      </div>

      {selected && (
        <div className="gallery-modal" onClick={() => setSelected(null)}>
          <div className="gallery-modal-content card" onClick={e => e.stopPropagation()}>
            <img src={selected.url} alt={selected.filename} />
            <div className="gallery-info">
              <p><strong>{selected.filename}</strong></p>
              <p className="meta">
                {selected.width && selected.height ? `${selected.width}×${selected.height}` : ''}
                {selected.byte_size ? ` ・ ${(selected.byte_size / 1024).toFixed(1)} KB` : ''}
                {' ・ '}{new Date(selected.created_at).toLocaleDateString('ja-JP')}
              </p>
              <div className="gallery-actions">
                <button onClick={() => copyMd(selected)}>
                  <Icon name="check" size={14} />{copied === selected.id ? 'コピー済み' : 'Markdownをコピー'}
                </button>
                <button onClick={() => copyUrl(selected)}>URLをコピー</button>
                <button className="danger" onClick={() => del(selected)}>
                  <Icon name="trash" size={14} />削除
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
