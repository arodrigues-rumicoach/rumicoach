import { createContext, useState, useEffect, useCallback, useRef, useMemo, type ReactNode } from 'react'
import { getStorageAdapter } from '../adapters/storage'
import { useAuth } from '../hooks/useAuth'
import { trackSettingChanged } from '@/analytics'

type VisualizerType = 'organic' | 'waveform'
export type ThemeType = 'lavender' | 'fireplace' | 'mountain_lake' | 'rain' | 'sunset_beach' | 'waterfall'

export interface ColorScheme {
  primary: string
  secondary: string
  tertiary: string
  accent: string
  navIcon: string
  navIconText: string
  navIconBlur: string
  navBlur: string
}

export interface SettingsContextType {
  visualizerType: VisualizerType
  setVisualizerType: (type: VisualizerType) => void
  shakeToReport: boolean
  setShakeToReport: (enabled: boolean) => void
  theme: ThemeType
  setTheme: (type: ThemeType) => Promise<void>
  applyRemoteTheme: (type: string) => void
  colorScheme: ColorScheme
  isLoading: boolean
}

const COLOR_SCHEMES: Record<string, ColorScheme> = {
  green: {
    primary: '#1a5f4f', secondary: '#d4f0ea', tertiary: '#0d3d32', accent: '#2d8a6e',
    navIcon: '#e0e0e0', navIconText: '#ffffff', navIconBlur: '#1a5f4f80', navBlur: '#1a5f4f60',
  },
  blue: {
    primary: '#1b4f7d', secondary: '#d4e8f5', tertiary: '#0d3254', accent: '#2d6a9e',
    navIcon: '#d0d0d0', navIconText: '#ffffff', navIconBlur: '#1b4f7d80', navBlur: '#1b4f7d60',
  },
  brown: {
    primary: '#5e3b08', secondary: '#f5e6d3', tertiary: '#3d2605', accent: '#8b5a2b',
    navIcon: '#cccccc', navIconText: '#ffffff', navIconBlur: '#5e3b0880', navBlur: '#5e3b0860',
  },
  violet: {
    primary: '#6e4060', secondary: '#e8d4dc', tertiary: '#4a2a42', accent: '#9e6b87',
    navIcon: '#c8c8c8', navIconText: '#ffffff', navIconBlur: '#6e406080', navBlur: '#6e406060',
  },
}

const THEME_COLORS: Record<string, ColorScheme> = {
  lavender: COLOR_SCHEMES.violet,
  fireplace: COLOR_SCHEMES.brown,
  mountain_lake: COLOR_SCHEMES.blue,
  rain: COLOR_SCHEMES.green,
  sunset_beach: COLOR_SCHEMES.brown,
  waterfall: COLOR_SCHEMES.blue,
}

const VALID_THEMES: ThemeType[] = ['lavender', 'fireplace', 'mountain_lake', 'rain', 'sunset_beach', 'waterfall']

const VISUALIZER_KEY = 'rumi_visualizer_type'
// Device-local on purpose: the accelerometer belongs to this phone, so someone
// who turns the gesture off here should not have it come back on their tablet.
const SHAKE_KEY = 'rumi_shake_to_report'
const THEME_KEY = 'rumi_theme'

export const SettingsContext = createContext<SettingsContextType | undefined>(undefined)

