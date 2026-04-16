import { useState, useEffect } from 'react'

type Theme = 'light' | 'dark' | 'auto'

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function useTheme() {
  const [pref, setPref] = useState<Theme>(() => {
    return (localStorage.getItem('theme') as Theme) || 'auto'
  })
  const [systemTheme, setSystemTheme] = useState<'light' | 'dark'>(getSystemTheme)
  const resolved = pref === 'auto' ? systemTheme : pref

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', resolved)
    localStorage.setItem('theme', pref)
  }, [pref, resolved])

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => setSystemTheme(mq.matches ? 'dark' : 'light')
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])

  const cycle = () => {
    setPref(p => p === 'auto' ? 'light' : p === 'light' ? 'dark' : 'auto')
  }

  return { pref, resolved, setPref, cycle }
}
