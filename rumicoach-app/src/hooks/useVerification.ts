import { useState, useEffect, useCallback, useRef } from 'react'
import { getAuthApi } from '@/api/auth'
import type { User } from '@/api'
import i18n from '@/i18n'

type VerificationState = 'idle' | 'sending' | 'sent' | 'verifying' | 'verified' | 'error'

const RESEND_SECONDS = 30

interface UseVerificationOptions {
  type: 'email' | 'phone'
  identifier: string
  event?: string
  onVerified?: (updatedUser: User) => void
  /** Awaited for its effect only — whatever it resolves to is discarded, so this stays
   *  assignable from AuthContext's refreshUser, which returns the refreshed profile. */
  refreshUser?: () => Promise<unknown>
}

interface UseVerificationReturn {
  state: VerificationState
  countdown: number
  error: string
  sendCode: () => Promise<void>
  verifyAndUpdate: (code: string) => Promise<void>
  reset: () => void
}

export function useVerification({
  type,
  identifier,
  event = 'contacts_info_update',
  onVerified,
  refreshUser,
}: UseVerificationOptions): UseVerificationReturn {
  const [state, setState] = useState<VerificationState>('idle')
  const [countdown, setCountdown] = useState(0)
  const [error, setError] = useState('')
  const onVerifiedRef = useRef(onVerified)
  onVerifiedRef.current = onVerified
  const refreshUserRef = useRef(refreshUser)
  refreshUserRef.current = refreshUser
  const verificationIdRef = useRef<string>('')

  useEffect(() => {
    let timer: ReturnType<typeof setInterval>
    if (countdown > 0) {
      timer = setInterval(() => setCountdown((p) => p - 1), 1000)
    }
    return () => {
      if (timer) clearInterval(timer)
    }
  }, [countdown])

  const sendCode = useCallback(async () => {
    setState('sending')
    setError('')
    try {
      const authApi = getAuthApi()
      const verificationId = await authApi.requestVerificationCodeWithIdentifier(type, identifier, event)
      verificationIdRef.current = verificationId
      setCountdown(RESEND_SECONDS)
      setState('sent')
    } catch (e: unknown) {
      const msg = extractErrorMessage(e, 'error_sending_code')
      setError(msg)
      setState('error')
    }
  }, [type, identifier, event])

  const verifyAndUpdate = useCallback(
    async (code: string) => {
      setState('verifying')
      setError('')
      try {
        const authApi = getAuthApi()
        await authApi.verifyCode(type, identifier, code)
        const updatedUser = await authApi.verifyAndUpdateIdentifier(type, identifier, verificationIdRef.current)
        await refreshUserRef.current?.()
        setState('verified')
        onVerifiedRef.current?.(updatedUser)
      } catch (e: unknown) {
        const msg = extractErrorMessage(e, 'invalid_code')
        setError(msg)
        setState('sent')
      }
    },
    [type, identifier]
  )

  const reset = useCallback(() => {
    setState('idle')
    setCountdown(0)
    setError('')
  }, [])

  return { state, countdown, error, sendCode, verifyAndUpdate, reset }
}

function extractErrorMessage(e: unknown, fallbackKey: string): string {
  console.log({ e })
  if (typeof e === 'object' && e !== null && 'response' in e) {
    const resp = (e as { response?: { data?: { message?: string; code?: string } } }).response
    console.log({ resp })
    if (resp?.data?.code) return i18n.t(`err_${resp.data.code.toLowerCase()}`)
    if (resp?.data?.message) return resp.data.message
  }
  if (typeof e === 'object' && e !== null && 'message' in e) {
    const msg = (e as { message?: string }).message
    console.log({ msg })
    if (msg) return msg
  }
  return fallbackKey
}
