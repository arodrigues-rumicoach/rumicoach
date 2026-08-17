import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createStreakRouter(store) {
  const router = createRouter()

  router.get('/streak', requireAuth(store), (req, res) => {
    initStore(store)
    const data = store.streakData[req.userId] || { currentStreak: 0, bestStreak: 0, lastSessionDate: null }
    res.json({
      currentStreak: data.currentStreak,
      bestStreak: data.bestStreak,
      lastSessionDate: data.lastSessionDate,
    })
  })

  return router
}
