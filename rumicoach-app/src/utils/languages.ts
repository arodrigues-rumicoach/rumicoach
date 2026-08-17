export interface Language {
  code: string
  name: string
  flag: string
}

export const LANGUAGES: Language[] = [
  { code: 'en-US', name: 'English (US)', flag: '🇺🇸' },
  { code: 'en-GB', name: 'English (UK)', flag: '🇬🇧' },
  { code: 'pt-PT', name: 'Português (Portugal)', flag: '🇵🇹' },
  { code: 'pt-BR', name: 'Português (Brasil)', flag: '🇧🇷' },
  { code: 'es-ES', name: 'Español (España)', flag: '🇪🇸' },
  { code: 'de-DE', name: 'Deutsch (Deutschland)', flag: '🇩🇪' },
  { code: 'fr-FR', name: 'Français (France)', flag: '🇫🇷' },
  { code: 'it-IT', name: 'Italiano (Italia)', flag: '🇮🇹' },
  { code: 'ja-JP', name: '日本語 (日本)', flag: '🇯🇵' },
  { code: 'zh-CN', name: '简体中文', flag: '🇨🇳' },
  { code: 'ko-KR', name: '한국어 (대한민국)', flag: '🇰🇷' },
  { code: 'hi-IN', name: 'हिन्दी (भारत)', flag: '🇮🇳' },
  { code: 'pl-PL', name: 'Polski (Polska)', flag: '🇵🇱' },
  { code: 'tr-TR', name: 'Türkçe (Türkiye)', flag: '🇹🇷' },
  { code: 'uk-UA', name: 'Українська (Україна)', flag: '🇺🇦' },
  { code: 'sv-SE', name: 'Svenska (Sverige)', flag: '🇸🇪' },
  { code: 'nl-NL', name: 'Nederlands (Nederland)', flag: '🇳🇱' },
  { code: 'nb-NO', name: 'Norsk Bokmål (Norge)', flag: '🇳🇴' },
  { code: 'da-DK', name: 'Dansk (Danmark)', flag: '🇩🇰' },
  { code: 'fi-FI', name: 'Suomi (Suomi)', flag: '🇫🇮' },
]
