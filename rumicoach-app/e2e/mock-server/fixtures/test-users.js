import { v4 as uuidv4 } from 'uuid'

function base64UrlEncode(str) {
  return Buffer.from(str)
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')
}

export function createMockJwt(payload) {
  const header = base64UrlEncode(JSON.stringify({ alg: 'HS256' }))
  const payloadEncoded = base64UrlEncode(JSON.stringify(payload))
  return `${header}.${payloadEncoded}.fakesig`
}

// ─── Users ───────────────────────────────────────────────────────────────────

export const testUsers = {
  'test-user-eu-1': {
    id: 'test-user-eu-1',
    email: 'e2e-test@rumi.coach',
    // A natural name rather than "E2E Tester": this fixture is the one the store
    // screenshots are captured from, and the app greets the user by name on the Journey
    // screen and again on Profile. No test asserts on it.
    name: 'Laura',
    avatarUrl: null,
    coach: 'Empath',
    dateOfBirth: '1990-06-15',
    gender: 'male',
    coachGender: 'female',
    phoneNumber: '+351912345678',
    country: 'PT',
    preferredLanguage: 'en',
    idealLifeVisionSetAt: '2025-01-15T10:00:00Z',
    wheelOfLife: {
      health: 7,
      career: 6,
      relationships: 8,
      personal_growth: 5,
      finances: 4,
      hobbies: 7,
      environment: 6,
      spirituality: 3,
    },
    theme: 'forest',
    focusArea: 'career',
    visualizerType: 'default',
    coachVoice: 'gacrux',
    region: 'eu',
    chatHistoryRetentionDays: 7,
    // The real backend sends the voice balance on every full /me — the settings
    // usage screen hides its balance card without it. 55 minutes left.
    balanceSeconds: 3300,
    createdAt: '2024-01-15T10:00:00Z',
  },
  'test-user-eu-2': {
    id: 'test-user-eu-2',
    email: 'maria@rumi.coach',
    name: 'Maria Silva',
    avatarUrl: null,
    coach: 'Strategist',
    dateOfBirth: '1985-03-22',
    gender: 'female',
    coachGender: 'male',
    phoneNumber: '+351987654321',
    country: 'PT',
    preferredLanguage: 'pt',
    idealLifeVisionSetAt: '2025-01-15T10:00:00Z',
    wheelOfLife: {
      health: 5,
      career: 8,
      relationships: 6,
      personal_growth: 7,
      finances: 6,
      hobbies: 4,
      environment: 5,
      spirituality: 6,
    },
    theme: 'ocean',
    focusArea: 'health',
    visualizerType: 'default',
    coachVoice: 'algieba',
    region: 'eu',
    chatHistoryRetentionDays: 30,
    createdAt: '2024-03-01T08:00:00Z',
  },
  'test-user-us-1': {
    id: 'test-user-us-1',
    email: 'john@rumi.coach',
    name: 'John Doe',
    avatarUrl: null,
    coach: 'Commander',
    dateOfBirth: '1992-11-08',
    gender: 'male',
    coachGender: 'female',
    phoneNumber: '+14155551234',
    country: 'US',
    preferredLanguage: 'en',
    idealLifeVisionSetAt: '2025-01-15T10:00:00Z',
    wheelOfLife: {
      health: 4,
      career: 7,
      relationships: 5,
      personal_growth: 6,
      finances: 8,
      hobbies: 3,
      environment: 4,
      spirituality: 5,
    },
    theme: 'sunset',
    focusArea: 'relationships',
    visualizerType: 'default',
    coachVoice: 'enceladus',
    region: 'us',
    chatHistoryRetentionDays: 7,
    createdAt: '2024-02-20T14:00:00Z',
  },
  'test-user-new': {
    id: 'test-user-new',
    email: 'newuser@rumi.coach',
    name: 'New User',
    avatarUrl: null,
    coach: 'Buddy',
    dateOfBirth: null,
    gender: null,
    coachGender: null,
    phoneNumber: null,
    country: null,
    preferredLanguage: 'en',
    idealLifeVisionSetAt: null,
    wheelOfLife: null,
    theme: 'light',
    focusArea: null,
    visualizerType: 'default',
    coachVoice: null,
    region: 'eu',
    chatHistoryRetentionDays: 7,
    createdAt: null,
  },
  'test-user-eu-admin': {
    id: 'test-user-eu-admin',
    email: 'admin@rumi.coach',
    name: 'Admin User',
    avatarUrl: null,
    coach: 'Empath',
    dateOfBirth: '1985-01-01',
    gender: 'male',
    coachGender: 'female',
    phoneNumber: '+351900000000',
    country: 'PT',
    preferredLanguage: 'en',
    idealLifeVisionSetAt: '2025-01-15T10:00:00Z',
    wheelOfLife: {
      health: 8,
      career: 8,
      relationships: 8,
      personal_growth: 8,
      finances: 8,
      hobbies: 8,
      environment: 8,
      spirituality: 8,
    },
    theme: 'dark',
    focusArea: 'career',
    visualizerType: 'default',
    coachVoice: 'gacrux',
    region: 'eu',
    chatHistoryRetentionDays: 30,
    createdAt: '2024-01-01T00:00:00Z',
  },
}

