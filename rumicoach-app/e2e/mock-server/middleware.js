import * as fixtures from './fixtures/test-users.js'

function decodeJwt(token) {
  const parts = token.split('.')
  if (parts.length !== 3) return null
  try {
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const padded = b64.padEnd(b64.length + ((4 - (b64.length % 4)) % 4), '=')
    const payload = JSON.parse(Buffer.from(padded, 'base64').toString('utf8'))
    return payload
  } catch {
    return null
  }
}

export function initStore(store) {
  if (store.users.size > 0) return

  for (const [id, user] of Object.entries(fixtures.testUsers)) {
    store.users.set(id, { ...user })
  }

  for (const [id, v] of Object.entries(fixtures.verificationCodes)) {
    store.verifications.set(id, { ...v })
  }

  for (const [userId, tokens] of Object.entries(fixtures.mockTokens)) {
    const user = store.users.get(userId)
    if (user) {
      user.accessToken = tokens.accessToken
      user.refreshToken = tokens.refreshToken
    }
  }

  for (const session of fixtures.sessions) {
    store.sessions.set(session.id, { ...session })
  }

  store.memories = fixtures.memories.map(m => ({ ...m }))
  store.commitments = fixtures.commitments.map(c => ({ ...c }))
  store.wheelOfLifeEntries = fixtures.wheelOfLifeEntries.map(w => ({ ...w }))
  store.eisenhowerEntries = fixtures.eisenhowerEntries.map(e => ({ ...e }))
  store.integrations = fixtures.integrations.map(i => ({ ...i }))

  store.streakData = {}
  for (const [userId, data] of Object.entries(fixtures.streakData)) {
    store.streakData[userId] = { ...data }
  }

  store.usageCalendar = {}
  for (const [userId, cal] of Object.entries(fixtures.usageCalendar)) {
    store.usageCalendar[userId] = {
      ...cal,
      days: {},
    }
    for (const [date, dayData] of Object.entries(cal.days)) {
      store.usageCalendar[userId].days[date] = {
        ...dayData,
        sessions: dayData.sessions?.map(s => ({ ...s })),
      }
    }
  }
}

export function requireAuth(store) {
  return (req, res, next) => {
    initStore(store)

    const authHeader = req.headers.authorization
    if (!authHeader) {
      return res.status(401).json({ code: 'AUTH_TOKEN_MISSING', error: 'Authorization header required' })
    }

    const token = authHeader.replace('Bearer ', '')
    const payload = decodeJwt(token)

    if (!payload || !payload.sub) {
      return res.status(401).json({ code: 'UNAUTHENTICATED', error: 'Invalid access token' })
    }

    const userId = payload.sub
    if (!store.users.has(userId)) {
      return res.status(401).json({ code: 'UNAUTHENTICATED', error: 'User not found' })
    }

    req.userId = userId
    req.region = payload.region || 'eu'
    next()
  }
}
