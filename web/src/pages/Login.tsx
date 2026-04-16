import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../App'
import { api } from '../api'
import type { LoginResult } from '../api'
import './Login.css'

export default function Login() {
  const { setUser } = useAuth()
  const nav = useNavigate()
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [mode, setMode] = useState<'login' | 'register' | '2fa'>('login')
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [twoFAInfo, setTwoFAInfo] = useState<{ has_totp?: boolean; has_webauthn?: boolean }>({})

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      if (mode === 'login') {
        const result: LoginResult = await api.login(login, password)
        if (result.requires_2fa) {
          setTwoFAInfo({ has_totp: result.has_totp, has_webauthn: result.has_webauthn })
          setMode('2fa')
          return
        }
      } else if (mode === 'register') {
        await api.register(login, email, displayName, password)
      }
      const user = await api.me()
      setUser(user)
      nav('/')
    } catch (err: any) {
      setError(err.message)
    }
  }

  const submitTOTP = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      await api.totpVerifyLogin(totpCode)
      const user = await api.me()
      setUser(user)
      nav('/')
    } catch (err: any) {
      setError(err.message)
    }
  }

  const loginWebAuthn = async () => {
    setError('')
    try {
      const options: any = await api.webauthnLoginBegin()
      const publicKey = {
        ...options.publicKey,
        challenge: base64urlToBuffer(options.publicKey.challenge),
        allowCredentials: options.publicKey.allowCredentials?.map((c: any) => ({
          ...c,
          id: base64urlToBuffer(c.id),
        })),
      }
      const assertion = await navigator.credentials.get({ publicKey }) as PublicKeyCredential
      const response = assertion.response as AuthenticatorAssertionResponse
      const body = {
        id: assertion.id,
        rawId: bufferToBase64url(assertion.rawId),
        type: assertion.type,
        response: {
          authenticatorData: bufferToBase64url(response.authenticatorData),
          clientDataJSON: bufferToBase64url(response.clientDataJSON),
          signature: bufferToBase64url(response.signature),
          userHandle: response.userHandle ? bufferToBase64url(response.userHandle) : null,
        },
      }
      const result = await api.webauthnLoginFinish(body)
      if (result.ok) {
        const user = await api.me()
        setUser(user)
        nav('/')
      }
    } catch (err: any) {
      setError(err.message || 'WebAuthn failed')
    }
  }

  if (mode === '2fa') {
    return (
      <div className="login-page">
        <div className="login-card card">
          <h1>Two-Factor Auth</h1>
          {twoFAInfo.has_totp && (
            <form onSubmit={submitTOTP}>
              <input
                placeholder="6-digit TOTP code"
                value={totpCode}
                onChange={e => setTotpCode(e.target.value)}
                autoFocus
                maxLength={6}
                pattern="[0-9]{6}"
              />
              {error && <p className="error">{error}</p>}
              <button type="submit" className="primary" style={{ width: '100%' }}>Verify</button>
            </form>
          )}
          {twoFAInfo.has_webauthn && (
            <div style={{ marginTop: 16 }}>
              <button onClick={loginWebAuthn} style={{ width: '100%' }}>
                Use Security Key
              </button>
            </div>
          )}
          <p className="switch-mode">
            <a href="#" onClick={e => { e.preventDefault(); setMode('login'); setError('') }}>Back to login</a>
          </p>
        </div>
      </div>
    )
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

// WebAuthn helpers
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
