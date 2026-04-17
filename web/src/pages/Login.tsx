import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth'
import { api } from '../api'
import type { LoginResult } from '../api'
import Icon from '../components/Icon'
import { decodeLogin, encodeAssertion, errMessage } from '../lib/webauthn'
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
    } catch (err) {
      setError(errMessage(err, 'ログインに失敗しました'))
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
    } catch (err) {
      setError(errMessage(err))
    }
  }

  const loginWebAuthn = async () => {
    setError('')
    try {
      const options = await api.webauthnLoginBegin()
      const publicKey = decodeLogin(options)
      const assertion = await navigator.credentials.get({ publicKey }) as PublicKeyCredential
      const result = await api.webauthnLoginFinish(encodeAssertion(assertion))
      if (result.ok) {
        const user = await api.me()
        setUser(user)
        nav('/')
      } else {
        setError(result.error || '認証に失敗しました')
      }
    } catch (err) {
      setError(errMessage(err, '認証に失敗しました'))
    }
  }

  // Passkey-only sign in: no username/password required.
  const signInWithPasskey = async () => {
    setError('')
    try {
      const options = await api.webauthnDiscoverableBegin()
      const publicKey = decodeLogin(options)
      const assertion = await navigator.credentials.get({ publicKey }) as PublicKeyCredential
      const result = await api.webauthnDiscoverableFinish(encodeAssertion(assertion))
      if (result.ok) {
        const user = await api.me()
        setUser(user)
        nav('/')
      } else {
        setError(result.error || 'パスキー認証に失敗しました')
      }
    } catch (err) {
      setError(errMessage(err, 'パスキー認証に失敗しました'))
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
        <h1>ねこのみすきー交換日記</h1>
        <button onClick={signInWithPasskey} className="passkey-btn" style={{ width: '100%', marginBottom: 12 }}>
          <Icon name="key" size={18} />パスキーでサインイン
        </button>
        <div className="login-separator"><span>または</span></div>
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
