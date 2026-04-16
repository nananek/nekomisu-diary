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
  const [resolved, setResolved] = useState<'light' | 'dark'>(() => resolveTheme(pref))

  useEffect(() => {
    const r = resolveTheme(pref)
    setResolved(r)
    document.documentElement.setAttribute('data-theme', r)
    localStorage.setItem('theme', pref)
  }, [pref])

  useEffect(() => {
    if (pref !== 'auto') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => {
      const r = resolveTheme('auto')
      setResolved(r)
      document.documentElement.setAttribute('data-theme', r)
    }
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [pref])

  const cycle = () => {
    setPref(p => p === 'auto' ? 'light' : p === 'light' ? 'dark' : 'auto')
  }

  return { pref, resolved, setPref, cycle }
}
