import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../auth'
import Icon from './Icon'
import './TwoFABanner.css'

const DISMISSED_KEY = 'twofa-banner-dismissed-until'
const SNOOZE_DAYS = 7

function loadDismissed(): boolean {
  const until = localStorage.getItem(DISMISSED_KEY)
  return !!(until && Date.now() < Number(until))
}

export default function TwoFABanner() {
  const { user } = useAuth()
  const [dismissed, setDismissed] = useState<boolean>(loadDismissed)

  const dismiss = () => {
    localStorage.setItem(DISMISSED_KEY, String(Date.now() + SNOOZE_DAYS * 86400 * 1000))
    setDismissed(true)
  }

  const visible = !!user && !user.has_2fa && !dismissed
  if (!visible) return null

  return (
    <div className="twofa-banner">
      <div className="twofa-content">
        <Icon name="shield" size={18} />
        <span>二段階認証を有効にするとアカウントを安全に守れます</span>
      </div>
      <div className="twofa-actions">
        <Link to="/settings" className="btn primary">設定する</Link>
        <button className="ghost" onClick={dismiss} aria-label="閉じる">
          <Icon name="check" size={14} />後で
        </button>
      </div>
    </div>
  )
}
