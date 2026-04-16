import { useState } from 'react'
import { api } from '../api'
import { errMessage } from '../lib/webauthn'
import Icon from './Icon'

export default function ImageUploader({ onInsert }: { onInsert: (url: string) => void }) {
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')

  const upload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setUploading(true)
    setError('')
    try {
      const result = await api.uploadMedia(file)
      onInsert(result.url)
    } catch (err) {
      setError(errMessage(err, 'アップロードに失敗しました'))
    } finally {
      setUploading(false)
      e.target.value = ''
    }
  }

  return (
    <>
      <label style={{ cursor: 'pointer' }}>
        <input type="file" accept="image/*" onChange={upload} style={{ display: 'none' }} />
        <span className="btn"><Icon name="image" size={16} />{uploading ? 'アップロード中...' : '画像を挿入'}</span>
      </label>
      {error && <p className="error">{error}</p>}
    </>
  )
}
