import { v4 as uuidv4 } from 'uuid'
import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createIntegrationsRouter(store) {
  const router = createRouter()

  router.get('/integrations', requireAuth(store), (req, res) => {
    initStore(store)
    const userIntegrations = store.integrations
      .filter(i => i.userId === req.userId && (i.status === 'pending' || i.status === 'active'))
    res.json(userIntegrations)
  })

  router.post('/integrations/:provider/link', requireAuth(store), (req, res) => {
    initStore(store)
    const provider = req.params.provider

    if (provider !== 'whatsapp' && provider !== 'telegram') {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: `Unsupported provider: ${provider}` })
    }

    const code = uuidv4().replace(/-/g, '').substring(0, 12)
    const expiresAt = new Date(Date.now() + 15 * 60 * 1000).toISOString()

    const integration = {
      id: uuidv4(),
      userId: req.userId,
      provider,
      status: 'pending',
      maskedExternalId: null,
      replyMode: 'auto',
      createdAt: new Date().toISOString(),
      linkCode: code,
      expiresAt,
    }

    store.integrations.push(integration)

    const waLink = provider === 'whatsapp'
      ? `https://wa.me/RumiCoach?text=${code}`
      : `https://t.me/RumiCoachBot?start=${code}`

    res.status(201).json({ code, waLink, expiresAt })
  })

  router.delete('/integrations/:id', requireAuth(store), (req, res) => {
    initStore(store)
    const idx = store.integrations.findIndex(i => i.id === req.params.id && i.userId === req.userId)
    if (idx === -1) {
      return res.status(404).json({ code: 'NOT_FOUND', error: 'Integration not found' })
    }

    store.integrations.splice(idx, 1)
    res.status(200).json({ deleted: true })
  })

  return router
}
