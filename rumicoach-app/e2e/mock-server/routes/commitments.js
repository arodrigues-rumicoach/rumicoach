import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createCommitmentsRouter(store) {
  const router = createRouter()

  router.get('/commitments', requireAuth(store), (req, res) => {
    initStore(store)
    const userCommitments = store.commitments.filter(c => c.userId === req.userId)
    res.json(userCommitments)
  })

  router.patch('/commitments/:id', requireAuth(store), (req, res) => {
    initStore(store)
    const action = store.commitments.find(c => c.id === req.params.id && c.userId === req.userId)
    if (!action) {
      return res.status(404).json({ code: 'NOT_FOUND', error: 'Commitment not found' })
    }

    // The real API (UpdateCommitmentRequest) toggles completion via `done`;
    // `status` is derived server-side. Keep accepting `status` for older fixtures.
    if (typeof req.body.done === 'boolean') {
      action.status = req.body.done ? 'completed' : 'pending'
    }

    if (req.body.status) {
      const validStatuses = ['pending', 'completed', 'overdue']
      if (!validStatuses.includes(req.body.status)) {
        return res.status(400).json({ code: 'INVALID_PAYLOAD', error: `Invalid status: ${req.body.status}` })
      }
      action.status = req.body.status
    }

    res.json(action)
  })

  return router
}
