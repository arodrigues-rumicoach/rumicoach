export { api, fetchUsageCalendar } from './client'

export type CoachType = 'Commander' | 'Strategist' | 'Empath' | 'Buddy'

export interface User {
  id: string
  email?: string
  name?: string
  avatarUrl?: string
  coach?: CoachType
  dateOfBirth?: string | null
  gender?: 'male' | 'female' | 'other' | null
  coachGender?: 'male' | 'female' | null
  phoneNumber?: string | null
  country?: string | null
  preferredLanguage?: string | null
  wheelOfLife?: Record<string, number>
  theme?: string
  focusArea?: string | null
  visualizerType?: string
  coachVoice?: 'enceladus' | 'algieba' | 'gacrux' | 'charon' | 'vindemiatrix' | 'aoede' | null
  region?: 'eu' | 'us'
  /** How long WhatsApp/Telegram messages are kept, in days. 0 keeps them
   *  indefinitely. Server-side default is 7. */
  chatHistoryRetentionDays?: number
  /** Voice-session balance in seconds. Can go negative: a session in progress is
   *  debited at its end, so the last one can overshoot. Absent on payloads that are
   *  not a full /me; shown on the settings screen, which treats absent as unknown. */
  balanceSeconds?: number
  /** True while the account still has free introductory sessions left — the onboarding
   *  intro and the Vision session it hands over to.
   *
   *  Reported for display only. Whether a session may start is the server's decision
   *  and it makes it on the WebSocket, so nothing in the app may gate on this: doing
   *  that is what once paywalled users part-way through onboarding. The server decides
   *  it from the artifacts those sessions produce (the profile details and the
   *  ideal-life vision); the name predates that and is kept because it is what the API
   *  returns. */
  inFirstJourney?: boolean
  /** When the three consents were given, or null/absent if never. Timestamps rather than
   *  booleans because the backend stores them as nullable `time.Time` — the moment of
   *  consent is the auditable fact, not just that it happened.
   *
   *  aiAcceptedAt is the one the app gates on: App Review rejected 1.0 under 5.1.1(i) for
   *  sending voice audio to Google's AI without asking first, so nothing may reach a
   *  session until it is set. See src/consent/aiConsent.ts for why the read is tolerant
   *  about the exact spelling. */
  termsAndConditionsAcceptedAt?: string | null
  marketingAcceptedAt?: string | null
  aiAcceptedAt?: string | null
  createdAt?: string
}

/** The closed set PATCH /me accepts for chatHistoryRetentionDays — mirrors
 *  models.ChatRetentionOptions. Ascending, with 0 ("no limit") last: it is the
 *  only value that gives up the guarantee, so it should not sit at the head of
 *  the list looking like the safe default. Nothing below 3 is offered because
 *  the companion replays the last 72 hours and would lose the thread mid-chat. */
export const CHAT_RETENTION_OPTIONS = [3, 7, 30, 0] as const

export type MemoryCategory = 'identity' | 'values' | 'needs' | 'context' | 'obstacles' | 'insight'

export interface Memory {
  id: string
  userId: string
  category: MemoryCategory
  content: string
  createdAt: string
}

export interface PaginationInfo {
  currentPage: number
  totalPages: number
  totalItems: number
  itemsPerPage: number
}

export interface MemoryPaginatedResponse {
  items: Memory[]
  pagination: PaginationInfo
}

export type SessionType = 'onboarding' | 'session_vision' | 'checkin' | 'checkin_daily' | 'session_movement' | 'session_values' | 'session_energy' | 'session_decisions' | 'session_beliefs' | 'session_identity' | 'session_acceptance' | 'session_priorities';

export interface CommunicationSession {
  id: string;
  startTime: string;
  duration?: number;
  sessionType: string;
  userSessionInsight?: string | null;
  userEvaluation?: number | null;
  userFeedback?: string | null;
}

export interface UserSessionPaginatedResponse {
  items: CommunicationSession[];
  pagination: PaginationInfo;
}

