import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'
import { getLocales } from 'expo-localization'
import i18n, { setLocale, SUPPORTED_LOCALES, type SupportedLocale } from './instance'

interface I18nContextType {
  locale: string
  setLanguage: (locale: string) => void
}

const I18nContext = createContext<I18nContextType>({
  locale: 'en-US',
  setLanguage: () => { },
})

export function useI18n() {
  return useContext(I18nContext)
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState(() => {
    const deviceLocale = getLocales()?.[0]?.languageTag
    if (deviceLocale && SUPPORTED_LOCALES.includes(deviceLocale as SupportedLocale)) {
      setLocale(deviceLocale)
      return deviceLocale
    }
    return 'en-US'
  })

  const setLanguage = useCallback((newLocale: string) => {
    setLocale(newLocale)
    setLocaleState(newLocale)
  }, [])

  return (
    <I18nContext.Provider value={{ locale, setLanguage }}>
      {children}
    </I18nContext.Provider>
  )
}
