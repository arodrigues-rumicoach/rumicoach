import { v4 as uuidv4 } from 'uuid'
import { testUsers, mockTokens } from '../fixtures/test-users.js'

const connections = new Map()

// The sentinel subprotocol that precedes the access token in Sec-WebSocket-Protocol.
// Only the sentinel is echoed back on the handshake, never the token.
export const WS_AUTH_PROTOCOL = 'rumi-auth'

// Must match wsCloseInsufficientBalance in the backend's api/routes/chat.go and
// WS_CLOSE_INSUFFICIENT_BALANCE in src/context/SessionContext.tsx.
const WS_CLOSE_INSUFFICIENT_BALANCE = 4402

// The introductory sessions run until they have produced what they exist to produce:
// the profile details the intro collects and the ideal-life vision the Vision session
// writes. Same rule as the backend's balance.OpeningPairUnfinished, minus the abuse
// cap, which needs session history the mock does not keep.
//
// Account-level, not per session type — matching the real pre-flight, which runs before
// the server has resolved which session this is. The type-scoped half of the rule
// (balance.FreeSessionAvailable) decides the debit at session end, which the mock does
// not model.
function hasFreeIntroSession(user) {
  const profileDone = !!user.dateOfBirth && !!user.gender && !!user.country
  return !profileDone || !user.idealLifeVisionSetAt
}

function base64UrlDecode(input) {
  const base64 = input.replace(/-/g, '+').replace(/_/g, '/')
  const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), '=')
  return Buffer.from(padded, 'base64').toString('utf8')
}

function decodeJwtPayload(token) {
  const parts = token.split('.')
  if (parts.length !== 3) return null
  try {
    return JSON.parse(base64UrlDecode(parts[1]))
  } catch {
    return null
  }
}

/**
 * Bun WebSocket config — only open/message/close handlers.
 * Upgrade is handled in server.js fetch via server.upgrade().
 */
export function createWebSocketHandler(store) {
  return {
    open(ws) {
      const data = ws.data
      if (!data) return

      const { userId, region, sessionId } = data

      // Mirrors the real server: a session it will not grant is refused here, on an
      // already-open socket, because a refused upgrade carries no reason a WebSocket
      // client can read. The app turns this into the paywall — see the 'error' case in
      // SessionContext and WS_CLOSE_INSUFFICIENT_BALANCE.
      if (data.refuseForBalance) {
        ws.send(JSON.stringify({
          type: 'error',
          code: 'INSUFFICIENT_BALANCE',
          message: 'Not enough session minutes to start a session',
        }))
        ws.close(WS_CLOSE_INSUFFICIENT_BALANCE, 'INSUFFICIENT_BALANCE')
        return
      }

      // Store connection state
      const conn = {
        userId,
        region,
        sessionId,
        exchangeCount: 0,
        maxExchanges: 4,
        timeouts: [],
        phase: 'setup',
        ws,
      }
      connections.set(userId, conn)

      // Send session_created
      ws.send(JSON.stringify({ type: 'session_created', session_id: sessionId }))

      // After 500ms send setup_complete
      const t = setTimeout(() => {
        ws.send(JSON.stringify({ type: 'setup_complete' }))
        conn.phase = 'listening'
      }, 500)
      conn.timeouts.push(t)
    },

    message(ws, msg) {
      const data = ws.data
      if (!data) return

      const conn = connections.get(data.userId)
      if (!conn || conn.phase === 'terminated') return

      let parsed
      try {
        parsed = JSON.parse(typeof msg === 'string' ? msg : msg.toString())
      } catch {
        return
      }

      switch (parsed.type) {
        case 'user_message':
          conn.exchangeCount++
          handleThinking(conn)
          break
        case 'session_end':
          handleSessionEnd(conn)
          break
        case 'wheel_of_life_update':
          sendMockWheelOfLife(conn)
          break
        case 'eisenhower_matrix_update':
          sendMockEisenhower(conn)
          break
        case 'action_plan_update':
          sendMockActionPlan(conn)
          break
        case 'session_summary':
          sendMockSessionSummary(conn)
          break
        case 'session_tasks_update':
          sendMockSessionTasks(conn)
          break
        case 'show_screen':
          ws.send(JSON.stringify(parsed))
          break
        case 'onboarding':
          sendMockOnboarding(conn)
          break
      }
    },

    close(ws, code, reason) {
      const data = ws.data
      if (!data) return

      const conn = connections.get(data.userId)
      if (!conn) return

      for (const t of conn.timeouts) clearTimeout(t)
      conn.timeouts = []
      conn.phase = 'terminated'
      connections.delete(data.userId)
    },
  }
}

