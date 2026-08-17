// Localized App Store Connect metadata for the membership group, its two plans and
// the three minute top-ups.
//
// This is *store* copy, not in-app copy: Apple shows the display name and description
// in the App Store listing and in the system purchase sheet. It is entered per locale
// under Monetization → Subscriptions / In-App Purchases, and there is no way to import
// it from the app bundle — the tables here are the source you paste from, kept in the
// repo so the wording can be reviewed and diffed like everything else.
//
// Product names and the "2 months free" phrasing are lifted from the website's own
// translations (rumicoach-website/src/website/translations/*.ts, pricing block) so the
// App Store, the web paywall and the app all name the same thing the same way.
//
// Apple silently truncates over-limit strings, so the character budgets below are
// enforced by appStoreLocalizations.test.ts rather than trusted to review.
//
// Locale keys are App Store Connect locale codes, which do not always match our i18n
// codes (it-IT → it, zh-CN → zh-Hans, nb-NO → no).

// Imported from ./instance rather than the @/i18n barrel on purpose: the barrel also
// re-exports I18nProvider, which pulls in react and expo-localization, and that drags
// React Native's Flow-typed source into scripts/appstore-subscriptions.ts — a plain Bun
// process with no Metro to strip Flow. instance.ts is i18n-js plus locale objects only.
import { SUPPORTED_LOCALES, type SupportedLocale } from '@/i18n/instance'
import type { BillingPeriod, TopUpKey } from './catalog'

/** App Store Connect field limits, in characters. */
export const APP_STORE_LIMITS = {
  subscriptionGroupDisplayName: 30,
  displayName: 30,
  description: 45,
} as const

/** App Store Connect locale code. */
export type AppStoreLocale =
  | 'en-US' | 'en-GB' | 'pt-BR' | 'pt-PT' | 'es-ES' | 'de-DE' | 'fr-FR' | 'it'
  | 'ja' | 'zh-Hans' | 'ko' | 'hi' | 'pl' | 'tr' | 'uk' | 'sv' | 'nl-NL'
  | 'no' | 'da' | 'fi'

/** Our i18n locale → the App Store Connect locale that serves it. */
export const APP_STORE_LOCALE_BY_APP_LOCALE: Record<SupportedLocale, AppStoreLocale> = {
  'en-US': 'en-US',
  'en-GB': 'en-GB',
  'pt-BR': 'pt-BR',
  'pt-PT': 'pt-PT',
  'es-ES': 'es-ES',
  'de-DE': 'de-DE',
  'fr-FR': 'fr-FR',
  'it-IT': 'it',
  'ja-JP': 'ja',
  'zh-CN': 'zh-Hans',
  'ko-KR': 'ko',
  'hi-IN': 'hi',
  'pl-PL': 'pl',
  'tr-TR': 'tr',
  'uk-UA': 'uk',
  'sv-SE': 'sv',
  'nl-NL': 'nl-NL',
  'nb-NO': 'no',
  'da-DK': 'da',
  'fi-FI': 'fi',
}

/**
 * Subscription Group Display Name (≤30 chars) — the membership name exactly as the
 * website renders it in the pricing card.
 */
export const GROUP_DISPLAY_NAME: Record<AppStoreLocale, string> = {
  'en-US': 'Rumi Membership',
  'en-GB': 'Rumi Membership',
  'pt-BR': 'Assinatura Rumi',
  'pt-PT': 'Assinatura Rumi',
  'es-ES': 'Membresía Rumi',
  'de-DE': 'Rumi-Mitgliedschaft',
  'fr-FR': 'Abonnement Rumi',
  it: 'Abbonamento Rumi',
  ja: 'Rumiメンバーシップ',
  'zh-Hans': 'Rumi 会员',
  ko: 'Rumi 멤버십',
  hi: 'Rumi सदस्यता',
  pl: 'Członkostwo Rumi',
  tr: 'Rumi Üyeliği',
  uk: 'Членство Rumi',
  sv: 'Rumi-medlemskap',
  'nl-NL': 'Rumi-lidmaatschap',
  no: 'Rumi-medlemskap',
  da: 'Rumi-medlemskab',
  fi: 'Rumi-jäsenyys',
}

export type ProductLocalization = {
  /** Display Name (≤30 chars). */
  readonly displayName: string
  /** Description (≤45 chars). */
  readonly description: string
}

/**
 * Membership plans. The monthly description states what you get; the annual one
 * carries the "2 months free" promise in each locale's own wording.
 */
