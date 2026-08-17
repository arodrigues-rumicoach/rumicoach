import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'
import { profileData } from '../fixtures/test-users.js'

export default function createUserRouter(store) {
  const router = createRouter()

  router.get('/me', requireAuth(store), (req, res) => {
    initStore(store)
    const user = store.users.get(req.userId)
    if (!user) {
      return res.status(404).json({ code: 'ACCOUNT_NOT_FOUND', error: 'User not found' })
    }
    const { accessToken, refreshToken, ...safeUser } = user
    res.json(safeUser)
  })

  router.patch('/me', requireAuth(store), (req, res) => {
    initStore(store)
    const user = store.users.get(req.userId)
    if (!user) {
      return res.status(404).json({ code: 'ACCOUNT_NOT_FOUND', error: 'User not found' })
    }

    const allowed = ['name', 'avatarUrl', 'coach', 'dateOfBirth', 'gender', 'coachGender',
      'phoneNumber', 'country', 'preferredLanguage', 'wheelOfLife',
      'theme', 'focusArea', 'visualizerType', 'coachVoice', 'chatHistoryRetentionDays']

    for (const [key, value] of Object.entries(req.body)) {
      if (allowed.includes(key)) {
        user[key] = value
      }
    }

    const { accessToken, refreshToken, ...safeUser } = user
    res.json(safeUser)
  })

  router.delete('/me', requireAuth(store), (req, res) => {
    initStore(store)
    store.users.delete(req.userId)
    delete store.streakData[req.userId]
    delete store.usageCalendar[req.userId]
    store.memories = store.memories.filter(m => m.userId !== req.userId)
    store.commitments = store.commitments.filter(c => c.userId !== req.userId)
    store.wheelOfLifeEntries = store.wheelOfLifeEntries.filter(w => w.userId !== req.userId)
    store.eisenhowerEntries = store.eisenhowerEntries.filter(e => e.userId !== req.userId)
    store.integrations = store.integrations.filter(i => i.userId !== req.userId)
    for (const [sid] of store.sessions) {
      const s = store.sessions.get(sid)
      if (s.userId === req.userId) store.sessions.delete(sid)
    }
    res.status(200).json({ deleted: true })
  })

  router.delete('/me/data', requireAuth(store), (req, res) => {
    initStore(store)
    const scope = req.query.scope || 'all'
    const validScopes = ['memories', 'chat', 'commitments', 'progress', 'all']
    if (!validScopes.includes(scope)) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: `Invalid scope: ${scope}` })
    }

    if (scope === 'memories' || scope === 'all') {
      store.memories = store.memories.filter(m => m.userId !== req.userId)
    }
    if (scope === 'chat' || scope === 'all') {
      for (const [sid] of store.sessions) {
        const s = store.sessions.get(sid)
        if (s.userId === req.userId) store.sessions.delete(sid)
      }
    }
    if (scope === 'commitments' || scope === 'all') {
      store.commitments = store.commitments.filter(c => c.userId !== req.userId)
    }
    if (scope === 'progress' || scope === 'all') {
      store.wheelOfLifeEntries = store.wheelOfLifeEntries.filter(w => w.userId !== req.userId)
      store.eisenhowerEntries = store.eisenhowerEntries.filter(e => e.userId !== req.userId)
      store.streakData[req.userId] = { currentStreak: 0, bestStreak: 0, lastSessionDate: null }
      store.usageCalendar[req.userId] = { dayStreak: 0, sessionsCount: 0, hours: 0, days: {} }
    }

    res.status(200).json({ deleted: true, scope })
  })

  router.get('/me/profile', requireAuth(store), (req, res) => {
    initStore(store)
    const user = store.users.get(req.userId)
    if (!user) {
      return res.status(404).json({ code: 'ACCOUNT_NOT_FOUND', error: 'User not found' })
    }
    const profile = profileData[req.userId] || {
      focusArea: user.focusArea || null,
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
    }
    res.json({ name: user.name, email: user.email, ...profile })
  })

  return router
}
