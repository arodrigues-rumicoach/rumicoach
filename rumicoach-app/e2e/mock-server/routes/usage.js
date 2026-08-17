import { initStore, requireAuth } from '../middleware.js'
import { createRouter } from '../bun-router.js'

export default function createUsageRouter(store) {
  const router = createRouter()

  router.get('/usage-calendar', requireAuth(store), (req, res) => {
    initStore(store)
    const month = req.query.month
    const cal = store.usageCalendar[req.userId]

    if (!cal) {
      return res.json({
        dayStreak: 0,
        sessionsCount: 0,
        hours: 0,
        days: {},
      })
    }

    const days = {}
    let totalSessions = 0
    let totalHours = 0

    for (const [date, dayData] of Object.entries(cal.days)) {
      if (month && !date.startsWith(month)) continue
      const dayEntry = { date: dayData.date, kind: dayData.kind }
      if (dayData.sessions) {
        dayEntry.sessions = dayData.sessions
        totalSessions += dayData.sessions.length
        totalHours += dayData.sessions.reduce((sum, s) => sum + (s.duration || 0), 0) / 3600
      }
      days[date] = dayEntry
    }

    res.json({
      dayStreak: cal.dayStreak,
      sessionsCount: month ? totalSessions : cal.sessionsCount,
      hours: month ? parseFloat(totalHours.toFixed(1)) : cal.hours,
      days,
    })
  })

  // GET /me/transactions — the minutes bank's ledger, bank-statement style.
  // Mirrors the real backend: signed amountSeconds and a server-computed
  // balanceAfter on every row, newest first. With grouped=true, same-day runs
  // of message debits fold into one row carrying day + messageCount — the
  // fixture message days are pre-grouped, so each day is exactly one run.
  router.get('/me/transactions', requireAuth(store), (req, res) => {
    initStore(store)

    const page = Math.max(1, parseInt(req.query.page) || 1)
    const limit = Math.max(1, Math.min(100, parseInt(req.query.limit) || 20))
    const grouped = req.query.grouped === 'true' || req.query.grouped === '1'

    const events = []
    for (const s of store.sessions.values()) {
      if (s.userId !== req.userId) continue
      const free = s.sessionType === 'onboarding' || s.sessionType === 'session_vision'
      events.push({
        id: `tx-${s.id}`,
        type: free ? 'session_free' : 'session_usage',
        amountSeconds: free ? 0 : -(s.duration || 0),
        sessionId: s.id,
        sessionType: s.sessionType,
        createdAt: s.startTime,
      })
    }

    for (const day of usageMessageDays) {
      if (day.userId !== req.userId) continue
      if (grouped) {
        events.push({
          id: `tx-msg-${day.date}`,
          type: 'message_usage',
          amountSeconds: -(day.messageCount * SECONDS_PER_MESSAGE),
          createdAt: day.occurredAt,
          day: day.date,
          messageCount: day.messageCount,
        })
      } else {
        // Raw view: one debit per charged reply, spread backwards through the day.
        for (let i = 0; i < day.messageCount; i++) {
          events.push({
            id: `tx-msg-${day.date}-${i}`,
            type: 'message_usage',
            amountSeconds: -SECONDS_PER_MESSAGE,
            createdAt: new Date(new Date(day.occurredAt).getTime() - i * 60_000).toISOString(),
          })
        }
      }
    }

    // The opening grant, dated before every fixture event so the running
    // balance never starts from nothing.
    events.push({
      id: 'tx-grant-1',
      type: 'subscription',
      amountSeconds: 9000,
      product: 'membership_monthly',
      // Derived server-side from the product id in the real backend; clients
      // translate the enum instead of parsing slugs.
      plan: 'monthly',
      createdAt: '2024-01-01T09:00:00Z',
    })

    // balanceAfter accumulates oldest-first, like the real ledger writes it.
    events.sort((a, b) => new Date(a.createdAt) - new Date(b.createdAt))
    let balance = 0
    for (const e of events) {
      balance += e.amountSeconds
      e.balanceAfter = balance
    }
    events.reverse()

    const totalItems = events.length
    const totalPages = Math.max(1, Math.ceil(totalItems / limit))
    const startIndex = (page - 1) * limit

    res.json({
      items: events.slice(startIndex, startIndex + limit),
      pagination: {
        currentPage: page,
        totalPages,
        totalItems,
        itemsPerPage: limit,
      },
    })
  })

  return router
}

/** Flat per-message rate — server-owned in the real backend. */
const SECONDS_PER_MESSAGE = 5

// Companion-message day-groups interleaved between the fixture sessions, so the
// feed shows both entry kinds.
const usageMessageDays = [
  {
    userId: 'test-user-eu-1',
    date: '2024-05-21',
    occurredAt: '2024-05-21T18:40:00Z',
    messageCount: 12,
  },
  {
    userId: 'test-user-eu-1',
    date: '2024-05-19',
    occurredAt: '2024-05-19T09:12:00Z',
    messageCount: 3,
  },
]