// ─── Verification Codes ──────────────────────────────────────────────────────

export const verificationCodes = {
  'email:test-user-eu-1:login': { code: '123456', type: 'email', event: 'login', email: 'e2e-test@rumi.coach' },
  'email:test-user-eu-1:signup': { code: '654321', type: 'email', event: 'signup', email: 'e2e-test@rumi.coach' },
  'phone:test-user-eu-1:login': { code: '111111', type: 'phone', event: 'login', phoneNumber: '+351912345678' },
  'phone:test-user-eu-1:signup': { code: '222222', type: 'phone', event: 'signup', phoneNumber: '+351912345678' },
  'email:test-user-eu-2:login': { code: '333333', type: 'email', event: 'login', email: 'maria@rumi.coach' },
  'email:test-user-eu-2:signup': { code: '444444', type: 'email', event: 'signup', email: 'maria@rumi.coach' },
  'phone:test-user-us-1:login': { code: '555555', type: 'phone', event: 'login', phoneNumber: '+14155551234' },
  'phone:test-user-us-1:signup': { code: '666666', type: 'phone', event: 'signup', phoneNumber: '+14155551234' },
  'email:test-user-new:signup': { code: '777777', type: 'email', event: 'signup', email: 'newuser@rumi.coach' },
}

// ─── Mock Tokens ─────────────────────────────────────────────────────────────

export const mockTokens = {
  'test-user-eu-1': {
    accessToken: createMockJwt({
      sub: 'test-user-eu-1',
      region: 'eu',
      is_admin: false,
      iat: 1700000000,
    }),
    refreshToken: 'mock_refresh_token_eu_abc123',
  },
  'test-user-eu-2': {
    accessToken: createMockJwt({
      sub: 'test-user-eu-2',
      region: 'eu',
      is_admin: false,
      iat: 1700000000,
    }),
    refreshToken: 'mock_refresh_token_eu2_def456',
  },
  'test-user-us-1': {
    accessToken: createMockJwt({
      sub: 'test-user-us-1',
      region: 'us',
      is_admin: false,
      iat: 1700000000,
    }),
    refreshToken: 'mock_refresh_token_us_xyz789',
  },
  'test-user-new': {
    accessToken: createMockJwt({
      sub: 'test-user-new',
      region: 'eu',
      is_admin: false,
      iat: 1700000000,
    }),
    refreshToken: 'mock_refresh_token_new_abc123',
  },
  'test-user-eu-admin': {
    accessToken: createMockJwt({
      sub: 'test-user-eu-admin',
      region: 'eu',
      is_admin: true,
      iat: 1700000000,
    }),
    refreshToken: 'mock_refresh_token_admin_abc123',
  },
}

// ─── Profile Data ────────────────────────────────────────────────────────────

