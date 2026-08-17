import { v4 as uuidv4 } from 'uuid'
import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createWheelOfLifeRouter(store) {
  const router = createRouter()

  router.get('/wheel-of-life', requireAuth(store), (req, res) => {
    initStore(store)
    const entries = store.wheelOfLifeEntries
      .filter(e => e.userId === req.userId)
      .sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt))
    res.json(entries)
  })

  router.post('/wheel-of-life', requireAuth(store), (req, res) => {
    initStore(store)
    const { data } = req.body

    if (!data || typeof data !== 'object') {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'data object is required' })
    }

    const userEntries = store.wheelOfLifeEntries.filter(e => e.userId === req.userId)
    const latestVersion = userEntries.length > 0
      ? Math.max(...userEntries.map(e => e.version)) + 1
      : 1

    const entry = {
      id: uuidv4(),
      userId: req.userId,
      version: latestVersion,
      data,
      createdAt: new Date().toISOString(),
    }

    store.wheelOfLifeEntries.push(entry)
    res.status(201).json(entry)
  })

  return router
}
