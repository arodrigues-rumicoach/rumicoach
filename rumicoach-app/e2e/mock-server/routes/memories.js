import { v4 as uuidv4 } from 'uuid'
import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createMemoriesRouter(store) {
  const router = createRouter()

  router.get('/memories', requireAuth(store), (req, res) => {
    initStore(store)

    const category = req.query.category || null
    const page = Math.max(1, parseInt(req.query.page) || 1)
    const limit = Math.max(1, Math.min(100, parseInt(req.query.limit) || 20))

    let userMemories = store.memories.filter(m => m.userId === req.userId)
    if (category) {
      userMemories = userMemories.filter(m => m.category === category)
    }

    userMemories.sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt))

    const totalItems = userMemories.length
    const totalPages = Math.max(1, Math.ceil(totalItems / limit))
    const startIndex = (page - 1) * limit
    const endIndex = startIndex + limit
    const items = userMemories.slice(startIndex, endIndex)

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

  router.post('/memories', requireAuth(store), (req, res) => {
    initStore(store)
    const { category, content } = req.body

    if (!category || !content) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'category and content are required' })
    }

    const validCategories = ['identity', 'values', 'needs', 'context', 'obstacles', 'insight']
    if (!validCategories.includes(category)) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: `Invalid category: ${category}` })
    }

    const memory = {
      id: uuidv4(),
      userId: req.userId,
      category,
      content,
      createdAt: new Date().toISOString(),
    }

    store.memories.push(memory)
    res.status(201).json(memory)
  })

  router.delete('/memories/:id', requireAuth(store), (req, res) => {
    initStore(store)
    const idx = store.memories.findIndex(m => m.id === req.params.id && m.userId === req.userId)
    if (idx === -1) {
      return res.status(404).json({ code: 'NOT_FOUND', error: 'Memory not found' })
    }

    store.memories.splice(idx, 1)
    res.status(200).json({ deleted: true })
  })

  return router
}
