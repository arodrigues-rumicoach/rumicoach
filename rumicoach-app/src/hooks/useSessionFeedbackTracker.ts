import { useState, useEffect, useCallback, useRef } from 'react'
import type { ChatStatus } from '../context/SessionContext'

const MIN_DURATION_FOR_FEEDBACK_SECONDS = 60

// Track sessions for which the feedback modal has already been presented so it
// is never shown twice for the same session, even if the component remounts or
// the disconnect effect fires multiple times.
const feedbackPresentedForSessionIds = new Set<string>()

/** Resets the internal set of session ids already presented for feedback. */
export function resetSessionFeedbackTracker() {
  feedbackPresentedForSessionIds.clear()
}

export function useSessionFeedbackTracker(
  status: ChatStatus,
  sessionId: string | null,
  durationSeconds: number,
  // While true, a due feedback modal is HELD rather than shown. The session-end
  // summary card is the reason this exists: the rating modal used to pop right
  // on top of the card the user had just been told to read (QA) — so the caller
  // passes "the card is on screen and the user is looking at it", and the modal
  // waits for the card to be dismissed or the user to move on.
  holdWhileVisible = false,
) {
  const [lastActiveSessionId, setLastActiveSessionId] = useState<string | null>(null)
  const [lastActiveSessionDuration, setLastActiveSessionDuration] = useState(0)
  const [showFeedback, setShowFeedback] = useState(false)
  const [pendingFeedback, setPendingFeedback] = useState(false)

  // Keep a stable reference to the latest session id so dismissFeedback can
  // register it without depending on the state value.
  const lastActiveSessionIdRef = useRef<string | null>(null)
  useEffect(() => {
    lastActiveSessionIdRef.current = lastActiveSessionId
  }, [lastActiveSessionId])

  useEffect(() => {
    if (status !== 'disconnected' && sessionId) {
      setLastActiveSessionId(sessionId)
      setLastActiveSessionDuration(durationSeconds)
    } else if (status === 'disconnected' && lastActiveSessionId) {
      if (lastActiveSessionDuration >= MIN_DURATION_FOR_FEEDBACK_SECONDS) {
        if (!feedbackPresentedForSessionIds.has(lastActiveSessionId)) {
          feedbackPresentedForSessionIds.add(lastActiveSessionId)
          if (holdWhileVisible) {
            setPendingFeedback(true)
          } else {
            setShowFeedback(true)
          }
        }
      } else {
        setLastActiveSessionId(null)
      }
    }
  }, [status, sessionId, durationSeconds, lastActiveSessionId, lastActiveSessionDuration, holdWhileVisible])

  // Release a held modal the moment the reason for holding it is gone — the
  // user dismissed the summary card or navigated away from it. A new session
  // starting while it is held cancels it instead: rating the previous session
  // over the next one's opening would be worse than losing the rating.
  useEffect(() => {
    if (!pendingFeedback) return
    if (status !== 'disconnected') {
      setPendingFeedback(false)
      return
    }
    if (!holdWhileVisible) {
      setPendingFeedback(false)
      setShowFeedback(true)
    }
  }, [pendingFeedback, holdWhileVisible, status])

  const dismissFeedback = useCallback(() => {
    const id = lastActiveSessionIdRef.current
    if (id) {
      feedbackPresentedForSessionIds.add(id)
    }
    setShowFeedback(false)
    setLastActiveSessionId(null)
  }, [])

  return { lastActiveSessionId, showFeedback, dismissFeedback }
}