export const profileData = {
  'test-user-eu-1': {
    focusArea: 'career',
    vision: {
      text: 'Become a leader in tech',
      craftedAt: '2024-06-01T00:00:00Z',
    },
    lifeBalance: {
      completedAt: '2024-06-01T00:00:00Z',
      data: JSON.stringify([
        { name: 'Health', currentScore: 7, targetScore: 9, reasoning: 'Improve fitness' },
        { name: 'Career', currentScore: 6, targetScore: 9, reasoning: 'Reach senior level' },
        { name: 'Relationships', currentScore: 8, targetScore: 9, reasoning: 'Deepen connections' },
        { name: 'Personal Growth', currentScore: 5, targetScore: 8, reasoning: 'Read more' },
        { name: 'Finances', currentScore: 4, targetScore: 7, reasoning: 'Build savings' },
        { name: 'Hobbies', currentScore: 7, targetScore: 8, reasoning: 'Explore new interests' },
        { name: 'Environment', currentScore: 6, targetScore: 8, reasoning: 'Live sustainably' },
        { name: 'Spirituality', currentScore: 3, targetScore: 6, reasoning: 'Find inner peace' },
      ]),
    },
    progress: {
      currentStreak: 5,
      bestStreak: 14,
      totalSessions: 23,
      hoursWithRumi: 12.5,
      insightsDiscovered: 18,
      commitmentsKept: 12,
    },
    badges: [
      { type: 'firstSession', earnedAt: '2024-01-16T00:00:00Z' },
      { type: 'visionSet', earnedAt: '2024-06-01T00:00:00Z' },
      { type: 'firstCommitment', earnedAt: '2024-01-20T00:00:00Z' },
      { type: 'threeDayStreak', earnedAt: '2024-01-19T00:00:00Z' },
      { type: 'tenInsights', earnedAt: '2024-05-15T00:00:00Z' },
    ],
  },
  'test-user-eu-2': {
    focusArea: 'health',
    vision: {
      text: 'Live a balanced life',
      craftedAt: '2024-04-01T00:00:00Z',
    },
    lifeBalance: {
      completedAt: '2024-04-01T00:00:00Z',
      data: JSON.stringify([
        { name: 'Health', currentScore: 5, targetScore: 8 },
        { name: 'Career', currentScore: 8, targetScore: 9 },
        { name: 'Relationships', currentScore: 6, targetScore: 8 },
        { name: 'Personal Growth', currentScore: 7, targetScore: 8 },
        { name: 'Finances', currentScore: 6, targetScore: 7 },
        { name: 'Hobbies', currentScore: 4, targetScore: 7 },
        { name: 'Environment', currentScore: 5, targetScore: 7 },
        { name: 'Spirituality', currentScore: 6, targetScore: 8 },
      ]),
    },
    progress: {
      currentStreak: 2,
      bestStreak: 7,
      totalSessions: 10,
      hoursWithRumi: 5.2,
      insightsDiscovered: 8,
      commitmentsKept: 5,
    },
    badges: [
      { type: 'firstSession', earnedAt: '2024-03-02T00:00:00Z' },
      { type: 'visionSet', earnedAt: '2024-04-01T00:00:00Z' },
      { type: 'firstCommitment', earnedAt: '2024-03-05T00:00:00Z' },
    ],
  },
  'test-user-us-1': {
    focusArea: 'relationships',
    vision: null,
    lifeBalance: null,
    progress: {
      currentStreak: 0,
      bestStreak: 3,
      totalSessions: 5,
      hoursWithRumi: 2.8,
      insightsDiscovered: 3,
      commitmentsKept: 2,
    },
    badges: [
      { type: 'firstSession', earnedAt: '2024-02-21T00:00:00Z' },
    ],
  },
  'test-user-new': {
    focusArea: null,
    vision: null,
    lifeBalance: null,
    progress: {
      currentStreak: 0,
      bestStreak: 0,
      totalSessions: 0,
      hoursWithRumi: 0,
      insightsDiscovered: 0,
      commitmentsKept: 0,
    },
    badges: [],
  },
}

// ─── Memories ────────────────────────────────────────────────────────────────

