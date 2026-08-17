import { describe, expect, it, beforeEach } from '@jest/globals'
import { renderHook, act, waitFor } from '@testing-library/react-native'
import { useSessionFeedbackTracker, resetSessionFeedbackTracker } from '../useSessionFeedbackTracker'

describe('useSessionFeedbackTracker', () => {
  beforeEach(() => {
    resetSessionFeedbackTracker()
  })
  it('does not show feedback for sessions shorter than 60 seconds', async () => {
    const { result, rerender } = await renderHook(
      ({ status, sessionId, durationSeconds }) =>
        useSessionFeedbackTracker(status, sessionId, durationSeconds),
      {
        initialProps: {
          status: 'listening' as const,
          sessionId: 'session-1',
          durationSeconds: 30,
        },
      },
    )

    await rerender({ status: 'disconnected', sessionId: null, durationSeconds: 0 })

    expect(result.current?.lastActiveSessionId).toBeNull()
    expect(result.current?.showFeedback).toBe(false)
  })

  it('shows feedback once after a session of 60 seconds or more', async () => {
    const { result, rerender } = await renderHook(
      ({ status, sessionId, durationSeconds }) =>
        useSessionFeedbackTracker(status, sessionId, durationSeconds),
      {
        initialProps: {
          status: 'listening' as const,
          sessionId: 'session-1',
          durationSeconds: 60,
        },
      },
    )

    await rerender({ status: 'disconnected', sessionId: null, durationSeconds: 0 })

    expect(result.current?.lastActiveSessionId).toBe('session-1')
    expect(result.current?.showFeedback).toBe(true)
  })

  it('hides feedback after dismissFeedback is called', async () => {
    const { result, rerender } = await renderHook(
      ({ status, sessionId, durationSeconds }) =>
        useSessionFeedbackTracker(status, sessionId, durationSeconds),
      {
        initialProps: {
          status: 'listening' as const,
          sessionId: 'session-1',
          durationSeconds: 60,
        },
      },
    )

    await rerender({ status: 'disconnected', sessionId: null, durationSeconds: 0 })
    expect(result.current?.showFeedback).toBe(true)

    await act(async () => {
      result.current?.dismissFeedback()
    })

    await waitFor(() => {
      expect(result.current?.showFeedback).toBe(false)
      expect(result.current?.lastActiveSessionId).toBeNull()
    })
  })

  it('does not show feedback again after dismissFeedback', async () => {
    const { result, rerender } = await renderHook(
      ({ status, sessionId, durationSeconds }) =>
        useSessionFeedbackTracker(status, sessionId, durationSeconds),
      {
        initialProps: {
          status: 'listening' as const,
          sessionId: 'session-1',
          durationSeconds: 60,
        },
      },
    )

    await rerender({ status: 'disconnected', sessionId: null, durationSeconds: 0 })
    expect(result.current?.showFeedback).toBe(true)

    await act(async () => {
      result.current?.dismissFeedback()
    })

    await waitFor(() => {
      expect(result.current?.showFeedback).toBe(false)
      expect(result.current?.lastActiveSessionId).toBeNull()
    })

    // Simulate additional renders while disconnected
    await rerender({ status: 'disconnected', sessionId: null, durationSeconds: 0 })
    await rerender({ status: 'disconnected', sessionId: null, durationSeconds: 0 })

    expect(result.current?.showFeedback).toBe(false)
    expect(result.current?.lastActiveSessionId).toBeNull()
  })

  it('does not show feedback again for the same session after the component remounts', async () => {
    // First instance shows and dismisses feedback.
    const { result: result1, rerender: rerender1 } = await renderHook(
      ({ status, sessionId, durationSeconds }) =>
        useSessionFeedbackTracker(status, sessionId, durationSeconds),
      {
        initialProps: {
          status: 'listening' as const,
          sessionId: 'session-1',
          durationSeconds: 60,
        },
      },
    )

    await rerender1({ status: 'disconnected', sessionId: null, durationSeconds: 0 })
    expect(result1.current?.showFeedback).toBe(true)

    await act(async () => {
      result1.current?.dismissFeedback()
    })

    await waitFor(() => {
      expect(result1.current?.showFeedback).toBe(false)
    })

    // A new hook instance (e.g. after the screen remounts) tries to show
    // feedback for the same session id.
    const { result: result2, rerender: rerender2 } = await renderHook(
      ({ status, sessionId, durationSeconds }) =>
        useSessionFeedbackTracker(status, sessionId, durationSeconds),
      {
        initialProps: {
          status: 'listening' as const,
          sessionId: 'session-1',
          durationSeconds: 60,
        },
      },
    )

    await rerender2({ status: 'disconnected', sessionId: null, durationSeconds: 0 })

    expect(result2.current?.showFeedback).toBe(false)
    expect(result2.current?.lastActiveSessionId).toBe('session-1')
  })

  it('still allows feedback for a different session', async () => {
    const { result: result1, rerender: rerender1 } = await renderHook(
      ({ status, sessionId, durationSeconds }) =>
        useSessionFeedbackTracker(status, sessionId, durationSeconds),
      {
        initialProps: {
          status: 'listening' as const,
          sessionId: 'session-1',
          durationSeconds: 60,
        },
      },
    )

    await rerender1({ status: 'disconnected', sessionId: null, durationSeconds: 0 })
    expect(result1.current?.showFeedback).toBe(true)

    const { result: result2, rerender: rerender2 } = await renderHook(
      ({ status, sessionId, durationSeconds }) =>
        useSessionFeedbackTracker(status, sessionId, durationSeconds),
      {
        initialProps: {
          status: 'listening' as const,
          sessionId: 'session-2',
          durationSeconds: 60,
        },
      },
    )

    await rerender2({ status: 'disconnected', sessionId: null, durationSeconds: 0 })

    expect(result2.current?.showFeedback).toBe(true)
    expect(result2.current?.lastActiveSessionId).toBe('session-2')
  })
})
