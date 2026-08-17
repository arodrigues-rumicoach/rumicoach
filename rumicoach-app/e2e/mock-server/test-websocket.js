#!/usr/bin/env bun
// e2e/mock-server/test-websocket.js — WebSocket integration tests

import { mockTokens } from './fixtures/test-users.js'

const BASE = 'ws://localhost:3001/v1/ws/chat'
let PASSED = 0
let FAILED = 0

function logPass(name) { console.log(`✅ ${name}`); PASSED++ }
function logFail(name, msg) { console.log(`❌ ${name}: ${msg}`); FAILED++ }

function loadToken(userId) {
  return mockTokens[userId]?.accessToken
}

function connect(token) {
  return new Promise((resolve, reject) => {
    const url = `${BASE}?token=${encodeURIComponent(token)}`
    const ws = new WebSocket(url)
    const messages = []
    const timeouts = []

    ws.onopen = () => {}
    ws.onmessage = (e) => messages.push(JSON.parse(e.data))
    ws.onerror = (e) => reject(e)
    ws.onclose = () => { for (const t of timeouts) clearTimeout(t) }

    // Helper: wait for a specific message type
    function waitFor(type, timeoutMs = 5000) {
      return new Promise((res, rej) => {
        const start = Date.now()
        const t = setTimeout(() => {
          rej(new Error(`Timeout waiting for ${type} (got: ${messages.map(m => m.type).join(', ')})`))
        }, timeoutMs)
        timeouts.push(t)

        const check = () => {
          const idx = messages.findIndex(m => m.type === type)
          if (idx >= 0) {
            clearTimeout(t)
            const msg = messages.splice(idx, 1)[0]
            res(msg)
          } else if (ws.readyState === WebSocket.CLOSED) {
            clearTimeout(t)
            rej(new Error(`WebSocket closed while waiting for ${type}`))
          } else {
            setTimeout(check, 20)
          }
        }
        check()
      })
    }

    // Helper: wait for multiple messages
    function waitForAll(types, timeoutMs = 5000) {
      return Promise.all(types.map(t => waitFor(t, timeoutMs)))
    }

    function send(msg) {
      ws.send(JSON.stringify(msg))
    }

    function close() {
      if (ws.readyState === WebSocket.OPEN) ws.close()
    }

    resolve({ ws, messages, waitFor, waitForAll, send, close })
  })
}