export const PLAN_LOCALIZATIONS: Record<
  BillingPeriod,
  Record<AppStoreLocale, ProductLocalization>
> = {
  monthly: {
    'en-US': {
      displayName: 'Rumi Membership Monthly',
      description: '150 guided voice minutes every month.',
    },
    'en-GB': {
      displayName: 'Rumi Membership Monthly',
      description: '150 guided voice minutes every month.',
    },
    'pt-BR': {
      displayName: 'Assinatura Rumi Mensal',
      description: '150 minutos de voz guiada por mês.',
    },
    'pt-PT': {
      displayName: 'Assinatura Rumi Mensal',
      description: '150 minutos de voz guiada por mês.',
    },
    'es-ES': {
      displayName: 'Membresía Rumi Mensual',
      description: '150 minutos de voz guiada al mes.',
    },
    'de-DE': {
      displayName: 'Rumi-Mitgliedschaft Monatlich',
      description: '150 Minuten Sprachcoaching pro Monat.',
    },
    'fr-FR': {
      displayName: 'Abonnement Rumi Mensuel',
      description: '150 minutes de voix guidée par mois.',
    },
    it: {
      displayName: 'Abbonamento Rumi Mensile',
      description: '150 minuti di voce guidata al mese.',
    },
    ja: {
      displayName: 'Rumiメンバーシップ（月額）',
      description: '毎月150分のガイド付き音声会話。',
    },
    'zh-Hans': {
      displayName: 'Rumi 会员（按月）',
      description: '每月150分钟引导式语音对话。',
    },
    ko: {
      displayName: 'Rumi 멤버십 월간',
      description: '매월 150분 가이드 음성 대화.',
    },
    hi: {
      displayName: 'Rumi सदस्यता मासिक',
      description: 'हर महीने 150 मिनट निर्देशित बातचीत।',
    },
    pl: {
      displayName: 'Członkostwo Rumi miesięcznie',
      description: '150 minut rozmów głosowych miesięcznie.',
    },
    tr: {
      displayName: 'Rumi Üyeliği Aylık',
      description: 'Her ay 150 dakika rehberli sesli sohbet.',
    },
    uk: {
      displayName: 'Членство Rumi щомісяця',
      description: '150 хвилин голосових розмов щомісяця.',
    },
    sv: {
      displayName: 'Rumi-medlemskap månad',
      description: '150 minuter guidade röstsamtal per månad.',
    },
    'nl-NL': {
      displayName: 'Rumi-lidmaatschap maand',
      description: '150 minuten begeleide spraak per maand.',
    },
    no: {
      displayName: 'Rumi-medlemskap måned',
      description: '150 minutter veiledede samtaler per måned.',
    },
    da: {
      displayName: 'Rumi-medlemskab måned',
      description: '150 minutters guidede samtaler pr. måned.',
    },
    fi: {
      displayName: 'Rumi-jäsenyys kuukausi',
      description: '150 minuuttia ohjattua puhetta kuussa.',
    },
  },
  annual: {
    'en-US': {
      displayName: 'Rumi Membership Annual',
      description: '150 minutes a month. 2 months free.',
    },
    'en-GB': {
      displayName: 'Rumi Membership Annual',
      description: '150 minutes a month. 2 months free.',
    },
    'pt-BR': {
      displayName: 'Assinatura Rumi Anual',
      description: '150 minutos por mês. 2 meses grátis.',
    },
    'pt-PT': {
      displayName: 'Assinatura Rumi Anual',
      description: '150 minutos por mês. 2 meses grátis.',
    },
    'es-ES': {
      displayName: 'Membresía Rumi Anual',
      description: '150 minutos al mes. 2 meses gratis.',
    },
    'de-DE': {
      displayName: 'Rumi-Mitgliedschaft Jährlich',
      description: '150 Minuten/Monat. 2 Monate geschenkt.',
    },
    'fr-FR': {
      displayName: 'Abonnement Rumi Annuel',
      description: '150 minutes par mois. 2 mois offerts.',
    },
    it: {
      displayName: 'Abbonamento Rumi Annuale',
      description: '150 minuti al mese. 2 mesi gratis.',
    },
    ja: {
      displayName: 'Rumiメンバーシップ（年額）',
      description: '毎月150分。2ヶ月分無料。',
    },
    'zh-Hans': {
      displayName: 'Rumi 会员（按年）',
      description: '每月150分钟，免费2个月。',
    },
    ko: {
      displayName: 'Rumi 멤버십 연간',
      description: '매월 150분. 2개월 무료.',
    },
    hi: {
      displayName: 'Rumi सदस्यता वार्षिक',
      description: 'हर महीने 150 मिनट। 2 महीने मुफ़्त।',
    },
    pl: {
      displayName: 'Członkostwo Rumi rocznie',
      description: '150 minut na miesiąc. 2 miesiące gratis.',
    },
    tr: {
      displayName: 'Rumi Üyeliği Yıllık',
      description: 'Ayda 150 dakika. 2 ay hediye.',
    },
    uk: {
      displayName: 'Членство Rumi щороку',
      description: '150 хвилин на місяць. 2 місяці безкоштовно.',
    },
    sv: {
      displayName: 'Rumi-medlemskap år',
      description: '150 minuter per månad. 2 månader gratis.',
    },
    'nl-NL': {
      displayName: 'Rumi-lidmaatschap jaar',
      description: '150 minuten per maand. 2 maanden gratis.',
    },
    no: {
      displayName: 'Rumi-medlemskap år',
      description: '150 minutter per måned. 2 måneder gratis.',
    },
    da: {
      displayName: 'Rumi-medlemskab år',
      description: '150 minutter pr. måned. 2 måneder gratis.',
    },
    fi: {
      displayName: 'Rumi-jäsenyys vuosi',
      description: '150 minuuttia kuussa. 2 kk ilmaiseksi.',
    },
  },
}

