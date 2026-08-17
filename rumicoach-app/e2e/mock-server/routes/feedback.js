import { v4 as uuidv4 } from 'uuid'
import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createFeedbackRouter(store) {
  const router = createRouter()

  router.post('/chat/sessions/:id/feedback', requireAuth(store), (req, res) => {
    initStore(store)
    const { id } = req.params
    const { evaluation, feedback } = req.body

    const session = store.sessions.get(id)
    if (!session || session.userId !== req.userId) {
      return res.status(404).json({ code: 'NOT_FOUND', error: 'Session not found' })
    }

    if (evaluation !== undefined) {
      if (typeof evaluation !== 'number' || evaluation < 1 || evaluation > 5) {
        return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'evaluation must be a number 1-5' })
      }
      session.userEvaluation = evaluation
    }

    if (feedback !== undefined) {
      session.userFeedback = feedback
    }

    store.sessions.set(id, session)
    res.json({ id, evaluation: session.userEvaluation, feedback: session.userFeedback })
  })

  router.post('/feedback', requireAuth(store), (req, res) => {
    initStore(store)
    const { category, description, screenshot } = req.body

    if (!category) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'category is required' })
    }

    const validCategories = ['bug', 'feature', 'improvement', 'other']
    if (!validCategories.includes(category)) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: `Invalid category: ${category}` })
    }

    const feedbackEntry = {
      id: uuidv4(),
      userId: req.userId,
      category,
      description: description || '',
      screenshot: screenshot || null,
      createdAt: new Date().toISOString(),
    }

    if (!store.feedback) store.feedback = []
    store.feedback.push(feedbackEntry)

    res.status(201).json({ id: feedbackEntry.id, received: true })
  })

  return router
}
