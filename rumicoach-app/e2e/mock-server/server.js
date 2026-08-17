/**
 * Mock API server for RumiCoach E2E testing.
 *
 * Uses Bun.serve() for native Bun compatibility. Express routers are adapted
 * via bun-adapter.js to work with Bun's Request/Response API.
 *
 * To route the app to this server, set:
 *   EXPO_PUBLIC_API_HOST=localhost:3001
 *   EXPO_PUBLIC_AUTH_BACKEND_URL=http://localhost:3001/v1
 */

import createAuthRouter from './routes/auth.js'
import createUserRouter from './routes/user.js'
import createJourneyRouter from './routes/journey.js'
import createSessionsRouter from './routes/sessions.js'
import createMemoriesRouter from './routes/memories.js'
import createCommitmentsRouter from './routes/commitments.js'
import createWheelOfLifeRouter from './routes/wheel-of-life.js'
import createEisenhowerRouter from './routes/eisenhower.js'
import createStreakRouter from './routes/streak.js'
import createUsageRouter from './routes/usage.js'
import createIntegrationsRouter from './routes/integrations.js'
import createFeedbackRouter from './routes/feedback.js'
import createSimulationRouter from './routes/simulation.js'
import { createWebSocketHandler, validateWsUpgrade } from './routes/websocket.js'
import { initStore } from './middleware.js'
import { bunReqToExpress, createExpressRes } from './bun-adapter.js'
import { runMiddlewareChain } from './bun-router.js'

// Shared in-memory state store
const store = {
  users: new Map(),
  sessions: new Map(),
  memories: [],
  commitments: [],
  wheelOfLifeEntries: [],
  eisenhowerEntries: [],
  verifications: new Map(),
  streakData: {},
  usageCalendar: {},
  integrations: [],
}

// Initialize store with fixture data
initStore(store)

// Build a flat route table from routers
const routes = []
function extractRoutes(router, mountPath) {
  if (!router.stack) return
  for (const layer of router.stack) {
    if (!layer.route) continue
    const methods = Object.keys(layer.route.methods)
    for (const method of methods) {
      const path = layer.route.path
      const fullPath = mountPath + path
      const handlers = layer.route.methods[method]
      routes.push({ method: method.toUpperCase(), path: fullPath, handlers })
    }
  }
}

// Auth routes
extractRoutes(createAuthRouter(store), '/v1')
// User routes
extractRoutes(createUserRouter(store), '/v1')
// Journey routes
extractRoutes(createJourneyRouter(store), '/v1')
// Sessions routes
extractRoutes(createSessionsRouter(store), '/v1')
// Memories routes
extractRoutes(createMemoriesRouter(store), '/v1')
// Commitments routes
extractRoutes(createCommitmentsRouter(store), '/v1')
// Wheel of Life routes
extractRoutes(createWheelOfLifeRouter(store), '/v1')
// Eisenhower routes
extractRoutes(createEisenhowerRouter(store), '/v1')
// Streak routes
extractRoutes(createStreakRouter(store), '/v1')
// Usage routes
extractRoutes(createUsageRouter(store), '/v1')
// Feedback routes
extractRoutes(createFeedbackRouter(store), '/v1')
// Simulation routes (for error/resilience testing)
extractRoutes(createSimulationRouter(store), '/v1')
// Integrations routes (mounted at /v1/me)
extractRoutes(createIntegrationsRouter(store), '/v1/me')

// Simple path matcher: convert /memories/:id to regex
function matchPath(pattern, actual) {
  const params = {}
  const segments = pattern.split('/')
  const actualSegments = actual.split('/')
  
  if (segments.length !== actualSegments.length) return null
  
  for (let i = 0; i < segments.length; i++) {
    if (segments[i].startsWith(':')) {
      params[segments[i].slice(1)] = decodeURIComponent(actualSegments[i])
    } else if (segments[i] !== actualSegments[i]) {
      return null
    }
  }
  return params
}

const PORT = Number(process.env.MOCK_PORT) || 3001

