import { useEffect, useRef } from 'react'

/** Refetches only when a page returns from the browser's background state. */
export function usePageVisibilityRefetch(onVisible: () => void) {
  const callbackRef = useRef(onVisible)
  callbackRef.current = onVisible

  useEffect(() => {
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') callbackRef.current()
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () =>
      document.removeEventListener('visibilitychange', handleVisibilityChange)
  }, [])
}
