import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createSessionsRouter(store) {
  const router = createRouter()

  router.get('/sessions', requireAuth(store), (req, res) => {
    initStore(store)

    const page = Math.max(1, parseInt(req.query.page) || 1)
    const limit = Math.max(1, Math.min(100, parseInt(req.query.limit) || 20))

    const userSessions = []
    for (const s of store.sessions.values()) {
      if (s.userId === req.userId) userSessions.push(s)
    }

    userSessions.sort((a, b) => new Date(b.startTime) - new Date(a.startTime))

    const totalItems = userSessions.length
    const totalPages = Math.max(1, Math.ceil(totalItems / limit))
    const startIndex = (page - 1) * limit
    const endIndex = startIndex + limit
    const items = userSessions.slice(startIndex, endIndex)

    res.json({
      items,
      pagination: {
        currentPage: page,
        totalPages,
        totalItems,
        itemsPerPage: limit,
      },
    })
  })

  return router
}