/**
 * Validate a WebSocket upgrade request and extract user data.
 * Called from server.js fetch handler before server.upgrade().
 * Returns { userId, region, sessionId, subprotocol, refuseForBalance } or null if
 * rejected. `subprotocol` is what the handshake must echo, and is null unless the
 * client offered one — see the note where it is set.
 */
export function validateWsUpgrade(req, store) {
  const url = new URL(req.url)
  if (url.pathname !== '/v1/ws/chat') return null

  // The app sends the token as the second Sec-WebSocket-Protocol value, after the
  // "rumi-auth" sentinel — the same shape the real backend reads (api/routes/chat.go),
  // and the only one a browser can produce, since the WebSocket API has no way to set
  // an Authorization header. The other two forms are kept for scripts and older clients.
  let token = null
  const protocols = (req.headers.get('Sec-WebSocket-Protocol') || '')
    .split(',')
    .map((p) => p.trim())
    .filter(Boolean)
  const sentinel = protocols.indexOf(WS_AUTH_PROTOCOL)
  if (sentinel !== -1 && protocols[sentinel + 1]) {
    token = protocols[sentinel + 1]
  }
  if (!token) token = req.headers.get('Authorization')?.replace('Bearer ', '')
  if (!token) token = url.searchParams.get('token')
  if (!token) return null

  // Echoed only back to a client that actually offered it. RFC 6455 requires a client
  // to fail the connection when the server answers with a subprotocol it did not ask
  // for, so echoing unconditionally breaks every plain ws:// caller — the scripted
  // tests here connect with ?token= and no subprotocol at all.
  const subprotocol = sentinel !== -1 ? WS_AUTH_PROTOCOL : null

  const payload = decodeJwtPayload(token)
  if (!payload || !payload.sub) return null

  const user = store.users.get(payload.sub)
  if (!user) return null

  // Decided here rather than by refusing the upgrade: the app has to be able to read
  // why, and open() is the first place it can be told. A balance the fixture does not
  // set at all means "unmetered", so existing scenarios are unaffected.
  const metered = typeof user.balanceSeconds === 'number'
  const refuseForBalance = metered && user.balanceSeconds < 60 && !hasFreeIntroSession(user)

  return {
    userId: payload.sub,
    region: payload.region || 'eu',
    sessionId: uuidv4(),
    subprotocol,
    refuseForBalance,
  }
}

// ─── Session logic ───────────────────────────────────────────────────────────

function sendJson(conn, msg) {
  if (conn.ws) conn.ws.send(JSON.stringify(msg))
}

function handleThinking(conn) {
  sendJson(conn, { type: 'assistant_status', status: 'thinking' })
  const t = setTimeout(() => handleSpeaking(conn), 1000)
  conn.timeouts.push(t)
}

function handleSpeaking(conn) {
  sendJson(conn, { type: 'assistant_status', status: 'speaking' })
  const t = setTimeout(() => handleListening(conn), 2000)
  conn.timeouts.push(t)
}

function handleListening(conn) {
  if (conn.exchangeCount >= conn.maxExchanges) {
    handleSessionWrapUp(conn)
    return
  }
  conn.phase = 'listening'
  sendJson(conn, { type: 'assistant_status', status: 'listening' })
}

function handleSessionWrapUp(conn) {
  conn.phase = 'wrapping_up'
  const t1 = setTimeout(() => {
    sendMockWheelOfLife(conn)
    const t2 = setTimeout(() => {
      sendMockEisenhower(conn)
      const t3 = setTimeout(() => {
        sendMockActionPlan(conn)
        const t4 = setTimeout(() => {
          sendMockSessionSummary(conn)
          const t5 = setTimeout(() => {
            sendMockSessionTasks(conn)
            const t6 = setTimeout(() => {
              handleSessionEnd(conn)
            }, 1000)
            conn.timeouts.push(t6)
          }, 1000)
          conn.timeouts.push(t5)
        }, 1000)
        conn.timeouts.push(t4)
      }, 1000)
      conn.timeouts.push(t3)
    }, 1000)
    conn.timeouts.push(t2)
  }, 1000)
  conn.timeouts.push(t1)
}

