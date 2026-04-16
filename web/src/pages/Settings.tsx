import { useState, useEffect, useRef } from 'react'
import { useAuth } from '../App'
import { api } from '../api'
import type { WebAuthnCredential } from '../api'
import Icon from '../components/Icon'
import './Settings.css'

export default function Settings() {
  const { user, setUser } = useAuth()

  return (
    <div className="settings">
      <h2><Icon name="settings" size={20} /> 設定</h2>
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
      setMsg('更新しました')
      const u = await api.me(); setUser(u)
    }
    return (
      <section className="card settings-section">
        <h3><Icon name="user" size={18} /> プロフィール</h3>
        <form onSubmit={save} className="settings-form">
          <label>表示名</label>
          <input value={displayName} onChange={e => setDisplayName(e.target.value)} />
          <button type="submit" className="primary"><Icon name="check" size={16} />保存</button>
          {msg && <p className="success">{msg}</p>}
        </form>
      </section>
    )
  }

  function AvatarSection() {
    const fileRef = useRef<HTMLInputElement>(null)
    const [uploading, setUploading] = useState(false)
    const upload = async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]; if (!file) return
      setUploading(true)
      try { await api.uploadAvatar(file); const u = await api.me(); setUser(u) }
      finally { setUploading(false) }
    }
    return (
      <section className="card settings-section">
        <h3><Icon name="camera" size={18} /> アバター</h3>
        <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
          {user?.avatar_path
            ? <img className="avatar" src={user.avatar_path} alt="" style={{ width: 64, height: 64 }} />
            : <span className="avatar placeholder" style={{ width: 64, height: 64, fontSize: 28 }}>{user?.display_name[0]}</span>
          }
          <div>
            <input type="file" accept="image/*" ref={fileRef} onChange={upload} style={{ display: 'none' }} />
            <button onClick={() => fileRef.current?.click()} disabled={uploading}>
              <Icon name="image" size={16} />{uploading ? 'アップロード中...' : '画像を変更'}
            </button>
          </div>
        </div>
      </section>
    )
  }

  function PasswordSection() {
    const [oldPass, setOldPass] = useState(''); const [newPass, setNewPass] = useState('')
    const [msg, setMsg] = useState(''); const [error, setError] = useState('')
    const submit = async (e: React.FormEvent) => {
      e.preventDefault(); setMsg(''); setError('')
      try { await api.changePassword(oldPass, newPass); setMsg('パスワードを変更しました'); setOldPass(''); setNewPass('') }
      catch (err: any) { setError(err.message) }
    }
    return (
      <section className="card settings-section">
        <h3><Icon name="lock" size={18} /> パスワード</h3>
        <form onSubmit={submit} className="settings-form">
          <input type="password" placeholder="現在のパスワード" value={oldPass} onChange={e => setOldPass(e.target.value)} required />
          <input type="password" placeholder="新しいパスワード（8文字以上）" value={newPass} onChange={e => setNewPass(e.target.value)} required minLength={8} />
          {error && <p className="error">{error}</p>}
          {msg && <p className="success">{msg}</p>}
          <button type="submit" className="primary"><Icon name="check" size={16} />変更する</button>
        </form>
      </section>
    )
  }

  function TOTPSection() {
    const [setupUrl, setSetupUrl] = useState(''); const [secret, setSecret] = useState('')
    const [code, setCode] = useState(''); const [msg, setMsg] = useState(''); const [error, setError] = useState('')
    const startSetup = async () => {
      setError(''); setMsg('')
      const r = await api.totpSetup(); setSecret(r.secret); setSetupUrl(r.url)
    }
    const confirm = async (e: React.FormEvent) => {
      e.preventDefault(); setError('')
      try { await api.totpConfirm(code); setMsg('TOTP を有効にしました'); setSetupUrl(''); setSecret(''); setCode(''); const u = await api.me(); setUser(u) }
      catch (err: any) { setError(err.message) }
    }
    const disable = async () => { await api.totpDisable(); const u = await api.me(); setUser(u) }

    return (
      <section className="card settings-section">
        <h3><Icon name="shield" size={18} /> 認証アプリ（TOTP）</h3>
        {user?.has_totp ? (
          <div>
            <p className="success">有効</p>
            <button className="danger" onClick={disable}><Icon name="trash" size={16} />無効にする</button>
          </div>
        ) : setupUrl ? (
          <div>
            <p className="meta">認証アプリでスキャンするか、シークレットを手動入力してください：</p>
            <p style={{ wordBreak: 'break-all', fontFamily: 'var(--mono)', fontSize: 12, margin: '8px 0', padding: 8, background: 'var(--accent-light)', borderRadius: 'var(--radius)' }}>{secret}</p>
            <form onSubmit={confirm} className="settings-form">
              <input placeholder="6桁の認証コード" value={code} onChange={e => setCode(e.target.value)} maxLength={6} inputMode="numeric" />
              {error && <p className="error">{error}</p>}
              {msg && <p className="success">{msg}</p>}
              <button type="submit" className="primary"><Icon name="check" size={16} />確認</button>
            </form>
          </div>
        ) : (
          <button onClick={startSetup}><Icon name="shield" size={16} />設定する</button>
        )}
      </section>
    )
  }

  function WebAuthnSection() {
    const [creds, setCreds] = useState<WebAuthnCredential[]>([])
    const [error, setError] = useState(''); const [msg, setMsg] = useState('')
    useEffect(() => { api.webauthnCredentials().then(d => setCreds(d.credentials)) }, [])

    const register = async () => {
      setError(''); setMsg('')
      try {
        const options: any = await api.webauthnRegisterBegin()
        const publicKey = {
          ...options.publicKey,
          challenge: b64ToBuf(options.publicKey.challenge),
          user: { ...options.publicKey.user, id: b64ToBuf(options.publicKey.user.id) },
          excludeCredentials: options.publicKey.excludeCredentials?.map((c: any) => ({ ...c, id: b64ToBuf(c.id) })),
        }
        const cred = await navigator.credentials.create({ publicKey }) as PublicKeyCredential
        const resp = cred.response as AuthenticatorAttestationResponse
        const result = await api.webauthnRegisterFinish({
          id: cred.id, rawId: bufTo64(cred.rawId), type: cred.type,
          response: { attestationObject: bufTo64(resp.attestationObject), clientDataJSON: bufTo64(resp.clientDataJSON) },
        })
        if (result.ok) { setMsg('セキュリティキーを登録しました'); const d = await api.webauthnCredentials(); setCreds(d.credentials); const u = await api.me(); setUser(u) }
        else setError(result.error || '登録に失敗しました')
      } catch (err: any) { setError(err.message || '操作がキャンセルされました') }
    }
    const remove = async (id: string) => {
      await api.webauthnDeleteCredential(id); const d = await api.webauthnCredentials(); setCreds(d.credentials); const u = await api.me(); setUser(u)
    }

    return (
      <section className="card settings-section">
        <h3><Icon name="key" size={18} /> セキュリティキー（WebAuthn）</h3>
        {creds.length > 0 && (
          <ul className="cred-list">
            {creds.map(c => (
              <li key={c.id}>
                <span><Icon name="key" size={14} /> {c.name} <span className="meta">({new Date(c.created_at).toLocaleDateString('ja-JP')})</span></span>
                <button className="ghost" onClick={() => remove(c.id)} style={{ color: 'var(--danger)' }}><Icon name="trash" size={14} /></button>
              </li>
            ))}
          </ul>
        )}
        {error && <p className="error">{error}</p>}
        {msg && <p className="success">{msg}</p>}
        <button onClick={register}><Icon name="plus" size={16} />キーを追加</button>
      </section>
    )
  }
}

function b64ToBuf(b: string): ArrayBuffer {
  const s = b.replace(/-/g,'+').replace(/_/g,'/');
  const p = s.length%4===0?'':'='.repeat(4-s.length%4);
  const d = atob(s+p); const a = new Uint8Array(d.length);
  for(let i=0;i<d.length;i++) a[i]=d.charCodeAt(i); return a.buffer;
}
function bufTo64(buf: ArrayBuffer): string {
  const a = new Uint8Array(buf); let s='';
  for(const b of a) s+=String.fromCharCode(b);
  return btoa(s).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
}