export function SettingsProvider({ children }: { children: ReactNode }) {
  const { user, updateUser } = useAuth()
  const [visualizerType, setVisualizerTypeState] = useState<VisualizerType>('organic')
  const [shakeToReport, setShakeToReportState] = useState(true)
  const [theme, setThemeState] = useState<ThemeType>('sunset_beach')
  const [isLoading, setIsLoading] = useState(true)
  const settingsLoadedRef = useRef(false)
  const hasDeviceThemeRef = useRef(false)

  const userRef = useRef(user)
  useEffect(() => { userRef.current = user }, [user])

  useEffect(() => {
    if (!settingsLoadedRef.current) return
    if (hasDeviceThemeRef.current) return
    const userTheme = user?.theme as ThemeType | undefined
    if (!userTheme || !VALID_THEMES.includes(userTheme)) return
    hasDeviceThemeRef.current = true
    if (userTheme !== theme) {
      setThemeState(userTheme)
      getStorageAdapter().setItemAsync(THEME_KEY, userTheme).catch(() => { })
    }
  }, [user?.theme, theme, isLoading])

  const loadSettings = useCallback(async () => {
    let isMounted = true
    try {
      const storedVisualizer = await getStorageAdapter().getItemAsync(VISUALIZER_KEY)
      if (isMounted && (storedVisualizer === 'organic' || storedVisualizer === 'waveform')) {
        setVisualizerTypeState(storedVisualizer as VisualizerType)
      }
      const storedTheme = await getStorageAdapter().getItemAsync(THEME_KEY)
      if (isMounted && storedTheme && VALID_THEMES.includes(storedTheme as ThemeType)) {
        hasDeviceThemeRef.current = true
        setThemeState(storedTheme as ThemeType)
      }
      const storedShake = await getStorageAdapter().getItemAsync(SHAKE_KEY)
      // Absent means never set, which is on — the default.
      if (isMounted && storedShake === 'off') setShakeToReportState(false)
    } catch {
      // ignore
    } finally {
      if (isMounted) {
        settingsLoadedRef.current = true
        setIsLoading(false)
      }
    }
    return () => { isMounted = false }
  }, [])

  useEffect(() => { loadSettings() }, [loadSettings])

  const setVisualizerType = useCallback((type: VisualizerType) => {
    setVisualizerTypeState(type)
    trackSettingChanged('visualizer', type)
    getStorageAdapter().setItemAsync(VISUALIZER_KEY, type).catch(() => { })
  }, [])

  const setShakeToReport = useCallback((enabled: boolean) => {
    setShakeToReportState(enabled)
    trackSettingChanged('shakeToReport', enabled)
    getStorageAdapter().setItemAsync(SHAKE_KEY, enabled ? 'on' : 'off').catch(() => { })
  }, [])

  // Applies the theme locally right away (so the UI responds instantly) but
  // resolves only once the server has it. Callers that navigate afterwards MUST
  // await: /journey echoes the stored theme back through applyRemoteTheme,
  // so leaving before the PATCH lands makes the stale server value overwrite
  // the selection the user just made.
  const setTheme = useCallback(async (type: ThemeType) => {
    hasDeviceThemeRef.current = true
    setThemeState(type)
    // Only the user's own pick. applyRemoteTheme below is the AI choosing, and
    // counting that as a preference change would drown the real signal.
    trackSettingChanged('theme', type)
    getStorageAdapter().setItemAsync(THEME_KEY, type).catch(() => { })
    if (!userRef.current) return
    try {
      await updateUser({ theme: type })
    } catch (err) {
      if (__DEV__) console.error('Failed to persist theme', err)
    }
  }, [updateUser])

  // applyRemoteTheme applies a server-picked theme (the AI's end-of-session choice
  // returned by /journey) for this device without persisting it as the user's
  // manual settings choice (user.theme stays untouched).
  const applyRemoteTheme = useCallback((type: string) => {
    if (!VALID_THEMES.includes(type as ThemeType)) return
    hasDeviceThemeRef.current = true
    setThemeState(type as ThemeType)
    getStorageAdapter().setItemAsync(THEME_KEY, type).catch(() => { })
  }, [])

  const colorScheme = useMemo(() => THEME_COLORS[theme] ?? THEME_COLORS.waterfall, [theme])

  const value = useMemo(() => ({
    visualizerType, setVisualizerType, shakeToReport, setShakeToReport, theme, setTheme, applyRemoteTheme, colorScheme, isLoading
  }), [visualizerType, setVisualizerType, shakeToReport, setShakeToReport, theme, setTheme, applyRemoteTheme, colorScheme, isLoading])

  return (
    <SettingsContext.Provider value={value}>
      {children}
    </SettingsContext.Provider>
  )
}