/**
 * Minute top-ups (consumables). Names come from the website's top-up cards; every
 * description repeats the rollover promise, which is the reason people buy them.
 */
export const TOP_UP_LOCALIZATIONS: Record<
  TopUpKey,
  Record<AppStoreLocale, ProductLocalization>
> = {
  topup_quick: {
    'en-US': { displayName: 'Quick Boost – 60 min', description: '60 extra minutes that never expire.' },
    'en-GB': { displayName: 'Quick Boost – 60 min', description: '60 extra minutes that never expire.' },
    'pt-BR': { displayName: 'Impulso Rápido – 60 min', description: '+60 minutos que nunca expiram.' },
    'pt-PT': { displayName: 'Impulso Rápido – 60 min', description: '+60 minutos que nunca expiram.' },
    'es-ES': { displayName: 'Impulso rápido – 60 min', description: '+60 minutos que nunca caducan.' },
    'de-DE': { displayName: 'Schneller Schub – 60 Min', description: '+60 Minuten, die nie verfallen.' },
    'fr-FR': { displayName: 'Coup de pouce – 60 min', description: "+60 minutes qui n'expirent jamais." },
    it: { displayName: 'Spinta rapida – 60 min', description: '+60 minuti che non scadono mai.' },
    ja: { displayName: 'クイックブースト 60分', description: '+60分。有効期限はありません。' },
    'zh-Hans': { displayName: '快速补给 60 分钟', description: '+60 分钟，永不过期。' },
    ko: { displayName: '빠른 충전 60분', description: '+60분. 만료되지 않습니다.' },
    hi: { displayName: 'त्वरित बूस्ट – 60 मिनट', description: '+60 मिनट, कभी समाप्त नहीं होते।' },
    pl: { displayName: 'Szybki zastrzyk – 60 min', description: '+60 minut, które nigdy nie wygasają.' },
    tr: { displayName: 'Hızlı Destek – 60 dk', description: '+60 dakika, süresi hiç dolmaz.' },
    uk: { displayName: 'Швидкий поштовх – 60 хв', description: '+60 хвилин, які не закінчуються.' },
    sv: { displayName: 'Snabb boost – 60 min', description: '+60 minuter som aldrig går ut.' },
    'nl-NL': { displayName: 'Snelle boost – 60 min', description: '+60 minuten die nooit verlopen.' },
    no: { displayName: 'Rask boost – 60 min', description: '+60 minutter som aldri utløper.' },
    da: { displayName: 'Hurtigt boost – 60 min', description: '+60 minutter, der aldrig udløber.' },
    fi: { displayName: 'Nopea boosti – 60 min', description: '+60 minuuttia, jotka eivät vanhene.' },
  },
  topup_deep: {
    'en-US': { displayName: 'Deep Dive – 120 min', description: '120 extra minutes that never expire.' },
    'en-GB': { displayName: 'Deep Dive – 120 min', description: '120 extra minutes that never expire.' },
    'pt-BR': { displayName: 'Mergulho Profundo – 120 min', description: '+120 minutos que nunca expiram.' },
    'pt-PT': { displayName: 'Mergulho Profundo – 120 min', description: '+120 minutos que nunca expiram.' },
    'es-ES': { displayName: 'Inmersión profunda – 120 min', description: '+120 minutos que nunca caducan.' },
    'de-DE': { displayName: 'Tiefgang – 120 Min', description: '+120 Minuten, die nie verfallen.' },
    'fr-FR': { displayName: 'Immersion profonde – 120 min', description: "+120 minutes qui n'expirent jamais." },
    it: { displayName: 'Immersione profonda – 120 min', description: '+120 minuti che non scadono mai.' },
    ja: { displayName: 'ディープダイブ 120分', description: '+120分。有効期限はありません。' },
    'zh-Hans': { displayName: '深度沉浸 120 分钟', description: '+120 分钟，永不过期。' },
    ko: { displayName: '깊은 몰입 120분', description: '+120분. 만료되지 않습니다.' },
    hi: { displayName: 'गहन विसर्जन – 120 मिनट', description: '+120 मिनट, कभी समाप्त नहीं होते।' },
    pl: { displayName: 'Głębokie zanurzenie – 120 min', description: '+120 minut, które nigdy nie wygasają.' },
    tr: { displayName: 'Derin Dalış – 120 dk', description: '+120 dakika, süresi hiç dolmaz.' },
    uk: { displayName: 'Глибоке занурення – 120 хв', description: '+120 хвилин, які не закінчуються.' },
    sv: { displayName: 'Djupdykning – 120 min', description: '+120 minuter som aldrig går ut.' },
    'nl-NL': { displayName: 'Diepe duik – 120 min', description: '+120 minuten die nooit verlopen.' },
    no: { displayName: 'Dypdykk – 120 min', description: '+120 minutter som aldri utløper.' },
    da: { displayName: 'Dybdegående – 120 min', description: '+120 minutter, der aldrig udløber.' },
    fi: { displayName: 'Syväsukellus – 120 min', description: '+120 minuuttia, jotka eivät vanhene.' },
  },
  topup_power: {
    'en-US': { displayName: 'Power User – 240 min', description: '240 extra minutes that never expire.' },
    'en-GB': { displayName: 'Power User – 240 min', description: '240 extra minutes that never expire.' },
    'pt-BR': { displayName: 'Super Usuário – 240 min', description: '+240 minutos que nunca expiram.' },
    'pt-PT': { displayName: 'Super Utilizador – 240 min', description: '+240 minutos que nunca expiram.' },
    'es-ES': { displayName: 'Usuario avanzado – 240 min', description: '+240 minutos que nunca caducan.' },
    'de-DE': { displayName: 'Power-User – 240 Min', description: '+240 Minuten, die nie verfallen.' },
    'fr-FR': { displayName: 'Utilisateur intensif – 240 min', description: "+240 minutes qui n'expirent jamais." },
    it: { displayName: 'Utente avanzato – 240 min', description: '+240 minuti che non scadono mai.' },
    ja: { displayName: 'パワーユーザー 240分', description: '+240分。有効期限はありません。' },
    'zh-Hans': { displayName: '重度用户 240 分钟', description: '+240 分钟，永不过期。' },
    ko: { displayName: '파워 유저 240분', description: '+240분. 만료되지 않습니다.' },
    hi: { displayName: 'पावर यूज़र – 240 मिनट', description: '+240 मिनट, कभी समाप्त नहीं होते।' },
    pl: { displayName: 'Zaawansowany – 240 min', description: '+240 minut, które nigdy nie wygasają.' },
    tr: { displayName: 'Yoğun Kullanıcı – 240 dk', description: '+240 dakika, süresi hiç dolmaz.' },
    uk: { displayName: 'Активний користувач – 240 хв', description: '+240 хвилин, які не закінчуються.' },
    sv: { displayName: 'Storanvändare – 240 min', description: '+240 minuter som aldrig går ut.' },
    'nl-NL': { displayName: 'Power-gebruiker – 240 min', description: '+240 minuten die nooit verlopen.' },
    no: { displayName: 'Storbruker – 240 min', description: '+240 minutter som aldri utløper.' },
    da: { displayName: 'Storforbruger – 240 min', description: '+240 minutter, der aldrig udløber.' },
    fi: { displayName: 'Tehokäyttäjä – 240 min', description: '+240 minuuttia, jotka eivät vanhene.' },
  },
}

/** Every App Store locale we publish subscription metadata for. */
export const APP_STORE_LOCALES: readonly AppStoreLocale[] = SUPPORTED_LOCALES.map(
  locale => APP_STORE_LOCALE_BY_APP_LOCALE[locale]
)
