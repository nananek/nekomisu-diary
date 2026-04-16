import { useState } from 'react'
import { useAuth } from '../App'
import { api } from '../api'
import './Settings.css'

export default function Settings() {
  const { user } = useAuth()
  const [oldPass, setOldPass] = useState('')
  const [newPass, setNewPass] = useState('')
  const [msg, setMsg] = useState('')
  const [error, setError] = useState('')

  const changePassword = async (e: React.FormEvent) => {
    e.preventDefault()
    setMsg('')
    setError('')
    try {
      await api.changePassword(oldPass, newPass)
      setMsg('Password updated')
      setOldPass('')
      setNewPass('')
    } catch (err: any) {
      setError(err.message)
    }
  }

  return (
    <div className="settings">
      <h2>Settings</h2>

      <section className="card settings-section">
        <h3>Profile</h3>
        <p><strong>{user?.display_name}</strong> ({user?.login})</p>
      </section>

      <section className="card settings-section">
        <h3>Change Password</h3>
        <form onSubmit={changePassword} className="pass-form">
          <input type="password" placeholder="Current password" value={oldPass} onChange={e => setOldPass(e.target.value)} required />
          <input type="password" placeholder="New password (min 8 chars)" value={newPass} onChange={e => setNewPass(e.target.value)} required minLength={8} />
          {error && <p className="error">{error}</p>}
          {msg && <p className="success">{msg}</p>}
          <button type="submit" className="primary">Update Password</button>
        </form>
      </section>

      <section className="card settings-section">
        <h3>Two-Factor Authentication</h3>
        <p className="meta">
          {user?.has_2fa ? 'Enabled' : 'Not configured'}
          {' '}&mdash; TOTP and WebAuthn setup coming soon.
        </p>
      </section>
    </div>
  )
}
