import { I18n } from 'i18n-js'
import enUS from './en-US'
import enGB from './en-GB'
import ptBR from './pt-BR'
import ptPT from './pt-PT'
import esES from './es-ES'
import deDE from './de-DE'
import frFR from './fr-FR'
import itIT from './it-IT'
import jaJP from './ja-JP'
import zhCN from './zh-CN'
import koKR from './ko-KR'
import hiIN from './hi-IN'
import plPL from './pl-PL'
import trTR from './tr-TR'
import ukUA from './uk-UA'
import svSE from './sv-SE'
import nlNL from './nl-NL'
import nbNO from './nb-NO'
import daDK from './da-DK'
import fiFI from './fi-FI'

const i18n = new I18n({
  'en-US': enUS,
  'en-GB': enGB,
  'pt-BR': ptBR,
  'pt-PT': ptPT,
  'es-ES': esES,
  'de-DE': deDE,
  'fr-FR': frFR,
  'it-IT': itIT,
  'ja-JP': jaJP,
  'zh-CN': zhCN,
  'ko-KR': koKR,
  'hi-IN': hiIN,
  'pl-PL': plPL,
  'tr-TR': trTR,
  'uk-UA': ukUA,
  'sv-SE': svSE,
  'nl-NL': nlNL,
  'nb-NO': nbNO,
  'da-DK': daDK,
  'fi-FI': fiFI,
})

i18n.enableFallback = true
i18n.defaultLocale = 'en-US'
i18n.locale = 'en-US'

export const SUPPORTED_LOCALES = [
  'en-US',
  'en-GB',
  'pt-BR',
  'pt-PT',
  'es-ES',
  'de-DE',
  'fr-FR',
  'it-IT',
  'ja-JP',
  'zh-CN',
  'ko-KR',
  'hi-IN',
  'pl-PL',
  'tr-TR',
  'uk-UA',
  'sv-SE',
  'nl-NL',
  'nb-NO',
  'da-DK',
  'fi-FI',
] as const
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]

export const setLocale = (locale: string) => {
  if (SUPPORTED_LOCALES.includes(locale as SupportedLocale)) {
    i18n.locale = locale
  }
}

export const getLocale = () => i18n.locale

export default i18n
