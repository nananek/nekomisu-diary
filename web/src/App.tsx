import { useState, useEffect, createContext, useContext } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { api } from './api'
import type { User } from './api'
import Login from './pages/Login'
import Timeline from './pages/Timeline'
import PostView from './pages/PostView'
import NewPost from './pages/NewPost'
import Settings from './pages/Settings'
import EditPost from './pages/EditPost'
import Drafts from './pages/Drafts'
import Search from './pages/Search'
import Members from './pages/Members'
import MemberPosts from './pages/MemberPosts'
import Gallery from './pages/Gallery'
import Layout from './components/Layout'

interface AuthCtx {
  user: User | null
  setUser: (u: User | null) => void
  loading: boolean
}

export const AuthContext = createContext<AuthCtx>({
  user: null,
  setUser: () => {},
  loading: true,
})

export function useAuth() {
  return useContext(AuthContext)
}

function App() {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.me().then(setUser).catch(() => setUser(null)).finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="loading">Loading...</div>

  return (
    <AuthContext.Provider value={{ user, setUser, loading }}>
      <Routes>
        <Route path="/login" element={user ? <Navigate to="/" /> : <Login />} />
        <Route element={user ? <Layout /> : <Navigate to="/login" />}>
          <Route path="/" element={<Timeline />} />
          <Route path="/posts/:id" element={<PostView />} />
          <Route path="/posts/:id/edit" element={<EditPost />} />
          <Route path="/new" element={<NewPost />} />
          <Route path="/drafts" element={<Drafts />} />
          <Route path="/search" element={<Search />} />
          <Route path="/members" element={<Members />} />
          <Route path="/members/:userId" element={<MemberPosts />} />
          <Route path="/media" element={<Gallery />} />
          <Route path="/settings" element={<Settings />} />
        </Route>
      </Routes>
    </AuthContext.Provider>
  )
}

export default App
