import i18n from '@/i18n'

export interface SessionDetails {
  title: string
  subtitle: string
  actionText: string
  imageUrl: string
}

// Session cards: the static config carries the untranslated fallbacks and images;
// localized strings are resolved lazily by getSessionDetails, because a module-level
// i18n.t call would freeze whatever locale was active at import time (before the
// device/user language is applied).
const SESSION_CONFIG: Record<string, { title: string; subtitle: string; actionText: string; imageUrl: string }> = {
  // Onboarding is now just the intro: meeting Rumi, how privacy works, and what the
  // journey looks like. The ideal-life exercise it used to contain is its own session
  // ('session_vision'), which the intro hands over to.
  onboarding: {
    title: "Welcome",
    subtitle: "Meet Rumi and see how this journey works",
    actionText: "Begin",
    imageUrl: 'https://images.unsplash.com/photo-1499209974431-9dddcece7f88?q=80&w=1000&auto=format&fit=crop',
  },
  // Where the user builds the vision for their ideal life — the first real exercise.
  session_vision: {
    title: "Your vision",
    subtitle: "Build a clear vision of the life you want to live",
    actionText: "Start Your Journey",
    imageUrl: 'https://images.unsplash.com/photo-1469474968028-56623f02e42e?q=80&w=1000&auto=format&fit=crop',
  },
  // The API's session type is 'checkin'; this key used to be 'checkin_daily', which no
  // longer matches anything the backend sends, so the card silently vanished. The
  // translated copy still lives under the 'checkin_daily' i18n family (see I18N_KEY_ALIAS).
  checkin: {
    // Sunlit grass and wildflowers — bright and alive, for a card the user
    // meets every day. Three constraints this hero imposes, all learned the
    // hard way: it crops to a ~3:1 centre band (the subject must sit
    // mid-frame); it must not read as sky, or it vanishes into the ambient
    // theme, which is itself a blurred sunset; and it must not be gloomy —
    // a dusk/jetty shot made a daily prompt feel bleak.
    // 'check-in' is the backend's word for this session, not the user's — the
    // card asks the question the session actually opens with.
    title: "How are you today?",
    subtitle: "A moment just for you",
    actionText: "Begin",
    imageUrl: 'https://images.unsplash.com/photo-1744170913610-745a869ddce9?q=80&w=1000&auto=format&fit=crop',
  },
  session_movement: {
    // Hikers on a trail heading into the mountains — obstacles ahead, already moving.
    title: "Obstacles and Movement",
    subtitle: "Move past what's holding you back",
    actionText: "Start Session",
    imageUrl: 'https://images.unsplash.com/photo-1551632811-561732d1e306?q=80&w=1000&auto=format&fit=crop',
  },
  session_energy: {
    // Sunrise mist over a meadow — the day's energy arriving.
    title: "Energy and Vitality",
    subtitle: "Recharge what fuels your days",
    actionText: "Start Session",
    imageUrl: 'https://images.unsplash.com/photo-1470252649378-9c29740c9fa8?q=80&w=1000&auto=format&fit=crop',
  },
  session_decisions: {
    // An empty road rolling over blind crests — the path ahead you can't fully see.
    title: "Fears and Decisions",
    subtitle: "Face your fears, choose your path",
    actionText: "Start Session",
    imageUrl: 'https://images.unsplash.com/photo-1471958680802-1345a694ba6d?q=80&w=1000&auto=format&fit=crop',
  },
  session_values: {
    // A summit — what you orient your climb by.
    title: "Values",
    subtitle: "Discover what matters most to you",
    actionText: "Explore Values",
    imageUrl: 'https://images.unsplash.com/photo-1454496522488-7a8e488e8606?q=80&w=1000&auto=format&fit=crop',
  },
  session_beliefs: {
    // Pines dissolving into fog — the idea of not seeing clearly.
    title: "Beliefs",
    subtitle: "Question the stories you tell yourself",
    actionText: "Challenge Beliefs",
    imageUrl: 'https://images.unsplash.com/photo-1487621167305-5d248087c724?q=80&w=1000&auto=format&fit=crop',
  },
  // Who the user is becoming through how they have been choosing to live. A path
  // through sunlit trees — the fog of 'session_beliefs' (the session this one
  // follows) burned off.
  session_identity: {
    title: "Identity",
    subtitle: "See who you are becoming",
    actionText: "Start Session",
    imageUrl: 'https://images.unsplash.com/photo-1441974231531-c6227db76b6e?q=80&w=1000&auto=format&fit=crop',
  },
  // Who you choose to be when life does not meet your expectations. Still water
  // under mountains — calm about what cannot be moved.
  session_acceptance: {
    title: "Expectations and Acceptance",
    subtitle: "Make peace with what you cannot control",
    actionText: "Start Session",
    imageUrl: 'https://images.unsplash.com/photo-1501785888041-af3ef285b470?q=80&w=1000&auto=format&fit=crop',
  },
  // Last of the deep sessions: attention as a limited resource — recovered in
  // 'session_acceptance' by ceasing to fight the uncontrollable — and choosing
  // what deserves it now. A single sunlit clearing in a dense forest: one place
  // the light lands.
  session_priorities: {
    title: "Priorities",
    subtitle: "Choose what deserves your attention",
    actionText: "Start Session",
    imageUrl: 'https://images.unsplash.com/photo-1476231682828-37e571bc172f?q=80&w=1000&auto=format&fit=crop',
  },
}