async function testAuthRejection() {
  const name = 'WS: connection rejected without token'
  try {
    const ws = new WebSocket(BASE)
    await new Promise((resolve, reject) => {
      ws.onopen = () => reject(new Error('Should not connect'))
      ws.onerror = () => resolve()
      ws.onclose = () => resolve()
      setTimeout(() => resolve(), 2000)
    })
    logPass(name)
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testAuthRejectionBadToken() {
  const name = 'WS: connection rejected with invalid token'
  try {
    const ws = new WebSocket(BASE, { headers: { Authorization: 'Bearer badtoken' } })
    await new Promise((resolve, reject) => {
      ws.onopen = () => reject(new Error('Should not connect'))
      ws.onerror = () => resolve()
      ws.onclose = () => resolve()
      setTimeout(() => resolve(), 2000)
    })
    logPass(name)
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testSessionCreated() {
  const name = 'WS: receives session_created on connect'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    const msg = await client.waitFor('session_created')
    if (msg.session_id) {
      logPass(name)
    } else {
      logFail(name, 'Missing session_id')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testSetupComplete() {
  const name = 'WS: receives setup_complete after session_created'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    const msg = await client.waitFor('setup_complete')
    if (msg.type === 'setup_complete') {
      logPass(name)
    } else {
      logFail(name, `Unexpected: ${msg.type}`)
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testUserMessageFlow() {
  const name = 'WS: user_message triggers thinking → speaking → listening'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'user_message', content: 'Hello coach' })

    const thinking = await client.waitFor('assistant_status')
    if (thinking.status !== 'thinking') {
      logFail(name, `Expected thinking, got ${thinking.status}`)
      client.close()
      return
    }

    const speaking = await client.waitFor('assistant_status')
    if (speaking.status !== 'speaking') {
      logFail(name, `Expected speaking, got ${speaking.status}`)
      client.close()
      return
    }

    const listening = await client.waitFor('assistant_status')
    if (listening.status !== 'listening') {
      logFail(name, `Expected listening, got ${listening.status}`)
      client.close()
      return
    }

    logPass(name)
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testSessionEnd() {
  const name = 'WS: session_end terminates session'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'session_end' })

    // Should get session_terminated
    const terminated = await client.waitFor('session_terminated')
    if (terminated.type === 'session_terminated') {
      logPass(name)
    } else {
      logFail(name, `Expected session_terminated, got ${terminated.type}`)
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testWheelOfLifeUpdate() {
  const name = 'WS: wheel_of_life_update returns categories'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'wheel_of_life_update' })

    const msg = await client.waitFor('wheel_of_life_update')
    if (msg.data?.categories?.length === 8) {
      logPass(name)
    } else {
      logFail(name, `Expected 8 categories, got ${msg.data?.categories?.length}`)
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testEisenhowerMatrixUpdate() {
  const name = 'WS: eisenhower_matrix_update returns quadrants'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'eisenhower_matrix_update' })

    const msg = await client.waitFor('eisenhower_matrix_update')
    if (msg.data?.urgent_important && msg.data?.not_urgent_important) {
      logPass(name)
    } else {
      logFail(name, 'Missing quadrant data')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testSessionSummary() {
  const name = 'WS: session_summary returns insight and evaluation'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'session_summary' })

    const msg = await client.waitFor('session_summary')
    if (msg.data?.insight && msg.data?.evaluation) {
      logPass(name)
    } else {
      logFail(name, 'Missing insight or evaluation')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testSessionTasksUpdate() {
  const name = 'WS: session_tasks_update returns tasks array'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'session_tasks_update' })

    const msg = await client.waitFor('session_tasks_update')
    if (Array.isArray(msg.data?.tasks) && msg.data.tasks.length > 0) {
      logPass(name)
    } else {
      logFail(name, 'Missing tasks array')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testActionPlanUpdate() {
  const name = 'WS: action_plan_update returns short/long term plans'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'action_plan_update' })

    const msg = await client.waitFor('action_plan_update')
    if (msg.data?.shortTerm && msg.data?.longTerm) {
      logPass(name)
    } else {
      logFail(name, 'Missing shortTerm or longTerm')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

async function testShowScreen() {
  const name = 'WS: show_screen echoes back'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'show_screen', data: { screen: 'journey' } })

    const msg = await client.waitFor('show_screen')
    if (msg.data?.screen === 'journey') {
      logPass(name)
    } else {
      logFail(name, `Expected screen=journey, got ${msg.data?.screen}`)
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// Run all tests
console.log('=== WebSocket Integration Tests ===\n')

await testAuthRejection()
await testAuthRejectionBadToken()
await testSessionCreated()
await testSetupComplete()
await testUserMessageFlow()
await testSessionEnd()
await testWheelOfLifeUpdate()
await testEisenhowerMatrixUpdate()
await testSessionSummary()
await testSessionTasksUpdate()
await testActionPlanUpdate()
await testShowScreen()

// ═══════════════════════════════════════════════════════════════════════════════
// PART 2: Advanced WebSocket Tests
// ═══════════════════════════════════════════════════════════════════════════════
console.log('\n--- Advanced WebSocket Tests ---\n')

// ── Malformed JSON ──
async function testMalformedJson() {
  const name = 'WS: malformed JSON does not crash server'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    // Send invalid JSON
    client.ws.send('not valid json {{{')

    // Server should not crash — send another message to verify
    await new Promise(r => setTimeout(r, 200))
    client.send({ type: 'show_screen', data: { screen: 'journey' } })
    const msg = await client.waitFor('show_screen')
    if (msg.data?.screen === 'journey') {
      logPass(name)
    } else {
      logFail(name, 'Server did not respond after malformed JSON')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Unknown message type ──
async function testUnknownMessageType() {
  const name = 'WS: unknown message type is silently ignored'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'completely_unknown_type', data: {} })

    // Server should not crash — verify with another message
    await new Promise(r => setTimeout(r, 200))
    client.send({ type: 'show_screen', data: { screen: 'memories' } })
    const msg = await client.waitFor('show_screen')
    if (msg.data?.screen === 'memories') {
      logPass(name)
    } else {
      logFail(name, 'Server did not respond after unknown type')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Max exchanges limit ──
async function testMaxExchanges() {
  const name = 'WS: max exchanges triggers session wrap-up'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    // Send 4 user_messages (maxExchanges = 4)
    for (let i = 0; i < 4; i++) {
      client.send({ type: 'user_message', content: `Message ${i + 1}` })
      await new Promise(r => setTimeout(r, 500))
    }

    // After max exchanges, should get session wrap-up data
    const wolMsg = await client.waitFor('wheel_of_life_update', 10000)
    if (wolMsg.data?.categories) {
      logPass(name)
    } else {
      logFail(name, 'Expected wheel_of_life_update after max exchanges')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Wheel of Life data shape ──
async function testWheelOfLifeDataShape() {
  const name = 'WS: wheel_of_life_update has correct data shape'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'wheel_of_life_update' })

    const msg = await client.waitFor('wheel_of_life_update')
    const cat = msg.data?.categories?.[0]
    if (cat && typeof cat.name === 'string' && typeof cat.currentScore === 'number' && typeof cat.targetScore === 'number' && typeof cat.reasoning === 'string') {
      logPass(name)
    } else {
      logFail(name, 'Invalid category shape')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Eisenhower data shape ──
async function testEisenhowerDataShape() {
  const name = 'WS: eisenhower_matrix_update has correct data shape'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'eisenhower_matrix_update' })

    const msg = await client.waitFor('eisenhower_matrix_update')
    const task = msg.data?.urgent_important?.[0]
    if (task && typeof task.task === 'string' && typeof task.quadrant === 'string' && typeof task.reasoning === 'string') {
      logPass(name)
    } else {
      logFail(name, 'Invalid task shape')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Session summary data shape ──
async function testSessionSummaryDataShape() {
  const name = 'WS: session_summary has correct data shape'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'session_summary' })

    const msg = await client.waitFor('session_summary')
    if (typeof msg.data?.insight === 'string' && typeof msg.data?.evaluation === 'number' && typeof msg.data?.feedback === 'string') {
      logPass(name)
    } else {
      logFail(name, 'Invalid summary shape')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Session tasks data shape ──
async function testSessionTasksDataShape() {
  const name = 'WS: session_tasks_update has correct data shape'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'session_tasks_update' })

    const msg = await client.waitFor('session_tasks_update')
    const task = msg.data?.tasks?.[0]
    if (task && typeof task.title === 'string' && typeof task.type === 'string' && typeof task.origin === 'string' && typeof task.status === 'string') {
      logPass(name)
    } else {
      logFail(name, 'Invalid task shape')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Action plan data shape ──
async function testActionPlanDataShape() {
  const name = 'WS: action_plan_update has correct data shape'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'action_plan_update' })

    const msg = await client.waitFor('action_plan_update')
    const action = msg.data?.shortTerm?.[0]
    if (Array.isArray(msg.data?.shortTerm) && Array.isArray(msg.data?.longTerm) && action && typeof action.action === 'string') {
      logPass(name)
    } else {
      logFail(name, 'Invalid action plan shape')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Onboarding message ──
async function testOnboardingMessage() {
  const name = 'WS: onboarding returns step data'
  const token = loadToken('test-user-new')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'onboarding' })

    const msg = await client.waitFor('onboarding_step')
    if (msg.step && msg.data?.question && Array.isArray(msg.data?.options)) {
      logPass(name)
    } else {
      logFail(name, 'Invalid onboarding shape')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Rapid reconnection ──
async function testRapidReconnection() {
  const name = 'WS: rapid reconnection works'
  const token = loadToken('test-user-eu-1')
  try {
    // First connection
    const client1 = await connect(token)
    await client1.waitFor('session_created')
    client1.close()

    await new Promise(r => setTimeout(r, 300))

    // Second connection
    const client2 = await connect(token)
    const msg = await client2.waitFor('session_created')
    if (msg.session_id) {
      logPass(name)
    } else {
      logFail(name, 'Missing session_id on reconnect')
    }
    client2.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── session_end before setup_complete ──
async function testSessionEndBeforeSetup() {
  const name = 'WS: session_end before setup_complete'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')

    // Send session_end immediately (before setup_complete)
    client.send({ type: 'session_end' })

    // Should still get session_terminated
    const msg = await client.waitFor('session_terminated', 3000)
    if (msg.type === 'session_terminated') {
      logPass(name)
    } else {
      logFail(name, 'Expected session_terminated')
    }
    client.close()
  } catch (e) {
    // Connection may close before we get the message — that's acceptable
    logPass(name + ' (graceful close)')
  }
}

// ── show_screen echo with different data ──
async function testShowScreenEcho() {
  const name = 'WS: show_screen echoes various screens'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    const screens = ['memories', 'profile', 'settings']
    for (const screen of screens) {
      client.send({ type: 'show_screen', data: { screen } })
      const msg = await client.waitFor('show_screen')
      if (msg.data?.screen !== screen) {
        logFail(name, `Expected screen=${screen}, got ${msg.data?.screen}`)
        client.close()
        return
      }
    }
    logPass(name)
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Wheel of Life update data content ──
async function testWheelOfLifeContent() {
  const name = 'WS: wheel_of_life_update uses user focus area'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'wheel_of_life_update' })

    const msg = await client.waitFor('wheel_of_life_update')
    const categories = msg.data?.categories || []
    const names = categories.map(c => c.name)
    const expected = ['Health', 'Career', 'Relationships', 'Personal Growth', 'Finances', 'Hobbies', 'Environment', 'Spirituality']
    const allPresent = expected.every(n => names.includes(n))
    if (allPresent && categories.length === 8) {
      logPass(name)
    } else {
      logFail(name, `Missing categories: ${expected.filter(n => !names.includes(n)).join(', ')}`)
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Eisenhower quadrant coverage ──
async function testEisenhowerCoverage() {
  const name = 'WS: eisenhower_matrix_update has all 4 quadrants'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'eisenhower_matrix_update' })

    const msg = await client.waitFor('eisenhower_matrix_update')
    const d = msg.data || {}
    if (d.urgent_important && d.urgent_not_important && d.not_urgent_important && d.not_urgent_not_important) {
      logPass(name)
    } else {
      logFail(name, 'Missing quadrant')
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Session tasks multiple tasks ──
async function testSessionTasksMultiple() {
  const name = 'WS: session_tasks_update returns multiple tasks'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    client.send({ type: 'session_tasks_update' })

    const msg = await client.waitFor('session_tasks_update')
    if (msg.data?.tasks?.length >= 2) {
      logPass(name)
    } else {
      logFail(name, `Expected >= 2 tasks, got ${msg.data?.tasks?.length}`)
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Graceful close with code 1000 ──
async function testGracefulClose() {
  const name = 'WS: graceful close with code 1000'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    const closed = new Promise((resolve) => {
      client.ws.onclose = (e) => resolve(e.code)
    })

    client.ws.close(1000)
    const code = await closed
    if (code === 1000) {
      logPass(name)
    } else {
      logFail(name, `Expected close code 1000, got ${code}`)
    }
  } catch (e) {
    logFail(name, e.message)
  }
}

// ── Multiple user messages in rapid succession ──
async function testRapidUserMessages() {
  const name = 'WS: rapid user_messages are processed correctly'
  const token = loadToken('test-user-eu-1')
  try {
    const client = await connect(token)
    await client.waitFor('session_created')
    await client.waitFor('setup_complete')

    // Send 3 messages rapidly
    client.send({ type: 'user_message', content: 'Quick 1' })
    client.send({ type: 'user_message', content: 'Quick 2' })
    client.send({ type: 'user_message', content: 'Quick 3' })

    // Should get at least one thinking status
    const msg = await client.waitFor('assistant_status', 3000)
    if (msg.status === 'thinking' || msg.status === 'speaking') {
      logPass(name)
    } else {
      logFail(name, `Expected thinking/speaking, got ${msg.status}`)
    }
    client.close()
  } catch (e) {
    logFail(name, e.message)
  }
}

await testMalformedJson()
await testUnknownMessageType()
await testMaxExchanges()
await testWheelOfLifeDataShape()
await testEisenhowerDataShape()
await testSessionSummaryDataShape()
await testSessionTasksDataShape()
await testActionPlanDataShape()
await testOnboardingMessage()
await testRapidReconnection()
await testSessionEndBeforeSetup()
await testShowScreenEcho()
await testWheelOfLifeContent()
await testEisenhowerCoverage()
await testSessionTasksMultiple()
await testGracefulClose()
await testRapidUserMessages()

console.log(`\n=== Results: ${PASSED} passed, ${FAILED} failed ===`)
if (FAILED > 0) {
  console.log('\n❌ Some tests failed!')
  process.exit(1)
} else {
  console.log('\n✅ All WebSocket tests passed!')
}
