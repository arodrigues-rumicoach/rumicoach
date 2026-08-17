import { v4 as uuidv4 } from 'uuid'
import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createAuthRouter(store) {
  const router = createRouter()

  router.post('/auth/verifications/request', (req, res) => {
    initStore(store)
    const { type, event, email, phoneNumber } = req.body
    if (!type || !event) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'type and event are required' })
    }
    if (type === 'email' && !email) {
      return res.status(400).json({ code: 'IDENTIFIER_REQUIRED', error: 'email is required for email verification' })
    }
    if (type === 'phone' && !phoneNumber) {
      return res.status(400).json({ code: 'IDENTIFIER_REQUIRED', error: 'phoneNumber is required for phone verification' })
    }

    const verificationId = uuidv4()
    const code = String(Math.floor(100000 + Math.random() * 900000))
    store.verifications.set(verificationId, { code, type, email, phoneNumber, event })

    if (process.env.MOCK_DEBUG) console.log(`[Mock] Verification requested: ${verificationId} code=${code} type=${type} event=${event}`)
    res.json({ verificationId })
  })

  router.post('/auth/verifications/verify', (req, res) => {
    initStore(store)
    const { type, code, email, phoneNumber, event } = req.body
    if (!type || !code) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'type and code are required' })
    }

    let found = null
    for (const [, v] of store.verifications) {
      if (v.code === code && v.type === type) {
        if (type === 'email' && email && v.email !== email) continue
        if (type === 'phone' && phoneNumber && v.phoneNumber !== phoneNumber) continue
        found = v
        break
      }
    }

    if (!found) {
      return res.status(400).json({ code: 'INVALID_CODE', error: 'Invalid verification code' })
    }

    res.json({ verified: true })
  })

  router.post('/auth/verifications/sso', (req, res) => {
    initStore(store)
    const { idToken, accessToken, type } = req.body
    if (!idToken || !type) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'idToken and type are required' })
    }

    const verificationId = `${type}_sso_${uuidv4()}`
    store.verifications.set(verificationId, { code: 'sso_verified', type: 'email', event: 'sso' })
    res.json({ verificationId })
  })

  router.post('/auth/login/code', (req, res) => {
    initStore(store)
    const { type, identifier, code } = req.body
    if (!type || !identifier || !code) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'type, identifier, and code are required' })
    }

    let found = null
    for (const [, v] of store.verifications) {
      if (v.code === code && v.type === type) {
        if (type === 'email' && v.email === identifier) { found = v; break }
        if (type === 'phone' && v.phoneNumber === identifier) { found = v; break }
      }
    }

    if (!found) {
      return res.status(400).json({ code: 'INVALID_CODE', error: 'Invalid verification code' })
    }

    let user = null
    for (const [, u] of store.users) {
      if (type === 'email' && u.email === identifier) { user = u; break }
      if (type === 'phone' && u.phoneNumber === identifier) { user = u; break }
    }

    if (!user) {
      return res.status(404).json({ code: 'ACCOUNT_NOT_FOUND', error: 'No account found for this identifier' })
    }

    res.json({ accessToken: user.accessToken, refreshToken: user.refreshToken })
  })

  router.post('/auth/register', (req, res) => {
    initStore(store)
    const { email, name, phoneNumber, preferredLanguage, termsAndConditionsAccepted, aiAccepted, marketingAccepted, dataRegion, emailVerificationId, phoneVerificationId } = req.body

    if (!name) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'name is required' })
    }
    if (!email && !phoneNumber) {
      return res.status(400).json({ code: 'IDENTIFIER_REQUIRED', error: 'email or phoneNumber is required' })
    }
    if (emailVerificationId && !store.verifications.has(emailVerificationId)) {
      return res.status(400).json({ code: 'VERIFICATION_REQUIRED', error: 'Email must be verified before registration' })
    }
    if (phoneVerificationId && !store.verifications.has(phoneVerificationId)) {
      return res.status(400).json({ code: 'VERIFICATION_REQUIRED', error: 'Phone must be verified before registration' })
    }

    const region = dataRegion || 'eu'
    const userId = uuidv4()
    const now = new Date().toISOString()

    const newUser = {
      id: userId,
      email,
      name,
      phoneNumber,
      preferredLanguage,
      region,
      idealLifeVisionSetAt: null,
      termsAndConditionsAccepted: termsAndConditionsAccepted ?? false,
      aiAccepted: aiAccepted ?? false,
      marketingAccepted: marketingAccepted ?? false,
      createdAt: now,
    }

    store.users.set(userId, newUser)

    const header = Buffer.from(JSON.stringify({ alg: 'none', typ: 'JWT' })).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
    const payload = Buffer.from(JSON.stringify({ sub: userId, region, iat: Date.now() })).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
    const accessToken = `${header}.${payload}.mocksig`
    const refreshToken = `refresh-${userId}`

    newUser.accessToken = accessToken
    newUser.refreshToken = refreshToken
    store.streakData[userId] = { currentStreak: 0, bestStreak: 0, lastSessionDate: null }
    store.usageCalendar[userId] = { dayStreak: 0, sessionsCount: 0, hours: 0, days: {} }

    res.status(201).json({ accessToken, refreshToken })
  })

  router.post('/auth/google', (req, res) => {
    initStore(store)
    const { accessToken: googleAccessToken } = req.body
    if (!googleAccessToken) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'accessToken is required' })
    }

    let user = null
    for (const [, u] of store.users) {
      if (u.googleAccessToken === googleAccessToken) { user = u; break }
    }

    if (user) {
      return res.json({ accessToken: user.accessToken, refreshToken: user.refreshToken })
    }

    const userId = uuidv4()
    const now = new Date().toISOString()
    const newUser = {
      id: userId,
      name: 'Google User',
      googleAccessToken,
      region: 'eu',
      idealLifeVisionSetAt: null,
      createdAt: now,
    }
    store.users.set(userId, newUser)

    const header = Buffer.from(JSON.stringify({ alg: 'none', typ: 'JWT' })).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
    const payload = Buffer.from(JSON.stringify({ sub: userId, region: 'eu', iat: Date.now() })).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
    const accessToken = `${header}.${payload}.mocksig`
    const refreshToken = `refresh-${userId}`

    newUser.accessToken = accessToken
    newUser.refreshToken = refreshToken
    store.streakData[userId] = { currentStreak: 0, bestStreak: 0, lastSessionDate: null }
    store.usageCalendar[userId] = { dayStreak: 0, sessionsCount: 0, hours: 0, days: {} }

    res.status(201).json({ accessToken, refreshToken })
  })

  router.post('/auth/apple', (req, res) => {
    initStore(store)
    const { identityToken, email, name } = req.body
    if (!identityToken) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'identityToken is required' })
    }

    let user = null
    for (const [, u] of store.users) {
      if (u.appleIdentityToken === identityToken) { user = u; break }
    }

    if (user) {
      return res.json({ accessToken: user.accessToken, refreshToken: user.refreshToken })
    }

    const userId = uuidv4()
    const now = new Date().toISOString()
    const newUser = {
      id: userId,
      name: name || 'Apple User',
      email,
      appleIdentityToken,
      region: 'eu',
      idealLifeVisionSetAt: null,
      createdAt: now,
    }
    store.users.set(userId, newUser)

    const header = Buffer.from(JSON.stringify({ alg: 'none', typ: 'JWT' })).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
    const payload = Buffer.from(JSON.stringify({ sub: userId, region: 'eu', iat: Date.now() })).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
    const accessToken = `${header}.${payload}.mocksig`
    const refreshToken = `refresh-${userId}`

    newUser.accessToken = accessToken
    newUser.refreshToken = refreshToken
    store.streakData[userId] = { currentStreak: 0, bestStreak: 0, lastSessionDate: null }
    store.usageCalendar[userId] = { dayStreak: 0, sessionsCount: 0, hours: 0, days: {} }

    res.status(201).json({ accessToken, refreshToken })
  })

  router.post('/auth/refresh', (req, res) => {
    initStore(store)
    const { refreshToken } = req.body
    if (!refreshToken) {
      return res.status(400).json({ code: 'INVALID_REFRESH_TOKEN', error: 'refreshToken is required' })
    }

    let user = null
    for (const [, u] of store.users) {
      if (u.refreshToken === refreshToken) { user = u; break }
    }

    if (!user) {
      return res.status(400).json({ code: 'INVALID_REFRESH_TOKEN', error: 'Invalid refresh token' })
    }

    const header = Buffer.from(JSON.stringify({ alg: 'none', typ: 'JWT' })).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
    const payload = Buffer.from(JSON.stringify({ sub: user.id, region: user.region, iat: Date.now() })).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
    const newAccessToken = `${header}.${payload}.mocksig`
    const newRefreshToken = `refresh-${user.id}-${Date.now()}`

    user.accessToken = newAccessToken
    user.refreshToken = newRefreshToken

    res.json({ accessToken: newAccessToken, refreshToken: newRefreshToken })
  })

  router.put('/auth/me/identifier', requireAuth(store), (req, res) => {
    const { type, identifier, verificationId } = req.body
    if (!type || !identifier || !verificationId) {
      return res.status(400).json({ code: 'INVALID_PAYLOAD', error: 'type, identifier, and verificationId are required' })
    }

    const verification = store.verifications.get(verificationId)
    if (!verification) {
      return res.status(400).json({ code: 'VERIFICATION_REQUIRED', error: 'Invalid verification ID' })
    }

    const user = store.users.get(req.userId)
    if (!user) {
      return res.status(404).json({ code: 'ACCOUNT_NOT_FOUND', error: 'User not found' })
    }

    if (type === 'email') user.email = identifier
    if (type === 'phone') user.phoneNumber = identifier

    res.json({ id: user.id, email: user.email, phoneNumber: user.phoneNumber, name: user.name })
  })

  return router
}