// Session types whose translated copy lives under a different key family than the type
// name implies. 'checkin' would derive 'session_checkin_*', but only 'session_checkin_title'
// exists there (a short label the admin screen uses) — the card's full title/subtitle/action
// set is translated in all 19 locales under 'session_checkin_daily_*'. Point at the copy
// that exists rather than renaming keys across every locale.
const I18N_KEY_ALIAS: Record<string, string> = {
  checkin: 'checkin_daily',
}

// getSessionDetails resolves a session's localized card strings at call time.
export function getSessionDetails(type: string): SessionDetails | undefined {
  const cfg = SESSION_CONFIG[type]
  if (!cfg) return undefined
  // 'session_movement' → i18n key 'session_movement_title' (not
  // 'session_session_movement_title'): the deep-session types already carry the
  // 'session_' prefix, so strip it before templating. This keeps ONE key family
  // — the same one the admin screen reads — instead of the doubled
  // 'session_session_*' family that used to shadow it.
  const key = I18N_KEY_ALIAS[type] ?? type.replace(/^session_/, '')
  return {
    title: i18n.t(`session_${key}_title`) || cfg.title,
    subtitle: i18n.t(`session_${key}_subtitle`) || cfg.subtitle,
    actionText: i18n.t(`session_${key}_action`) || cfg.actionText,
    imageUrl: cfg.imageUrl,
  }
}

export const AREA_COLORS: Record<string, string> = {
  health: '#ef4444',
  career: '#3b82f6',
  finances: '#f59e0b',
  relationships: '#ec4899',
  growth: '#8b5cf6',
  fun: '#14b8a6',
  environment: '#84cc16',
  spirit: '#a855f7',
}

export const getAreaClass = (area: string): string => {
  const normalized = area.toLowerCase().trim()
  if (normalized.includes('health') || normalized.includes('fitness') || normalized.includes('saúde') || normalized.includes('corpo')) return 'health'
  if (normalized.includes('career') || normalized.includes('work') || normalized.includes('carreira') || normalized.includes('trabalho')) return 'career'
  if (normalized.includes('finance') || normalized.includes('money') || normalized.includes('dinheiro') || normalized.includes('finanças')) return 'finances'
  if (normalized.includes('relationship') || normalized.includes('family') || normalized.includes('love') || normalized.includes('relacionamento') || normalized.includes('família')) return 'relationships'
  if (normalized.includes('growth') || normalized.includes('learning') || normalized.includes('crescimento') || normalized.includes('desenvolvimento')) return 'growth'
  if (normalized.includes('fun') || normalized.includes('leisure') || normalized.includes('lazer') || normalized.includes('diversão')) return 'fun'
  if (normalized.includes('environment') || normalized.includes('home') || normalized.includes('casa') || normalized.includes('ambiente')) return 'environment'
  if (normalized.includes('spirit') || normalized.includes('soul') || normalized.includes('espírito') || normalized.includes('espiritualidade')) return 'spirit'
  return 'default'
}

export const getGreeting = () => {
  const hour = new Date().getHours()
  if (hour >= 5 && hour < 12) return i18n.t('greeting_morning') || 'Good morning'
  if (hour >= 12 && hour < 18) return i18n.t('greeting_afternoon') || 'Good afternoon'
  return i18n.t('greeting_evening') || 'Good evening'
}
