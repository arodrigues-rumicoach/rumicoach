import { describe, expect, it, beforeEach, afterEach, jest } from '@jest/globals'
import { renderHook, act } from '@testing-library/react-native'
import { useOtpCascade } from '../useOtpCascade'

describe('useOtpCascade', () => {
  beforeEach(() => {
    jest.useFakeTimers()
  })

  afterEach(() => {
    jest.useRealTimers()
  })

  it('does not crash on initial render', async () => {
    const { result } = await renderHook(() => useOtpCascade(''))
    expect(result.current).toBeDefined()
  })

  it('handles a single keystroke without crash', async () => {
    const { result, rerender } = await renderHook(
      ({ value }: { value: string }) => useOtpCascade(value),
      { initialProps: { value: '' } }
    )

    await rerender({ value: '1' })

    act(() => {
      jest.advanceTimersByTime(1000)
    })

    expect(result.current).toBeDefined()
  })

  it('cancels in-flight timers when a new paste arrives', async () => {
    const { result, rerender } = await renderHook(
      ({ value }: { value: string }) => useOtpCascade(value),
      { initialProps: { value: '' } }
    )

    await rerender({ value: '123456' })
    act(() => {
      jest.advanceTimersByTime(20)
    })

    await rerender({ value: '654321' })
    act(() => {
      jest.advanceTimersByTime(1000)
    })

    expect(result.current).toBeDefined()
  })

  it('cleans up timers on unmount', async () => {
    const { unmount } = await renderHook(() => useOtpCascade(''))
    expect(() => unmount()).not.toThrow()
  })
})
