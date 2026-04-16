import { Link, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../App'
import { api } from '../api'
import './Layout.css'

export default function Layout() {
  const { user, setUser } = useAuth()
  const nav = useNavigate()

  const logout = async () => {
    await api.logout()
    setUser(null)
    nav('/login')
  }

  return (
    <div className="layout">
      <header className="topbar">
        <Link to="/" className="site-title">Exchange Diary</Link>
        <nav className="topnav">
          <Link to="/new" className="btn primary">New</Link>
          <Link to="/search">Search</Link>
          <Link to="/members">Members</Link>
          <Link to="/drafts">Drafts</Link>
          <Link to="/settings">{user?.display_name}</Link>
          <button onClick={logout}>Logout</button>
        </nav>
      </header>
      <main className="main-content">
        <Outlet />
      </main>
    </div>
  )
}
