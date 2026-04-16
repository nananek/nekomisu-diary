import { useState, useEffect, useCallback } from 'react'
import { api } from '../api'

export function useUnread() {
  const [count, setCount] = useState(0)

  const refresh = useCallback(async () => {
    try {
      const d = await api.unread()
      setCount(d.unread)
    } catch { /* ignore */ }
  }, [])

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, 60_000) // every minute
    return () => clearInterval(interval)
  }, [refresh])

  const markSeen = useCallback(async () => {
    await api.markSeen()
    setCount(0)
  }, [])

  return { count, refresh, markSeen }
}
