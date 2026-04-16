import { useState, useEffect, useRef } from 'react'
import { useAuth } from '../App'
import { api } from '../api'
import type { WebAuthnCredential } from '../api'
import './Settings.css'

export default function Settings() {
  const { user, setUser } = useAuth()

  return (
    <div className="settings">
      <h2>Settings</h2>
      <ProfileSection />
      <AvatarSection />
      <PasswordSection />
      <TOTPSection />
      <WebAuthnSection />
    </div>
  )

  function ProfileSection() {
    const [displayName, setDisplayName] = useState(user?.display_name ?? '')
    const [msg, setMsg] = useState('')

    const save = async (e: React.FormEvent) => {
      e.preventDefault()
      await api.updateProfile({ display_name: displayName })
      setMsg('Updated')
      const u = await api.me()
      setUser(u)
    }

    return (
      <section className="card settings-section">
        <h3>Profile</h3>
        <form onSubmit={save} className="settings-form">
          <label>Display Name</label>
          <input value={displayName} onChange={e => setDisplayName(e.target.value)} />
          <button type="submit" className="primary">Save</button>
          {msg && <p className="success">{msg}</p>}
        </form>
      </section>
    )
  }

  function AvatarSection() {
    const fileRef = useRef<HTMLInputElement>(null)
    const [uploading, setUploading] = useState(false)

    const upload = async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]
      if (!file) return
      setUploading(true)
      try {
        await api.uploadAvatar(file)
        const u = await api.me()
        setUser(u)
      } finally {
        setUploading(false)
      }
    }

    return (
      <section className="card settings-section">
        <h3>Avatar</h3>
        <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
          {user?.avatar_path
            ? <img className="avatar" src={user.avatar_path} alt="" style={{ width: 64, height: 64 }} />
            : <span className="avatar placeholder" style={{ width: 64, height: 64, fontSize: 28 }}>{user?.display_name[0]}</span>
          }
          <div>
            <input type="file" accept="image/*" ref={fileRef} onChange={upload} style={{ display: 'none' }} />
            <button onClick={() => fileRef.current?.click()} disabled={uploading}>
              {uploading ? 'Uploading...' : 'Change Avatar'}
            </button>
          </div>
        </div>
      </section>
    )
  }

  function PasswordSection() {
    const [oldPass, setOldPass] = useState('')
    const [newPass, setNewPass] = useState('')
    const [msg, setMsg] = useState('')
    const [error, setError] = useState('')

    const submit = async (e: React.FormEvent) => {
      e.preventDefault()
      setMsg(''); setError('')
      try {
        await api.changePassword(oldPass, newPass)
        setMsg('Password updated')
        setOldPass(''); setNewPass('')
      } catch (err: any) { setError(err.message) }
    }

    return (
      <section className="card settings-section">
        <h3>Password</h3>
        <form onSubmit={submit} className="settings-form">
          <input type="password" placeholder="Current password" value={oldPass} onChange={e => setOldPass(e.target.value)} required />
          <input type="password" placeholder="New password (min 8)" value={newPass} onChange={e => setNewPass(e.target.value)} required minLength={8} />
          {error && <p className="error">{error}</p>}
          {msg && <p className="success">{msg}</p>}
          <button type="submit" className="primary">Update</button>
        </form>
      </section>
    )
  }

  function TOTPSection() {
    const [setupUrl, setSetupUrl] = useState('')
    const [secret, setSecret] = useState('')
    const [code, setCode] = useState('')
    const [msg, setMsg] = useState('')
    const [error, setError] = useState('')

    const startSetup = async () => {
      setError(''); setMsg('')
      const { secret, url } = await api.totpSetup()
      setSecret(secret)
      setSetupUrl(url)
    }

    const confirm = async (e: React.FormEvent) => {
      e.preventDefault()
      setError('')
      try {
        await api.totpConfirm(code)
        setMsg('TOTP enabled!')
        setSetupUrl(''); setSecret(''); setCode('')
        const u = await api.me()
        setUser(u)
      } catch (err: any) { setError(err.message) }
    }

    const disable = async () => {
      await api.totpDisable()
      const u = await api.me()
      setUser(u)
    }

    return (
      <section className="card settings-section">
        <h3>TOTP (Authenticator App)</h3>
        {user?.has_totp ? (
          <div>
            <p className="success">Enabled</p>
            <button className="danger" onClick={disable}>Disable TOTP</button>
          </div>
        ) : setupUrl ? (
          <div>
            <p className="meta">Scan this with your authenticator app, or enter the secret manually:</p>
            <p style={{ wordBreak: 'break-all', fontFamily: 'var(--mono)', fontSize: 13, margin: '8px 0' }}>{secret}</p>
            <img src={`https://api.qrserver.com/v1/create-qr-code/?data=${encodeURIComponent(setupUrl)}&size=200x200`} alt="QR" style={{ margin: '8px 0' }} />
            <form onSubmit={confirm} className="settings-form">
              <input placeholder="Enter 6-digit code to confirm" value={code} onChange={e => setCode(e.target.value)} maxLength={6} />
              {error && <p className="error">{error}</p>}
              {msg && <p className="success">{msg}</p>}
              <button type="submit" className="primary">Confirm</button>
            </form>
          </div>
        ) : (
          <button onClick={startSetup}>Setup TOTP</button>
        )}
      </section>
    )
  }

  function WebAuthnSection() {
    const [creds, setCreds] = useState<WebAuthnCredential[]>([])
    const [error, setError] = useState('')
    const [msg, setMsg] = useState('')

    useEffect(() => {
      api.webauthnCredentials().then(d => setCreds(d.credentials))
    }, [])

    const register = async () => {
      setError(''); setMsg('')
      try {
        const options: any = await api.webauthnRegisterBegin()
        const publicKey = {
          ...options.publicKey,
          challenge: base64urlToBuffer(options.publicKey.challenge),
          user: {
            ...options.publicKey.user,
            id: base64urlToBuffer(options.publicKey.user.id),
          },
          excludeCredentials: options.publicKey.excludeCredentials?.map((c: any) => ({
            ...c,
            id: base64urlToBuffer(c.id),
          })),
        }
        const credential = await navigator.credentials.create({ publicKey }) as PublicKeyCredential
        const response = credential.response as AuthenticatorAttestationResponse
        const body = {
          id: credential.id,
          rawId: bufferToBase64url(credential.rawId),
          type: credential.type,
          response: {
            attestationObject: bufferToBase64url(response.attestationObject),
            clientDataJSON: bufferToBase64url(response.clientDataJSON),
          },
        }
        const result = await api.webauthnRegisterFinish(body)
        if (result.ok) {
          setMsg('Security key registered!')
          const d = await api.webauthnCredentials()
          setCreds(d.credentials)
          const u = await api.me()
          setUser(u)
        } else {
          setError(result.error || 'Registration failed')
        }
      } catch (err: any) {
        setError(err.message || 'WebAuthn failed')
      }
    }

    const remove = async (id: string) => {
      await api.webauthnDeleteCredential(id)
      const d = await api.webauthnCredentials()
      setCreds(d.credentials)
      const u = await api.me()
      setUser(u)
    }

    return (
      <section className="card settings-section">
        <h3>Security Keys (WebAuthn)</h3>
        {creds.length > 0 && (
          <ul className="cred-list">
            {creds.map(c => (
              <li key={c.id}>
                <span>{c.name} <span className="meta">({new Date(c.created_at).toLocaleDateString('ja-JP')})</span></span>
                <button className="danger" onClick={() => remove(c.id)}>Remove</button>
              </li>
            ))}
          </ul>
        )}
        {error && <p className="error">{error}</p>}
        {msg && <p className="success">{msg}</p>}
        <button onClick={register}>Add Security Key</button>
      </section>
    )
  }
}

function base64urlToBuffer(base64url: string): ArrayBuffer {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/')
  const pad = base64.length % 4 === 0 ? '' : '='.repeat(4 - (base64.length % 4))
  const binary = atob(base64 + pad)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}

function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (const b of bytes) binary += String.fromCharCode(b)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}
