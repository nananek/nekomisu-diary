import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../App'
import { api } from '../api'
import './Login.css'

export default function Login() {
  const { setUser } = useAuth()
  const nav = useNavigate()
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      if (mode === 'login') {
        await api.login(login, password)
      } else {
        await api.register(login, email, displayName, password)
      }
      const user = await api.me()
      setUser(user)
      nav('/')
    } catch (err: any) {
      setError(err.message)
    }
  }

  return (
    <div className="login-page">
      <div className="login-card card">
        <h1>Exchange Diary</h1>
        <form onSubmit={submit}>
          <input
            placeholder="Login ID"
            value={login}
            onChange={e => setLogin(e.target.value)}
            required
            autoFocus
          />
          {mode === 'register' && (
            <>
              <input placeholder="Email" type="email" value={email} onChange={e => setEmail(e.target.value)} required />
              <input placeholder="Display Name" value={displayName} onChange={e => setDisplayName(e.target.value)} required />
            </>
          )}
          <input
            placeholder="Password"
            type="password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            required
          />
          {error && <p className="error">{error}</p>}
          <button type="submit" className="primary" style={{ width: '100%' }}>
            {mode === 'login' ? 'Login' : 'Register'}
          </button>
        </form>
        <p className="switch-mode">
          {mode === 'login' ? (
            <>New here? <a href="#" onClick={e => { e.preventDefault(); setMode('register') }}>Register</a></>
          ) : (
            <>Have an account? <a href="#" onClick={e => { e.preventDefault(); setMode('login') }}>Login</a></>
          )}
        </p>
      </div>
    </div>
  )
}