export const memories = [
  // test-user-eu-1: 5 memories across different categories
  {
    id: 'mem-eu1-identity-001',
    userId: 'test-user-eu-1',
    category: 'identity',
    content: 'I am a tech professional who values growth and continuous learning.',
    createdAt: '2024-01-20T10:30:00Z',
  },
  {
    id: 'mem-eu1-values-001',
    userId: 'test-user-eu-1',
    category: 'values',
    content: 'Integrity and empathy are the core values I want to embody in my work and relationships.',
    createdAt: '2024-02-15T14:00:00Z',
  },
  {
    id: 'mem-eu1-needs-001',
    userId: 'test-user-eu-1',
    category: 'needs',
    content: 'I need more structure in my daily routine to feel grounded.',
    createdAt: '2024-03-10T09:15:00Z',
  },
  {
    id: 'mem-eu1-context-001',
    userId: 'test-user-eu-1',
    category: 'context',
    content: 'Currently leading a team of 8 engineers and transitioning to a management role.',
    createdAt: '2024-04-05T16:45:00Z',
  },
  {
    id: 'mem-eu1-insight-001',
    userId: 'test-user-eu-1',
    category: 'insight',
    content: 'I realized that delegating more effectively would free up time for strategic thinking.',
    createdAt: '2024-05-20T11:00:00Z',
  },
  // test-user-eu-2: 3 memories
  {
    id: 'mem-eu2-identity-001',
    userId: 'test-user-eu-2',
    category: 'identity',
    content: 'I am a creative person who thrives in collaborative environments.',
    createdAt: '2024-03-05T12:00:00Z',
  },
  {
    id: 'mem-eu2-values-001',
    userId: 'test-user-eu-2',
    category: 'values',
    content: 'Balance and well-being are more important to me than career advancement.',
    createdAt: '2024-04-10T10:00:00Z',
  },
  {
    id: 'mem-eu2-obstacles-001',
    userId: 'test-user-eu-2',
    category: 'obstacles',
    content: 'I struggle with setting boundaries and saying no to others.',
    createdAt: '2024-05-01T15:30:00Z',
  },
]

// ─── Commitments ─────────────────────────────────────────────────────────────

export const commitments = [
  {
    id: 'act-eu1-pending-001',
    userId: 'test-user-eu-1',
    title: 'Schedule a career mentoring session',
    type: 'one_time',
    origin: 'plan',
    status: 'pending',
    date: '2024-07-15',
  },
  {
    id: 'act-eu1-completed-001',
    userId: 'test-user-eu-1',
    title: 'Read "Atomic Habits"',
    type: 'one_time',
    origin: 'manual',
    status: 'completed',
    date: '2024-05-01',
  },
  {
    id: 'act-eu1-overdue-001',
    userId: 'test-user-eu-1',
    title: 'Weekly team 1-on-1s',
    type: 'recurring',
    origin: 'behavior',
    status: 'overdue',
    days: [1, 3, 5],
  },
]

// ─── Sessions ────────────────────────────────────────────────────────────────

export const sessions = [
  {
    id: 'sess-eu1-onboarding-001',
    userId: 'test-user-eu-1',
    startTime: '2024-01-15T10:00:00Z',
    duration: 15,
    sessionType: 'onboarding',
    userSessionInsight: 'Discovered my primary focus area is career growth.',
    userEvaluation: 5,
    userFeedback: 'Great introduction to the app!',
  },
  {
    id: 'sess-eu1-checkin-001',
    userId: 'test-user-eu-1',
    startTime: '2024-01-16T08:00:00Z',
    duration: 8,
    sessionType: 'checkin',
    userSessionInsight: null,
    userEvaluation: 4,
    userFeedback: null,
  },
  {
    id: 'sess-eu1-vision-001',
    userId: 'test-user-eu-1',
    startTime: '2024-06-01T14:00:00Z',
    duration: 25,
    sessionType: 'session_vision',
    userSessionInsight: 'Clarified my vision to become a leader in tech.',
    userEvaluation: 5,
    userFeedback: 'Very insightful session.',
  },
  {
    id: 'sess-eu1-values-001',
    userId: 'test-user-eu-1',
    startTime: '2024-02-15T14:00:00Z',
    duration: 20,
    sessionType: 'session_values',
    userSessionInsight: 'Identified integrity and empathy as core values.',
    userEvaluation: 4,
    userFeedback: 'Helped me understand what matters most.',
  },
  {
    id: 'sess-eu1-beliefs-001',
    userId: 'test-user-eu-1',
    startTime: '2024-05-20T11:00:00Z',
    duration: 30,
    sessionType: 'session_beliefs',
    userSessionInsight: 'Recognized a limiting belief about delegation.',
    userEvaluation: 5,
    userFeedback: 'Eye-opening conversation.',
  },
]

// ─── Wheel of Life Entries ───────────────────────────────────────────────────

