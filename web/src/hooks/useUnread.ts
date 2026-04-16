import { useState, useEffect, useCallback } from 'react'
import { api } from '../api'

export function useUnread() {
  const [count, setCount] = useState(0)

  useEffect(() => {
    let active = true
    const tick = () => {
      api.unread()
        .then(d => { if (active) setCount(d.unread) })
        .catch(() => { /* ignore */ })
    }
    tick()
    const interval = setInterval(tick, 60_000)
    return () => { active = false; clearInterval(interval) }
  }, [])

  const markSeen = useCallback(async () => {
    await api.markSeen()
    setCount(0)
  }, [])

  const refresh = useCallback(() => {
    api.unread().then(d => setCount(d.unread)).catch(() => { /* ignore */ })
  }, [])

  return { count, refresh, markSeen }
}