const server = Bun.serve({
  port: PORT,

  // Bun's native WebSocket handler
  websocket: createWebSocketHandler(store),

  async fetch(req) {
    const url = new URL(req.url)

    // Handle WebSocket upgrade in fetch handler (Bun native approach)
    if (req.headers.get('upgrade')?.toLowerCase() === 'websocket') {
      const wsData = validateWsUpgrade(req, store)
      if (!wsData) {
        return new Response('Unauthorized', { status: 401 })
      }
      // Echo the sentinel subprotocol back, but only to a client that offered one: a
      // browser aborts the handshake if the server names none of the protocols it
      // asked for, and equally if it names one it did not.
      const upgraded = server.upgrade(req, {
        data: wsData,
        ...(wsData.subprotocol
          ? { headers: { 'Sec-WebSocket-Protocol': wsData.subprotocol } }
          : {}),
      })
      if (!upgraded) {
        return new Response('Upgrade failed', { status: 400 })
      }
      return undefined
    }

    // CORS preflight
    if (req.method === 'OPTIONS') {
      return new Response(null, {
        status: 204,
        headers: {
          'Access-Control-Allow-Origin': '*',
          'Access-Control-Allow-Methods': 'GET, POST, PUT, PATCH, DELETE, OPTIONS',
          'Access-Control-Allow-Headers': 'Content-Type, Authorization, X-Platform, X-App-Version, X-Timezone',
        },
      })
    }

    // Parse JSON body
    let body = null
    const contentType = req.headers.get('content-type') || ''
    if (['POST', 'PATCH', 'PUT', 'DELETE'].includes(req.method) && contentType.includes('json')) {
      try {
        body = await req.json()
      } catch {
        return new Response(JSON.stringify({ code: 'INVALID_PAYLOAD', error: 'Invalid JSON body' }), {
          status: 400,
          headers: { 'Content-Type': 'application/json' },
        })
      }
    }

    // Create Express-compatible req/res
    const expressReq = bunReqToExpress(req, url)
    expressReq.body = body

    const expressRes = createExpressRes()

    // Find matching route
    let matched = false
    for (const route of routes) {
      if (route.method !== req.method) continue

      const params = matchPath(route.path, url.pathname)
      if (!params) continue

      expressReq.params = params

      // Execute middleware chain
      try {
        await runMiddlewareChain(route.handlers, expressReq, expressRes)
      } catch (e) {
        console.error(`[MockServer] Route error: ${req.method} ${route.path}`, e.message)
        expressRes.status(500).json({ code: 'INTERNAL_ERROR', error: e.message })
      }
      matched = true
      break
    }

    if (!matched) {
      return new Response(JSON.stringify({ code: 'NOT_FOUND', error: `Not found: ${req.method} ${url.pathname}` }), {
        status: 404,
        headers: { 'Content-Type': 'application/json' },
      })
    }

    // Add CORS headers
    const response = expressRes.resolvedResponse
    const newHeaders = new Headers(response.headers)
    newHeaders.set('Access-Control-Allow-Origin', '*')
    newHeaders.set('Access-Control-Allow-Methods', 'GET, POST, PUT, PATCH, DELETE, OPTIONS')
    newHeaders.set('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Platform, X-App-Version, X-Timezone')

    return new Response(response.body, {
      status: response.status,
      headers: newHeaders,
    })
  },
})

console.log(`[MockServer] Listening on port ${PORT}`)
console.log('[MockServer] Auth backend: http://localhost:' + PORT + '/v1')
console.log('[MockServer] WebSocket: ws://localhost:' + PORT + '/v1/ws/chat')
console.log('[MockServer]')
console.log('[MockServer] To connect your app, set these env vars:')
console.log('[MockServer]   EXPO_PUBLIC_API_HOST=localhost:' + PORT)
console.log('[MockServer]   EXPO_PUBLIC_AUTH_BACKEND_URL=http://localhost:' + PORT + '/v1')
console.log('[MockServer]')
console.log('[MockServer] Available endpoints:')
console.log('[MockServer]   POST /v1/auth/verifications/request')
console.log('[MockServer]   POST /v1/auth/verifications/verify')
console.log('[MockServer]   POST /v1/auth/verifications/sso')
console.log('[MockServer]   POST /v1/auth/login/code')
console.log('[MockServer]   POST /v1/auth/register')
console.log('[MockServer]   POST /v1/auth/google')
console.log('[MockServer]   POST /v1/auth/refresh')
console.log('[MockServer]   PUT  /v1/auth/me/identifier')
console.log('[MockServer]   GET  /v1/me')
console.log('[MockServer]   PATCH /v1/me')
console.log('[MockServer]   DELETE /v1/me')
console.log('[MockServer]   DELETE /v1/me/data')
console.log('[MockServer]   GET  /v1/me/profile')
console.log('[MockServer]   GET  /v1/journey')
console.log('[MockServer]   GET  /v1/sessions')
console.log('[MockServer]   GET  /v1/memories')
console.log('[MockServer]   POST /v1/memories')
console.log('[MockServer]   DELETE /v1/memories/:id')
console.log('[MockServer]   GET  /v1/commitments')
console.log('[MockServer]   PATCH /v1/commitments/:id')
console.log('[MockServer]   GET  /v1/wheel-of-life')
console.log('[MockServer]   POST /v1/wheel-of-life')
console.log('[MockServer]   GET  /v1/eisenhower-matrix')
console.log('[MockServer]   POST /v1/eisenhower-matrix')
console.log('[MockServer]   GET  /v1/streak')
console.log('[MockServer]   GET  /v1/usage-calendar')
console.log('[MockServer]   GET  /v1/me/integrations')
console.log('[MockServer]   POST /v1/me/integrations/:provider/link')
console.log('[MockServer]   DELETE /v1/me/integrations/:id')
