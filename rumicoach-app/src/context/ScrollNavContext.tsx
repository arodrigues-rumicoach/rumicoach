import { createContext, useContext, useState, useCallback, useRef, useMemo, useEffect, type ReactNode } from 'react'
import type { NativeScrollEvent, NativeSyntheticEvent } from 'react-native'
import { useSharedValue, type SharedValue } from 'react-native-reanimated'

interface ScrollNavContextValue {
  isShrunk: boolean
  scrollY: SharedValue<number>
  onScroll: (event: NativeSyntheticEvent<NativeScrollEvent>) => void
  reset: () => void
}

const ScrollNavContext = createContext<ScrollNavContextValue | null>(null)

const SCROLL_THRESHOLD = 10

export function ScrollNavProvider({ children }: { children: ReactNode }) {
  const [isShrunk, setIsShrunk] = useState(false)
  const scrollY = useSharedValue(0)
  const lastOffsetY = useRef(0)
  const isShrunkRef = useRef(false)

  useEffect(() => { isShrunkRef.current = isShrunk }, [isShrunk])

  const onScroll = useCallback((event: NativeSyntheticEvent<NativeScrollEvent>) => {
    const { contentOffset, contentSize, layoutMeasurement } = event.nativeEvent
    const maxY = Math.max(0, contentSize.height - layoutMeasurement.height)
    const currentY = Math.max(0, Math.min(contentOffset.y, maxY))

    scrollY.value = currentY

    const delta = currentY - lastOffsetY.current

    if (Math.abs(delta) < SCROLL_THRESHOLD) return

    lastOffsetY.current = currentY

    if (delta > 0 && !isShrunkRef.current) {
      setIsShrunk(true)
    } else if (delta < 0 && isShrunkRef.current) {
      setIsShrunk(false)
    }
  }, [scrollY])

  const reset = useCallback(() => {
    setIsShrunk(false)
    lastOffsetY.current = scrollY.value
    scrollY.value = 0
  }, [scrollY])

  const value = useMemo(() => ({ isShrunk, scrollY, onScroll, reset }), [isShrunk, scrollY, onScroll, reset])

  return (
    <ScrollNavContext.Provider value={value}>
      {children}
    </ScrollNavContext.Provider>
  )
}

export function useScrollNav() {
  const context = useContext(ScrollNavContext)
  if (!context) {
    throw new Error('useScrollNav must be used within ScrollNavProvider')
  }
  return context
}