export const wheelOfLifeEntries = [
  {
    id: 'wol-eu1-v1',
    userId: 'test-user-eu-1',
    version: 1,
    data: {
      health: 6,
      career: 4,
      relationships: 7,
      personal_growth: 3,
      finances: 3,
      hobbies: 5,
      environment: 4,
      spirituality: 2,
    },
    createdAt: '2024-01-15T10:00:00Z',
  },
  {
    id: 'wol-eu1-v2',
    userId: 'test-user-eu-1',
    version: 2,
    data: {
      health: 7,
      career: 6,
      relationships: 8,
      personal_growth: 5,
      finances: 4,
      hobbies: 7,
      environment: 6,
      spirituality: 3,
    },
    createdAt: '2024-06-01T00:00:00Z',
  },
]

// ─── Eisenhower Matrix Entries ───────────────────────────────────────────────

export const eisenhowerEntries = [
  {
    id: 'eis-eu1-v1',
    userId: 'test-user-eu-1',
    version: 1,
    data: {
      urgentImportant: ['Prepare quarterly review presentation', 'Fix production bug'],
      urgentNotImportant: ['Reply to routine emails', 'Attend status meeting'],
      notUrgentImportant: ['Build mentoring program', 'Learn new framework'],
      notUrgentNotImportant: ['Organize desk', 'Update old documentation'],
    },
    createdAt: '2024-06-15T09:00:00Z',
  },
]

// ─── Streak Data ─────────────────────────────────────────────────────────────

export const streakData = {
  'test-user-eu-1': {
    currentStreak: 5,
    bestStreak: 14,
    lastSessionDate: '2024-07-10',
  },
  'test-user-eu-2': {
    currentStreak: 2,
    bestStreak: 7,
    lastSessionDate: '2024-07-09',
  },
  'test-user-us-1': {
    currentStreak: 0,
    bestStreak: 3,
    lastSessionDate: '2024-04-20',
  },
  'test-user-new': {
    currentStreak: 0,
    bestStreak: 0,
    lastSessionDate: null,
  },
}

// ─── Usage Calendar ──────────────────────────────────────────────────────────

function buildUsageDays() {
  const days = {}
  // Last 30 days from 2024-07-10
  const sessionDates = [
    '2024-07-10', '2024-07-09', '2024-07-08', '2024-07-07', '2024-07-06',
    '2024-07-03', '2024-07-01', '2024-06-28', '2024-06-25', '2024-06-20',
    '2024-06-15', '2024-06-10', '2024-06-05', '2024-06-01',
  ]
  const checkinDates = [
    '2024-07-05', '2024-07-02', '2024-06-29', '2024-06-26', '2024-06-22',
    '2024-06-18', '2024-06-12', '2024-06-08',
  ]

  for (let i = 0; i < 30; i++) {
    const date = new Date(Date.UTC(2024, 6, 10 - i))
    const dateStr = date.toISOString().slice(0, 10)

    if (sessionDates.includes(dateStr)) {
      days[dateStr] = { date: dateStr, kind: 'session', sessions: [] }
    } else if (checkinDates.includes(dateStr)) {
      days[dateStr] = { date: dateStr, kind: 'checkin', sessions: [] }
    } else {
      days[dateStr] = { date: dateStr, kind: 'none', sessions: [] }
    }
  }
  return days
}

export const usageCalendar = {
  'test-user-eu-1': {
    dayStreak: 5,
    sessionsCount: 23,
    hours: 12.5,
    days: buildUsageDays(),
  },
  'test-user-eu-2': {
    dayStreak: 2,
    sessionsCount: 10,
    hours: 5.2,
    days: buildUsageDays(),
  },
  'test-user-us-1': {
    dayStreak: 0,
    sessionsCount: 5,
    hours: 2.8,
    days: buildUsageDays(),
  },
  'test-user-new': {
    dayStreak: 0,
    sessionsCount: 0,
    hours: 0,
    days: buildUsageDays(),
  },
}

// ─── Integrations ────────────────────────────────────────────────────────────

export const integrations = [
  {
    id: 'int-eu1-whatsapp',
    userId: 'test-user-eu-1',
    provider: 'whatsapp',
    status: 'active',
    maskedExternalId: '+3519•••••78',
    replyMode: 'auto',
    createdAt: '2024-04-01T10:00:00Z',
  },
  {
    id: 'int-eu1-telegram',
    userId: 'test-user-eu-1',
    provider: 'telegram',
    status: 'pending',
    maskedExternalId: null,
    replyMode: 'text',
    createdAt: '2024-07-01T12:00:00Z',
  },
]
