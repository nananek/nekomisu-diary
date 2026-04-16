import { useState, useEffect } from 'react'

type Theme = 'light' | 'dark' | 'auto'

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function resolveTheme(pref: Theme): 'light' | 'dark' {
  return pref === 'auto' ? getSystemTheme() : pref
}

export function useTheme() {
  const [pref, setPref] = useState<Theme>(() => {
    return (localStorage.getItem('theme') as Theme) || 'auto'
  })
  // "auto" subscribes to the system preference changes; bumping this forces
  // a re-render so `resolved` below updates.
  const [systemBump, setSystemBump] = useState(0)
  const resolved = resolveTheme(pref)

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', resolved)
    localStorage.setItem('theme', pref)
  }, [pref, resolved])

  useEffect(() => {
    if (pref !== 'auto') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => setSystemBump(n => n + 1)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [pref])

  // Silence unused-var (systemBump is read via closure on re-render)
  void systemBump

  const cycle = () => {
    setPref(p => p === 'auto' ? 'light' : p === 'light' ? 'dark' : 'auto')
  }

  return { pref, resolved, setPref, cycle }
}