function handleSessionEnd(conn) {
  for (const t of conn.timeouts) clearTimeout(t)
  conn.timeouts = []
  sendJson(conn, {
    type: 'show_screen',
    data: { screen: 'journey', at: 'session_end' },
  })
  const t = setTimeout(() => {
    sendJson(conn, { type: 'session_terminated' })
    conn.ws?.close(1000)
  }, 500)
  conn.timeouts.push(t)
}

function sendMockWheelOfLife(conn) {
  const user = testUsers[conn.userId]
  const scores = user?.wheelOfLife || {}
  sendJson(conn, {
    type: 'wheel_of_life_update',
    data: {
      categories: [
        { name: 'Health', currentScore: scores.health ?? 5, targetScore: 8, reasoning: 'Focus on physical well-being' },
        { name: 'Career', currentScore: scores.career ?? 6, targetScore: 9, reasoning: 'Advance professionally' },
        { name: 'Relationships', currentScore: scores.relationships ?? 6, targetScore: 8, reasoning: 'Strengthen connections' },
        { name: 'Personal Growth', currentScore: scores.personal_growth ?? 5, targetScore: 8, reasoning: 'Continue learning' },
        { name: 'Finances', currentScore: scores.finances ?? 5, targetScore: 7, reasoning: 'Build financial stability' },
        { name: 'Hobbies', currentScore: scores.hobbies ?? 5, targetScore: 7, reasoning: 'Make time for passions' },
        { name: 'Environment', currentScore: scores.environment ?? 5, targetScore: 7, reasoning: 'Live sustainably' },
        { name: 'Spirituality', currentScore: scores.spirituality ?? 4, targetScore: 7, reasoning: 'Deepen inner practice' },
      ],
    },
  })
}

function sendMockEisenhower(conn) {
  const user = testUsers[conn.userId]
  const focusArea = user?.focusArea || 'career'
  sendJson(conn, {
    type: 'eisenhower_matrix_update',
    data: {
      urgent_important: [
        { task: `Prepare ${focusArea} review`, quadrant: 'urgent_important', reasoning: 'Critical deadline ahead' },
        { task: 'Address pressing blockers', quadrant: 'urgent_important', reasoning: 'Needs immediate attention' },
      ],
      urgent_not_important: [
        { task: 'Reply to routine emails', quadrant: 'urgent_not_important', reasoning: 'Can be delegated' },
      ],
      not_urgent_important: [
        { task: `Build ${focusArea} strategy`, quadrant: 'not_urgent_important', reasoning: 'Long-term impact' },
        { task: 'Develop new skills', quadrant: 'not_urgent_important', reasoning: 'Invest in growth' },
      ],
      not_urgent_not_important: [
        { task: 'Organize workspace', quadrant: 'not_urgent_not_important', reasoning: 'Low priority' },
      ],
    },
  })
}

function sendMockActionPlan(conn) {
  const user = testUsers[conn.userId]
  const focusArea = user?.focusArea || 'career'
  sendJson(conn, {
    type: 'action_plan_update',
    data: {
      shortTerm: [
        { action: `Set up ${focusArea} weekly review`, deadline: '2024-07-20', priority: 'high' },
        { action: 'Delegate routine tasks', deadline: '2024-07-18', priority: 'high' },
      ],
      longTerm: [
        { action: `Get certified in ${focusArea}`, deadline: '2025-01-01', priority: 'medium' },
        { action: 'Build mentorship program', deadline: '2025-03-01', priority: 'medium' },
      ],
    },
  })
}

function sendMockSessionSummary(conn) {
  sendJson(conn, {
    type: 'session_summary',
    data: {
      insight: 'Recognized a limiting belief about delegation and found actionable steps to overcome it.',
      evaluation: 5,
      feedback: 'Very insightful session. The coach helped me see things from a new perspective.',
    },
  })
}

function sendMockSessionTasks(conn) {
  sendJson(conn, {
    type: 'session_tasks_update',
    data: {
      tasks: [
        { title: 'Schedule career mentoring session', type: 'one_time', origin: 'plan', status: 'pending', date: '2024-07-15' },
        { title: 'Read Atomic Habits', type: 'one_time', origin: 'plan', status: 'pending', date: '2024-07-20' },
        { title: 'Weekly team 1-on-1s', type: 'recurring', origin: 'behavior', status: 'pending', days: [1, 3, 5] },
      ],
    },
  })
}

function sendMockOnboarding(conn) {
  sendJson(conn, {
    type: 'onboarding_step',
    step: 1,
    data: {
      question: 'What area of your life would you like to focus on?',
      options: ['Career', 'Relationships', 'Health', 'Personal Growth', 'Finances'],
    },
  })
}
