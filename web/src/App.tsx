import { useState, useEffect, createContext, useContext } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { api } from './api'
import type { User } from './api'
import Login from './pages/Login'
import Timeline from './pages/Timeline'
import PostView from './pages/PostView'
import NewPost from './pages/NewPost'
import Settings from './pages/Settings'
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
          <Route path="/new" element={<NewPost />} />
          <Route path="/settings" element={<Settings />} />
        </Route>
      </Routes>
    </AuthContext.Provider>
  )
}

export default App
