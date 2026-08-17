import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { FeedbackModal } from '@/components/organisms/FeedbackModal'
import { Toast } from '@/components/molecules'
import { useAuth } from '@/hooks/useAuth'
import { useSettings } from '@/hooks/useSettings'
import { useShakeToReport } from '@/hooks/useShakeToReport'
import i18n from '@/i18n'

interface FeedbackContextValue {
  openFeedback: () => void
}

const FeedbackReportContext = createContext<FeedbackContextValue | null>(null)

export function useFeedback(): FeedbackContextValue {
  const ctx = useContext(FeedbackReportContext)
  if (!ctx) throw new Error('useFeedback must be used inside FeedbackProvider')
  return ctx
}

/**
 * Owns the feedback form for the whole app.
 *
 * It lives here rather than on the settings screen because shake-to-report has
 * to work wherever the user hit the problem — which is the only place the
 * `screen` field in the report is worth anything.
 */
export function FeedbackProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  const { shakeToReport } = useSettings()
  const [visible, setVisible] = useState(false)
  const [sent, setSent] = useState(false)
  // Whether this form was opened by the shake gesture or from a menu. It rides
  // along on the submitted event, which is the only way to tell whether anyone
  // actually uses the gesture or whether it just fires by accident.
  const [byShake, setByShake] = useState(false)

  const openFeedback = useCallback(() => {
    setByShake(false)
    setVisible(true)
  }, [])

  const openFromShake = useCallback(() => {
    setByShake(true)
    setVisible(true)
  }, [])

  // Only while signed in: the endpoint needs a token, and a form that can only
  // fail is worse than no gesture. Also keeps the sensor off the auth screens.
  // Disabled while open, so shaking mid-report doesn't fight the form — and off
  // entirely when the user has turned the gesture off in App Settings.
  useShakeToReport(openFromShake, shakeToReport && !!user && !visible)

  const value = useMemo(() => ({ openFeedback }), [openFeedback])

  return (
    <FeedbackReportContext.Provider value={value}>
      {children}
      <FeedbackModal
        visible={visible}
        openedByShake={byShake}
        onClose={() => setVisible(false)}
        onSubmitSuccess={() => setSent(true)}
      />
      {sent && (
        <Toast
          message={i18n.t('feedback_submit_success') || 'Feedback submitted successfully'}
          type="success"
          onClose={() => setSent(false)}
        />
      )}
    </FeedbackReportContext.Provider>
  )
}