/** GET /me/transactions — the minutes bank's ledger, bank-statement style,
 *  newest first. Every row is server-owned money math: render amountSeconds
 *  and balanceAfter, never recompute either. With ?grouped=true, consecutive
 *  message debits on the same calendar day fold into one row (send X-Timezone,
 *  IANA name, so the day boundary follows the user's calendar, not UTC). */
export type BalanceTransactionType =
  | 'subscription'
  | 'top_up'
  | 'session_usage'
  | 'session_free'
  | 'message_usage'
  | 'refund'

export interface BalanceTransaction {
  /** For a grouped message row, the id of the newest debit in the run. */
  id: string
  type: BalanceTransactionType
  /** Signed: positive = credit, negative = debit. Always 0 for session_free —
   *  the row exists so the history accounts for the session, not to move
   *  money. For a grouped message row, the run's debits summed. */
  amountSeconds: number
  /** The balance after this row applied (after the newest debit, for a
   *  grouped message row). */
  balanceAfter: number
  createdAt: string
  /** session_usage / session_free rows only. */
  sessionId?: string
  sessionType?: string
  /** Plan/package label for purchase credits and refunds. */
  product?: string
  /** Billing cadence of a subscription credit, derived server-side from the
   *  product id — translate this, never parse `product`. Subscription rows
   *  only, and absent when the product names neither cadence (or the server
   *  predates the field). */
  plan?: 'monthly' | 'annual'
  description?: string
  /** Grouped message rows only: the run's calendar day (YYYY-MM-DD in the
   *  request's X-Timezone) and how many charged replies it collapses. */
  day?: string
  messageCount?: number
}

export interface BalanceTransactionPaginatedResponse {
  items: BalanceTransaction[]
  pagination: PaginationInfo
}

export type QuoteType = 'growth' | 'resilience' | 'action' | 'mindset' | 'wisdom' | 'purpose';

export interface Quote {
  id: string;
  quote: string;
  author: string;
  category: QuoteType;
}

export interface NextSessionInfo {
  session: SessionType;
  availableAt: string;
}

export interface JourneyData {
  session: SessionType;
  mode: 'start' | 'resume';
  quote: Quote;
  /** Only present while the AI's end-of-session pick should override the user's
   *  own theme choice. Absent means: leave the current theme alone. */
  theme?: string;
  nextSession?: NextSessionInfo | null;
  /** Upcoming deep sessions on the user's growth journey and their estimated
   *  availability dates. May be null or empty when there are no future sessions. */
  sessions?: NextSessionInfo[] | null;
  focusArea?: string | null;
  /** @deprecated The backend no longer returns these on /journey. Commitments
   *  are their own resource (GET /commitments) and the streak lives on GET /streak. */
  badgesEarned?: number;
  /** @deprecated see badgesEarned — use GET /streak. */
  streak?: number;
}

export interface Action {
  id: string
  title: string
  type: 'one_time' | 'recurring'
  origin?: 'plan' | 'manual' | 'behavior'
  status: 'pending' | 'completed' | 'overdue'
  days?: number[]
  date?: string
}

// GET /me/profile — profile screen data (see rumicoach-backend api/openapi.yaml)
/** Profile badges, in the order the grid shows them.
 *
 *  All 15 are in the backend enum (api/openapi.yaml) and awarded by
 *  internal/services/badge — kept in the same order there so the two read
 *  side by side. `goalReached` (graduated behaviour plan) is still awarded by
 *  the server but has no tile here, so the grid silently ignores it. */
export type BadgeType =
  | 'firstSession'
  | 'visionSet'
  | 'firstCommitment'
  | 'firstDeepSession'
  | 'alwaysWithYou'
  | 'threeDayStreak'
  | 'tenInsights'
  | 'allThemesExplored'
  | 'sevenDayStreak'
  | 'twentySessions'
  | 'twentyFiveCommitments'
  | 'thirtyDayStreak'
  | 'wheelRemapped'
  | 'areaImproved'
  | 'hundredSessions'

