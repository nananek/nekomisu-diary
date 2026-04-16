import { useState } from 'react'
import { Link, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../App'
import { api } from '../api'
import { usePWAInstall } from '../hooks/usePWAInstall'
import './Layout.css'

export default function Layout() {
  const { user, setUser } = useAuth()
  const nav = useNavigate()
  const { canInstall, install } = usePWAInstall()
  const [menuOpen, setMenuOpen] = useState(false)

  const logout = async () => {
    await api.logout()
    setUser(null)
    nav('/login')
  }

  return (
    <div className="layout">
      <header className="topbar">
        <Link to="/" className="site-title">Exchange Diary</Link>
        <button className="hamburger" onClick={() => setMenuOpen(!menuOpen)} aria-label="Menu">
          <span /><span /><span />
        </button>
        <nav className={`topnav ${menuOpen ? 'open' : ''}`}>
          <Link to="/new" className="btn primary" onClick={() => setMenuOpen(false)}>New</Link>
          <Link to="/search" onClick={() => setMenuOpen(false)}>Search</Link>
          <Link to="/members" onClick={() => setMenuOpen(false)}>Members</Link>
          <Link to="/drafts" onClick={() => setMenuOpen(false)}>Drafts</Link>
          <Link to="/settings" onClick={() => setMenuOpen(false)}>{user?.display_name}</Link>
          {canInstall && <button onClick={install}>Install App</button>}
          <button onClick={logout}>Logout</button>
        </nav>
      </header>
      <main className="main-content">
        <Outlet />
      </main>
    </div>
  )
}
