// Minimal typed view of the JSON options returned by the server
// (github.com/go-webauthn/webauthn serialises these with base64url strings
// in places where ArrayBuffers are required by the browser API).

interface CredentialRef {
  id: string
  type: string
  transports?: string[]
}

export interface RegistrationOptionsJSON {
  publicKey: {
    challenge: string
    rp: PublicKeyCredentialRpEntity
    user: { id: string; name: string; displayName: string }
    pubKeyCredParams: PublicKeyCredentialParameters[]
    timeout?: number
    excludeCredentials?: CredentialRef[]
    authenticatorSelection?: AuthenticatorSelectionCriteria
    attestation?: AttestationConveyancePreference
    extensions?: Record<string, unknown>
  }
}

export interface LoginOptionsJSON {
  publicKey: {
    challenge: string
    timeout?: number
    rpId?: string
    allowCredentials?: CredentialRef[]
    userVerification?: UserVerificationRequirement
    extensions?: Record<string, unknown>
  }
}

export function b64ToBuf(b: string): ArrayBuffer {
  const s = b.replace(/-/g, '+').replace(/_/g, '/')
  const pad = s.length % 4 === 0 ? '' : '='.repeat(4 - (s.length % 4))
  const bin = atob(s + pad)
  const a = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) a[i] = bin.charCodeAt(i)
  return a.buffer
}

export function bufTo64(buf: ArrayBuffer): string {
  const a = new Uint8Array(buf)
  let s = ''
  for (const b of a) s += String.fromCharCode(b)
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function decodeRegistration(options: RegistrationOptionsJSON): PublicKeyCredentialCreationOptions {
  const pk = options.publicKey
  return {
    ...pk,
    challenge: b64ToBuf(pk.challenge),
    user: { ...pk.user, id: b64ToBuf(pk.user.id) },
    excludeCredentials: pk.excludeCredentials?.map(c => ({
      ...c,
      id: b64ToBuf(c.id),
      type: c.type as PublicKeyCredentialType,
      transports: c.transports as AuthenticatorTransport[] | undefined,
    })),
  }
}

export function decodeLogin(options: LoginOptionsJSON): PublicKeyCredentialRequestOptions {
  const pk = options.publicKey
  return {
    ...pk,
    challenge: b64ToBuf(pk.challenge),
    allowCredentials: pk.allowCredentials?.map(c => ({
      ...c,
      id: b64ToBuf(c.id),
      type: c.type as PublicKeyCredentialType,
      transports: c.transports as AuthenticatorTransport[] | undefined,
    })),
  }
}

export function encodeAttestation(cred: PublicKeyCredential) {
  const resp = cred.response as AuthenticatorAttestationResponse
  return {
    id: cred.id,
    rawId: bufTo64(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: bufTo64(resp.attestationObject),
      clientDataJSON: bufTo64(resp.clientDataJSON),
    },
  }
}

export function encodeAssertion(cred: PublicKeyCredential) {
  const resp = cred.response as AuthenticatorAssertionResponse
  return {
    id: cred.id,
    rawId: bufTo64(cred.rawId),
    type: cred.type,
    response: {
      authenticatorData: bufTo64(resp.authenticatorData),
      clientDataJSON: bufTo64(resp.clientDataJSON),
      signature: bufTo64(resp.signature),
      userHandle: resp.userHandle ? bufTo64(resp.userHandle) : null,
    },
  }
}

export function errMessage(err: unknown, fallback = '失敗しました'): string {
  if (err instanceof Error) return err.message
  return fallback
}
