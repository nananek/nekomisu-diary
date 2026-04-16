import { createContext, useContext } from 'react'
import type { User } from './api'

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
