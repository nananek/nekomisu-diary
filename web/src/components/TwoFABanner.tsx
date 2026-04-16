import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../App'
import Icon from './Icon'
import './TwoFABanner.css'

const DISMISSED_KEY = 'twofa-banner-dismissed-until'
const SNOOZE_DAYS = 7

export default function TwoFABanner() {
  const { user } = useAuth()
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (!user) return
    if (user.has_2fa) { setVisible(false); return }
    const until = localStorage.getItem(DISMISSED_KEY)
    if (until && Date.now() < Number(until)) { setVisible(false); return }
    setVisible(true)
  }, [user])

  const dismiss = () => {
    localStorage.setItem(DISMISSED_KEY, String(Date.now() + SNOOZE_DAYS * 86400 * 1000))
    setVisible(false)
  }

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
