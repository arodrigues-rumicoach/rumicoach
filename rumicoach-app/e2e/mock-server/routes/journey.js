import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

const quotes = [
  { id: 'q1', quote: 'The only way to do great work is to love what you do.', author: 'Steve Jobs', category: 'passion' },
  { id: 'q2', quote: 'It does not matter how slowly you go as long as you do not stop.', author: 'Confucius', category: 'perseverance' },
  { id: 'q3', quote: 'Believe you can and you\'re halfway there.', author: 'Theodore Roosevelt', category: 'mindset' },
  { id: 'q4', quote: 'Growth begins at the end of your comfort zone.', author: 'Unknown', category: 'growth' },
  { id: 'q5', quote: 'The best time to plant a tree was 20 years ago. The second best time is now.', author: 'Chinese proverb', category: 'action' },
]

export default function createJourneyRouter(store) {
  const router = createRouter()

  router.get('/journey', requireAuth(store), (req, res) => {
    initStore(store)
    const user = store.users.get(req.userId)
    if (!user) {
      return res.status(404).json({ code: 'ACCOUNT_NOT_FOUND', error: 'User not found' })
    }

    const userSessions = []
    for (const s of store.sessions.values()) {
      if (s.userId === req.userId) userSessions.push(s)
    }

    const userMemories = store.memories.filter(m => m.userId === req.userId)
    const userCommitments = store.commitments.filter(c => c.userId === req.userId)
    const streak = store.streakData[req.userId] || { currentStreak: 0, bestStreak: 0, lastSessionDate: null }

    const lastSession = userSessions[userSessions.length - 1]
    const sessionType = lastSession?.sessionType || 'checkin'
    const mode = lastSession ? 'resume' : 'start'

    const quote = quotes[Math.floor(Math.random() * quotes.length)]

    const completedCommitments = userCommitments.filter(c => c.status === 'completed').length
    const badgesEarned = Math.min(completedCommitments, 10)

    res.json({
      session: sessionType,
      mode,
      quote,
      nextSession: { session: 'checkin', availableAt: new Date(Date.now() + 86400000).toISOString() },
      sessions: [
        { session: 'session_vision', availableAt: new Date(Date.now() + 3 * 86400000).toISOString() },
        { session: 'session_values', availableAt: new Date(Date.now() + 6 * 86400000).toISOString() },
      ],
      focusArea: user.focusArea || null,
      badgesEarned,
      streak: streak.currentStreak,
    })
  })

  return router
}
