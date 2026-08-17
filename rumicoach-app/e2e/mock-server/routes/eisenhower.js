import { v4 as uuidv4 } from 'uuid'
import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createEisenhowerRouter(store) {
  const router = createRouter()

  router.get('/eisenhower-matrix', requireAuth(store), (req, res) => {
    initStore(store)
    const entries = store.eisenhowerEntries
      .filter(e => e.userId === req.userId)
      .sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt))
    res.json(entries)
  })

  router.post('/eisenhower-matrix', requireAuth(store), (req, res) => {
    initStore(store)
    const { data } = req.body

    if (!data || typeof data !== 'object') {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'data object is required' })
    }

    const required = ['urgentImportant', 'urgentNotImportant', 'notUrgentImportant', 'notUrgentNotImportant']
    for (const key of required) {
      if (!Array.isArray(data[key])) {
        return res.status(400).json({ code: 'INVALID_PAYLOAD', error: `${key} must be an array` })
      }
    }

    const userEntries = store.eisenhowerEntries.filter(e => e.userId === req.userId)
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

    store.eisenhowerEntries.push(entry)
    res.status(201).json(entry)
  })

  return router
}