export interface ProfileVision {
  text?: string | null
  craftedAt?: string | null
}

export interface ProfileBalance {
  completedAt?: string | null
  /** JSON-encoded array of WheelOfLifeItem ({ name, currentScore, reasoning }) */
  data?: string | null
}

export interface ProfileProgress {
  currentStreak: number
  bestStreak: number
  totalSessions: number
  hoursWithRumi: number
  insightsDiscovered: number
  commitmentsKept: number
}

export interface ProfileBadge {
  type: BadgeType
  earnedAt: string
}

export interface UserProfileResponse {
  focusArea?: string
  vision?: ProfileVision
  lifeBalance?: ProfileBalance
  progress?: ProfileProgress
  badges?: ProfileBadge[]
}

export interface SessionCard {
  type: string
  title: string
  subtitle: string
  action: string
}

export interface WheelOfLifeEntry {
  id: string
  version: number
  data: Record<string, number>
  createdAt: string
}

export interface WheelCategory {
  name: string
  currentScore: number
  targetScore?: number
  reasoning?: string
}

export interface WheelOfLifeExercise {
  id: string
  data: WheelCategory[]
  createdAt: string
}

export interface EisenhowerEntry {
  id: string
  version: number
  data: {
    urgentImportant: string[]
    urgentNotImportant: string[]
    notUrgentImportant: string[]
    notUrgentNotImportant: string[]
  }
  createdAt: string
}

export interface EisenhowerTask {
  task: string
  quadrant: string
  reasoning: string
}

export interface EisenhowerMatrixData {
  urgent_important: EisenhowerTask[]
  urgent_not_important: EisenhowerTask[]
  not_urgent_important: EisenhowerTask[]
  not_urgent_not_important: EisenhowerTask[]
}

export interface EisenhowerMatrixExercise {
  id: string
  data: EisenhowerMatrixData
  createdAt: string
}

/** DELETE /me/data?scope= — which slice of the user's data to erase.
 *  Omitting the parameter means 'all', so older callers keep working.
 *  Each scope is one server-side transaction that also repairs the state
 *  derived from what it deletes (badges, streak, habit plans) — see
 *  rumicoach-backend/docs/delete-by-category.md. */
export type DataScope = 'memories' | 'chat' | 'commitments' | 'progress' | 'all'

export type IntegrationProvider = 'whatsapp' | 'telegram'

/** Mirrors models.Integration{Pending,Active,Revoked} in the backend.
 *  GET /me/integrations filters `revoked` out, so the list screen only ever
 *  sees pending/active — but a fetch by id can still return a revoked row. */
export type IntegrationStatus = 'pending' | 'active' | 'revoked'

/** How Rumi answers messages on a channel (ChannelUpdateRequest.replyMode). */
export type ReplyMode = 'text' | 'audio' | 'auto'

export interface Integration {
  id: string
  provider: IntegrationProvider
  status: IntegrationStatus
  /** Masked phone/chat id ("+3519•••••78") — only set once linked. */
  maskedExternalId?: string | null
  replyMode?: ReplyMode | null
  createdAt: string
}

/** POST /me/integrations/{provider}/link — one-time code, valid for 15 minutes. */
export interface ChannelLinkResponse {
  code: string
  /** Deep link that pre-fills the code. Named for WhatsApp (`wa.me/…?text=`),
   *  but the Telegram endpoint returns its `t.me/<bot>?start=…` link in the
   *  same field — Telegram's `?start=` hands the code to the bot on its own. */
  waLink: string
  expiresAt: string
}

export interface UsageCalendarResponse {
  dayStreak?: number;
  sessionsCount?: number;
  hours?: number;
  days?: Record<string, {
    date?: string;
    kind?: 'session' | 'checkin' | 'none';
    sessions?: CommunicationSession[];
  }>;
}
