import { useState } from 'react'
import { Link, Outlet, useNavigate, useLocation } from 'react-router-dom'
import { useAuth } from '../App'
import { api } from '../api'
import { usePWAInstall } from '../hooks/usePWAInstall'
import { useTheme } from '../hooks/useTheme'
import Icon from './Icon'
import './Layout.css'

export default function Layout() {
  const { user, setUser } = useAuth()
  const nav = useNavigate()
  const loc = useLocation()
  const { canInstall, install } = usePWAInstall()
  const { pref, cycle } = useTheme()
  const [menuOpen, setMenuOpen] = useState(false)

  const logout = async () => {
    await api.logout()
    setUser(null)
    nav('/login')
  }

  const close = () => setMenuOpen(false)
  const isActive = (path: string) => loc.pathname === path ? 'active' : ''

  const themeIcon = pref === 'auto' ? 'auto' : pref === 'dark' ? 'moon' : 'sun'
  const themeLabel = pref === 'auto' ? '自動' : pref === 'dark' ? 'ダーク' : 'ライト'

  return (
    <div className="layout">
      <header className="topbar">
        <Link to="/" className="site-title" onClick={close}>交換日記</Link>
        <div className="topbar-actions">
          <button className="ghost" onClick={cycle} title={`テーマ: ${themeLabel}`}>
            <Icon name={themeIcon} size={18} />
          </button>
          <button className="hamburger" onClick={() => setMenuOpen(!menuOpen)} aria-label="メニュー">
            <Icon name="menu" size={22} />
          </button>
        </div>
        <nav className={`topnav ${menuOpen ? 'open' : ''}`}>
          <Link to="/" className={`nav-item ${isActive('/')}`} onClick={close}>
            <Icon name="home" size={18} />タイムライン
          </Link>
          <Link to="/new" className={`nav-item ${isActive('/new')}`} onClick={close}>
            <Icon name="plus" size={18} />新しい日記
          </Link>
          <Link to="/search" className={`nav-item ${isActive('/search')}`} onClick={close}>
            <Icon name="search" size={18} />検索
          </Link>
          <Link to="/members" className={`nav-item ${isActive('/members')}`} onClick={close}>
            <Icon name="users" size={18} />メンバー
          </Link>
          <Link to="/drafts" className={`nav-item ${isActive('/drafts')}`} onClick={close}>
            <Icon name="draft" size={18} />下書き
          </Link>
          <Link to="/settings" className={`nav-item ${isActive('/settings')}`} onClick={close}>
            <Icon name="settings" size={18} />{user?.display_name}
          </Link>
          {canInstall && (
            <button className="nav-item" onClick={() => { install(); close() }}>
              <Icon name="download" size={18} />アプリを追加
            </button>
          )}
          <button className="nav-item nav-logout" onClick={logout}>
            <Icon name="logout" size={18} />ログアウト
          </button>
        </nav>
      </header>

      {/* Mobile bottom nav */}
      <nav className="bottom-nav">
        <Link to="/" className={isActive('/')}><Icon name="home" size={22} /><span>ホーム</span></Link>
        <Link to="/search" className={isActive('/search')}><Icon name="search" size={22} /><span>検索</span></Link>
        <Link to="/new" className="new-btn"><Icon name="plus" size={24} /></Link>
        <Link to="/members" className={isActive('/members')}><Icon name="users" size={22} /><span>メンバー</span></Link>
        <Link to="/settings" className={isActive('/settings')}><Icon name="settings" size={22} /><span>設定</span></Link>
      </nav>

      <main className="main-content">
        <Outlet />
      </main>
    </div>
  )
}
