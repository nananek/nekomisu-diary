import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../App'
import { api } from '../api'
import type { LoginResult } from '../api'
import Icon from '../components/Icon'
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
          ...c, id: base64urlToBuffer(c.id),
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
      setError(err.message || '認証に失敗しました')
    }
  }

  if (mode === '2fa') {
    return (
      <div className="login-page">
        <div className="login-card card">
          <h1><Icon name="shield" size={28} /> 二段階認証</h1>
          {twoFAInfo.has_totp && (
            <form onSubmit={submitTOTP}>
              <input placeholder="6桁の認証コード" value={totpCode} onChange={e => setTotpCode(e.target.value)} autoFocus maxLength={6} pattern="[0-9]{6}" inputMode="numeric" />
              {error && <p className="error">{error}</p>}
              <button type="submit" className="primary" style={{ width: '100%' }}><Icon name="check" size={18} />確認</button>
            </form>
          )}
          {twoFAInfo.has_webauthn && (
            <button onClick={loginWebAuthn} style={{ width: '100%', marginTop: 12 }}>
              <Icon name="key" size={18} />セキュリティキーを使う
            </button>
          )}
          <p className="switch-mode">
            <a href="#" onClick={e => { e.preventDefault(); setMode('login'); setError('') }}>ログインに戻る</a>
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="login-page">
      <div className="login-card card">
        <h1>交換日記</h1>
        <form onSubmit={submit}>
          <input placeholder="ログインID" value={login} onChange={e => setLogin(e.target.value)} required autoFocus />
          {mode === 'register' && (
            <>
              <input placeholder="メールアドレス" type="email" value={email} onChange={e => setEmail(e.target.value)} required />
              <input placeholder="表示名" value={displayName} onChange={e => setDisplayName(e.target.value)} required />
            </>
          )}
          <input placeholder="パスワード" type="password" value={password} onChange={e => setPassword(e.target.value)} required />
          {error && <p className="error">{error}</p>}
          <button type="submit" className="primary" style={{ width: '100%' }}>
            {mode === 'login' ? <><Icon name="lock" size={18} />ログイン</> : <><Icon name="user" size={18} />登録</>}
          </button>
        </form>
        <p className="switch-mode">
          {mode === 'login'
            ? <>はじめての方は<a href="#" onClick={e => { e.preventDefault(); setMode('register') }}>新規登録</a></>
            : <>アカウントをお持ちの方は<a href="#" onClick={e => { e.preventDefault(); setMode('login') }}>ログイン</a></>
          }
        </p>
      </div>
    </div>
  )
}

function base64urlToBuffer(b: string): ArrayBuffer {
  const s = b.replace(/-/g, '+').replace(/_/g, '/');
  const pad = s.length % 4 === 0 ? '' : '='.repeat(4 - (s.length % 4));
  const bin = atob(s + pad); const a = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) a[i] = bin.charCodeAt(i);
  return a.buffer;
}
function bufferToBase64url(buf: ArrayBuffer): string {
  const a = new Uint8Array(buf); let s = '';
  for (const b of a) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
