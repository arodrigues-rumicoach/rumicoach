import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

// In-memory state for error simulation
const simulationState = {
  tokenExpiry: new Map(), // userId → { expiresAt }
  rateLimits: new Map(),  // route → { count, resetAt }
  serverErrors: new Map(), // route → { enabled, statusCode }
  delayedResponses: new Map(), // route → delayMs
}

export function getSimulationState() {
  return simulationState
}

export default function createSimulationRouter(store) {
  const router = createRouter()

  // ── Token Expiry Simulation ──────────────────────────────────────────────
  // Mark a user's token as expired on next request
  router.post('/simulate/token-expiry', requireAuth(store), (req, res) => {
    const { expiresAt } = req.body || {}
    simulationState.tokenExpiry.set(req.userId, {
      expiresAt: expiresAt || new Date(Date.now() + 1000).toISOString(),
    })
    res.json({ simulated: true, userId: req.userId })
  })

  // Clear token expiry simulation
  router.delete('/simulate/token-expiry', requireAuth(store), (req, res) => {
    simulationState.tokenExpiry.delete(req.userId)
    res.json({ cleared: true })
  })

  // ── Rate Limit Simulation ────────────────────────────────────────────────
  // Set rate limit for a route
  router.post('/simulate/rate-limit', requireAuth(store), (req, res) => {
    const { route, maxRequests, windowMs } = req.body || {}
    if (!route) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'route is required' })
    }
    simulationState.rateLimits.set(route, {
      count: 0,
      maxRequests: maxRequests || 5,
      resetAt: new Date(Date.now() + (windowMs || 60000)).toISOString(),
    })
    res.json({ simulated: true, route })
  })

  // Clear rate limit simulation
  router.delete('/simulate/rate-limit', requireAuth(store), (req, res) => {
    const { route } = req.body || {}
    if (route) {
      simulationState.rateLimits.delete(route)
    } else {
      simulationState.rateLimits.clear()
    }
    res.json({ cleared: true })
  })

  // ── Server Error Simulation ──────────────────────────────────────────────
  // Enable 500 errors on a specific route
  router.post('/simulate/server-error', requireAuth(store), (req, res) => {
    const { route, statusCode } = req.body || {}
    if (!route) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'route is required' })
    }
    simulationState.serverErrors.set(route, {
      enabled: true,
      statusCode: statusCode || 500,
    })
    res.json({ simulated: true, route, statusCode: statusCode || 500 })
  })

  // Disable server error simulation
  router.delete('/simulate/server-error', requireAuth(store), (req, res) => {
    const { route } = req.body || {}
    if (route) {
      simulationState.serverErrors.delete(route)
    } else {
      simulationState.serverErrors.clear()
    }
    res.json({ cleared: true })
  })

  // ── Delayed Response Simulation ──────────────────────────────────────────
  router.post('/simulate/delay', requireAuth(store), (req, res) => {
    const { route, delayMs } = req.body || {}
    if (!route) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'route is required' })
    }
    simulationState.delayedResponses.set(route, delayMs || 5000)
    res.json({ simulated: true, route, delayMs: delayMs || 5000 })
  })

  router.delete('/simulate/delay', requireAuth(store), (req, res) => {
    const { route } = req.body || {}
    if (route) {
      simulationState.delayedResponses.delete(route)
    } else {
      simulationState.delayedResponses.clear()
    }
    res.json({ cleared: true })
  })

  // ── Clear All Simulations ────────────────────────────────────────────────
  router.post('/simulate/clear-all', requireAuth(store), (req, res) => {
    simulationState.tokenExpiry.clear()
    simulationState.rateLimits.clear()
    simulationState.serverErrors.clear()
    simulationState.delayedResponses.clear()
    res.json({ cleared: true })
  })

  return router
}
