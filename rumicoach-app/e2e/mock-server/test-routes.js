#!/usr/bin/env node
// e2e/mock-server/test-routes.js - Full HTTP route test suite

import { mockTokens } from './fixtures/test-users.js'

const BASE = 'http://localhost:3001'
const EU = mockTokens['test-user-eu-1']
const EU_ADMIN = mockTokens['test-user-eu-admin']
const US = mockTokens['test-user-us-1']

let passed = 0, failed = 0

async function test(name, fn) {
  try {
    await fn()
    console.log(`✅ ${name}`)
    passed++
  } catch (e) {
    console.log(`❌ ${name}`)
    console.log(`   Error: ${e.message}`)
    failed++
  }
}

function getHeaders(user) {
  return { Authorization: `Bearer ${user.accessToken}`, 'Content-Type': 'application/json' }
}

async function run() {
  console.log('=== Mock Server Route Tests ===\n')

  // ── Auth: 401 ──
  await test('1. GET /me — 401 no token', async () => {
    const res = await fetch(`${BASE}/v1/me`)
    const body = await res.json()
    if (res.status !== 401 || body.code !== 'UNAUTHORIZED') throw new Error(`Expected 401, got ${res.status} ${body.code}`)
  })

  await test('2. GET /me — 401 bad token', async () => {
    const res = await fetch(`${BASE}/v1/me`, { headers: { Authorization: 'Bearer invalid' } })
    const body = await res.json()
    if (res.status !== 401 || body.code !== 'UNAUTHORIZED') throw new Error(`Expected 401, got ${res.status} ${body.code}`)
  })

  // ── Auth: register ──
  await test('3. POST /auth/register', async () => {
    const res = await fetch(`${BASE}/v1/auth/register`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'New Tester', email: 'newtest@rumi.coach', dataRegion: 'eu', termsAndConditionsAccepted: true, aiAccepted: true }),
    })
    const body = await res.json()
    if (res.status !== 201 || body.code !== 'REGISTRATION_PENDING') throw new Error(`Expected 201 REGISTRATION_PENDING, got ${res.status} ${body.code}`)
  })

  // ── Auth: verifications ──
  await test('4. POST /auth/verifications/request (email)', async () => {
    const res = await fetch(`${BASE}/v1/auth/verifications/request`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: 'email', event: 'signup', email: 'test@rumi.coach' }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'VERIFICATION_SENT') throw new Error(`Expected 200 VERIFICATION_SENT, got ${res.status} ${body.code}`)
  })

  await test('5. POST /auth/verifications/verify', async () => {
    const res = await fetch(`${BASE}/v1/auth/verifications/verify`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: 'email', code: '123456', email: 'e2e-test@rumi.coach' }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'VERIFIED') throw new Error(`Expected 200 VERIFIED, got ${res.status} ${body.code}`)
  })

  await test('6. POST /auth/verifications/request (phone)', async () => {
    const res = await fetch(`${BASE}/v1/auth/verifications/request`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: 'phone', event: 'login', phoneNumber: '+1234567890' }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'VERIFICATION_SENT') throw new Error(`Expected 200 VERIFICATION_SENT, got ${res.status} ${body.code}`)
  })

  // ── Auth: login/code ──
  await test('7. POST /auth/login/code', async () => {
    const res = await fetch(`${BASE}/v1/auth/login/code`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: 'email', identifier: 'e2e-test@rumi.coach', code: '123456' }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'AUTHENTICATED' || !body.tokens) throw new Error(`Expected 200 AUTHENTICATED with tokens, got ${res.status} ${body.code}`)
  })

  // ── Auth: google ──
  await test('8. POST /auth/google', async () => {
    const res = await fetch(`${BASE}/v1/auth/google`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ accessToken: 'fake_google_token' }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'AUTHENTICATED') throw new Error(`Expected 200 AUTHENTICATED, got ${res.status} ${body.code}`)
  })

  // ── Auth: refresh ──
  await test('9. POST /auth/refresh', async () => {
    const res = await fetch(`${BASE}/v1/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refreshToken: EU.refreshToken }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'REFRESHED' || !body.tokens) throw new Error(`Expected 200 REFRESHED, got ${res.status} ${body.code}`)
  })

  // ── Auth: identifier update ──
  await test('10. PUT /auth/me/identifier', async () => {
    const res = await fetch(`${BASE}/v1/auth/me/identifier`, {
      method: 'PUT',
      headers: getHeaders(EU),
      body: JSON.stringify({ identifier: 'updated@rumi.coach' }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'IDENTIFIER_UPDATED') throw new Error(`Expected 200 IDENTIFIER_UPDATED, got ${res.status} ${body.code}`)
  })

  // ── User: GET /me ──
  await test('11. GET /me (EU user)', async () => {
    const res = await fetch(`${BASE}/v1/me`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || body.id !== 'test-user-eu-1') throw new Error(`Expected 200 eu user, got ${res.status} ${body.id}`)
  })

  await test('12. GET /me (US user)', async () => {
    const res = await fetch(`${BASE}/v1/me`, { headers: getHeaders(US) })
    const body = await res.json()
    if (res.status !== 200 || body.id !== 'test-user-us-1') throw new Error(`Expected 200 us user, got ${res.status} ${body.id}`)
  })

  // ── User: PATCH /me ──
  await test('13. PATCH /me', async () => {
    const res = await fetch(`${BASE}/v1/me`, {
      method: 'PATCH',
      headers: getHeaders(EU),
      body: JSON.stringify({ name: 'Updated Name', preferences: { language: 'en' } }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'USER_UPDATED') throw new Error(`Expected 200 USER_UPDATED, got ${res.status} ${body.code}`)
  })

  // ── User: GET /me/profile ──
  await test('14. GET /me/profile', async () => {
    const res = await fetch(`${BASE}/v1/me/profile`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || !body.name) throw new Error(`Expected 200 with profile data, got ${res.status}`)
  })

  // ── User: DELETE /me ──
  await test('15. DELETE /me', async () => {
    const res = await fetch(`${BASE}/v1/me`, {
      method: 'DELETE',
      headers: getHeaders(EU_ADMIN),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'ACCOUNT_DELETED') throw new Error(`Expected 200 ACCOUNT_DELETED, got ${res.status} ${body.code}`)
  })

  // ── User: DELETE /me/data ──
  await test('16. DELETE /me/data?scope=memories', async () => {
    const res = await fetch(`${BASE}/v1/me/data?scope=memories`, {
      method: 'DELETE',
      headers: getHeaders(EU),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'DATA_DELETED') throw new Error(`Expected 200 DATA_DELETED, got ${res.status} ${body.code}`)
  })

  // ── Journey ──
  await test('17. GET /journey', async () => {
    const res = await fetch(`${BASE}/v1/journey`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || !body.quote || !body.sessions || !body.badges) throw new Error(`Expected 200 with journey data, got ${res.status}`)
  })

  // ── Sessions ──
  await test('18. GET /sessions', async () => {
    const res = await fetch(`${BASE}/v1/sessions`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || !Array.isArray(body.items) || body.items.length === 0) throw new Error(`Expected 200 with sessions, got ${res.status}`)
  })

  // ── Memories ──
  await test('19. GET /memories', async () => {
    const res = await fetch(`${BASE}/v1/memories`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || !Array.isArray(body.items)) throw new Error(`Expected 200 with memories, got ${res.status}`)
  })

  await test('20. GET /memories?category=insight', async () => {
    const res = await fetch(`${BASE}/v1/memories?category=insight`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200) throw new Error(`Expected 200, got ${res.status}`)
  })

  await test('21. POST /memories', async () => {
    const res = await fetch(`${BASE}/v1/memories`, {
      method: 'POST',
      headers: getHeaders(EU),
      body: JSON.stringify({ category: 'insight', content: 'Test insight' }),
    })
    const body = await res.json()
    if (res.status !== 201 || !body.id) throw new Error(`Expected 201 with id, got ${res.status}`)
  })

  await test('22. DELETE /memories/:id', async () => {
    const listRes = await fetch(`${BASE}/v1/memories`, { headers: getHeaders(EU) })
    const listBody = await listRes.json()
    const id = listBody.items[0]?.id
    if (!id) throw new Error('No memories to delete')
    const res = await fetch(`${BASE}/v1/memories/${id}`, {
      method: 'DELETE',
      headers: getHeaders(EU),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'MEMORY_DELETED') throw new Error(`Expected 200 MEMORY_DELETED, got ${res.status} ${body.code}`)
  })

  // ── Commitments ──
  await test('23. GET /commitments', async () => {
    const res = await fetch(`${BASE}/v1/commitments`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || !Array.isArray(body) || body.length === 0) throw new Error(`Expected 200 with commitments, got ${res.status}, length=${body?.length ?? 0}`)
  })

  await test('24. PATCH /commitments/:id', async () => {
    const listRes = await fetch(`${BASE}/v1/commitments`, { headers: getHeaders(EU) })
    const listBody = await listRes.json()
    const id = listBody[0]?.id
    if (!id) throw new Error('No commitments to update')
    const res = await fetch(`${BASE}/v1/commitments/${id}`, {
      method: 'PATCH',
      headers: getHeaders(EU),
      body: JSON.stringify({ status: 'completed' }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'COMMITMENT_UPDATED') throw new Error(`Expected 200 COMMITMENT_UPDATED, got ${res.status} ${body.code}`)
  })

  // ── Wheel of Life ──
  await test('25. GET /wheel-of-life', async () => {
    const res = await fetch(`${BASE}/v1/wheel-of-life`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || body.entries.length === 0) throw new Error(`Expected 200 with entries, got ${res.status}`)
  })

  await test('26. POST /wheel-of-life', async () => {
    const res = await fetch(`${BASE}/v1/wheel-of-life`, {
      method: 'POST',
      headers: getHeaders(EU),
      body: JSON.stringify({ data: { health: 8, career: 7, relationships: 9, personal_growth: 6, finances: 5, hobbies: 8, environment: 7, spirituality: 4 } }),
    })
    const body = await res.json()
    if (res.status !== 201 || !body.id || !body.version) throw new Error(`Expected 201 with id+version, got ${res.status}`)
  })

  // ── Eisenhower ──
  await test('27. GET /eisenhower-matrix', async () => {
    const res = await fetch(`${BASE}/v1/eisenhower-matrix`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || body.entries.length === 0) throw new Error(`Expected 200 with entries, got ${res.status}`)
  })

  await test('28. POST /eisenhower-matrix', async () => {
    const res = await fetch(`${BASE}/v1/eisenhower-matrix`, {
      method: 'POST',
      headers: getHeaders(EU),
      body: JSON.stringify({ data: { urgentImportant: ['Task 1'], urgentNotImportant: ['Task 2'], notUrgentImportant: ['Task 3'], notUrgentNotImportant: [] } }),
    })
    const body = await res.json()
    if (res.status !== 201 || !body.id) throw new Error(`Expected 201 with id, got ${res.status}`)
  })

  // ── Streak ──
  await test('29. GET /streak', async () => {
    const res = await fetch(`${BASE}/v1/streak`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || body.currentStreak === undefined) throw new Error(`Expected 200 with streak data, got ${res.status}`)
  })

  // ── Usage Calendar ──
  await test('30. GET /usage-calendar', async () => {
    const res = await fetch(`${BASE}/v1/usage-calendar`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || !body.month || !body.days) throw new Error(`Expected 200 with calendar data, got ${res.status}`)
  })

  // ── Integrations ──
  await test('31. GET /me/integrations', async () => {
    const res = await fetch(`${BASE}/v1/me/integrations`, { headers: getHeaders(EU) })
    const body = await res.json()
    if (res.status !== 200 || !Array.isArray(body) || body.length === 0) throw new Error(`Expected 200 with integrations, got ${res.status}`)
  })

  await test('32. POST /me/integrations/whatsapp/link', async () => {
    const res = await fetch(`${BASE}/v1/me/integrations/whatsapp/link`, {
      method: 'POST',
      headers: getHeaders(EU),
      body: JSON.stringify({ phoneNumber: '+1234567890' }),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'INTEGRATION_LINKED') throw new Error(`Expected 200 INTEGRATION_LINKED, got ${res.status} ${body.code}`)
  })

  await test('33. DELETE /me/integrations/:id', async () => {
    const listRes = await fetch(`${BASE}/v1/me/integrations`, { headers: getHeaders(EU) })
    const listBody = await listRes.json()
    const id = listBody[0]?.id
    if (!id) throw new Error('No integrations to delete')
    const res = await fetch(`${BASE}/v1/me/integrations/${id}`, {
      method: 'DELETE',
      headers: getHeaders(EU),
    })
    const body = await res.json()
    if (res.status !== 200 || body.code !== 'INTEGRATION_UNLINKED') throw new Error(`Expected 200 INTEGRATION_UNLINKED, got ${res.status} ${body.code}`)
  })

  // ── CORS ──
  await test('34. OPTIONS preflight', async () => {
    const res = await fetch(`${BASE}/v1/me`, {
      method: 'OPTIONS',
      headers: {
        Origin: 'http://example.com',
        'Access-Control-Request-Method': 'GET',
        'Access-Control-Request-Headers': 'Authorization',
      },
    })
    if (res.status !== 204) throw new Error(`Expected 204, got ${res.status}`)
    const allowOrigin = res.headers.get('Access-Control-Allow-Origin')
    if (!allowOrigin) throw new Error('Missing Access-Control-Allow-Origin header')
  })

  // ── 404 ──
  await test('35. GET /v1/nonexistent — 404', async () => {
    const res = await fetch(`${BASE}/v1/nonexistent`)
    const body = await res.json()
    if (res.status !== 404 || body.code !== 'NOT_FOUND') throw new Error(`Expected 404 NOT_FOUND, got ${res.status} ${body.code}`)
  })

  console.log(`\n=== Results: ${passed} passed, ${failed} failed ===`)
  if (failed > 0) {
    console.log('\n❌ Some tests failed!')
    process.exit(1)
  } else {
    console.log('\n✅ All tests passed!')
  }
}

run().catch(e => {
  console.error('Test runner error:', e)
  process.exit(1)
})
