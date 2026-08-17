import { useState, useEffect, useCallback } from 'react'
import { Platform } from 'react-native'
import { getLocales } from 'expo-localization'
import { useAuth } from '@/hooks/useAuth'
import { useGoogleSignIn } from '@/adapters/auth/useGoogleSignIn'
import { getAuthAdapter } from '@/adapters/auth'
import { authBackendUrl } from '@/api/backend-url'
import { messageForApiError, parseApiError } from '@/api/errors'
import type { RegisterPayload } from '@/api/auth/types'
import {
  trackSignupStep,
  trackSignupCompleted,
  trackOnboardingCompleted,
} from '@/analytics'
import i18n from '@/i18n'
import { COUNTRIES } from '@/utils/countries'
import { isValidEmail, isValidPhone } from '@/utils/validation'
import { router } from 'expo-router'

type Step = 'NAME' | 'METHOD' | 'VERIFY' | 'REGION_TERMS' | 'COACH_PREFERENCE' | 'PROFILE_DATA' | 'SUCCESS'

const detectedRegion = getLocales()?.[0]?.regionCode
const defaultCountry = COUNTRIES.find(c => c.code === detectedRegion) ? detectedRegion : 'PT'

// EU/EEA region codes — used to select the default data region
const EU_EEA_REGIONS = new Set([
  'AT', 'BE', 'BG', 'HR', 'CY', 'CZ', 'DK', 'EE', 'FI', 'FR',
  'DE', 'GR', 'HU', 'IE', 'IT', 'LV', 'LT', 'LU', 'MT', 'NL',
  'PL', 'PT', 'RO', 'SK', 'SI', 'ES', 'SE', 'IS', 'LI', 'NO', 'GB',
])
const defaultDataRegion: 'eu' | 'us' = EU_EEA_REGIONS.has(detectedRegion ?? '') ? 'eu' : 'us'

export function useSignupForm() {
  const { user, token, isLoading, register, updateUser, ensureValidToken, loginWithGoogle, loginWithApple } = useAuth()
  const signInWithGoogle = useGoogleSignIn()

  const [step, setStep] = useState<Step>('REGION_TERMS')
  const [name, setName] = useState('')
  const [signupMethod, setSignupMethod] = useState<'email' | 'phone' | 'google' | 'apple' | null>(null)
  const [email, setEmail] = useState('')
  const [phoneCountryCode, setPhoneCountryCode] = useState(defaultCountry)
  const [phoneBody, setPhoneBody] = useState('')
  const [verificationCode, setVerificationCode] = useState('')
  const [verificationId, setVerificationId] = useState('')
  const [codeSent, setCodeSent] = useState(false)
  const [isVerified, setIsVerified] = useState(false)
  const [countdown, setCountdown] = useState(0)
  const [dataRegion, setDataRegion] = useState<'eu' | 'us'>(defaultDataRegion)
  const [acceptTerms, setAcceptTerms] = useState(false)
  // Tracked apart from acceptTerms. App Review rejected 1.0 under 5.1.1(i)/5.1.2(i) and
  // stated that carrying the AI disclosure inside the Terms is not sufficient, so the
  // consent to voice audio reaching Google's AI has to be its own answer.
  const [acceptAi, setAcceptAi] = useState(false)
  const [acceptMarketing, setAcceptMarketing] = useState(false)
  const [termsError, setTermsError] = useState('')
  const [loading, setLoading] = useState(false)
  const [socialLoading, setSocialLoading] = useState<'google' | 'apple' | null>(null)
  const [error, setError] = useState('')
  const [accountExists, setAccountExists] = useState(false)
  const [fieldError, setFieldError] = useState('')
  const [stepDirection, setStepDirection] = useState<'forward' | 'backward'>('forward')
  const [savedGoogleToken, setSavedGoogleToken] = useState<string | null>(null)
  const [savedGoogleIdToken, setSavedGoogleIdToken] = useState<string | undefined>()
  const [savedAppleIdentityToken, setSavedAppleIdentityToken] = useState<string | null>(null)

  // One effect instead of instrumenting fourteen setStep() calls — and it also
  // catches the backward moves, which are the interesting ones: a step people
  // keep returning to is a step that isn't explaining itself.
  useEffect(() => {
    trackSignupStep(step)
  }, [step])

  // Coach preference state
  const [coachGender, setCoachGender] = useState<'male' | 'female' | null>(null)
  const [coachVoice, setCoachVoice] = useState<string | null>(null)

  // Profile data state
  const [country, setCountry] = useState('')
  const [dateOfBirth, setDateOfBirth] = useState('')
  const [gender, setGender] = useState('')

  const needsVerification = signupMethod !== null && signupMethod !== 'google'
  const verified = signupMethod === 'google' || isVerified
  const verificationSent = codeSent || countdown > 0 || verificationCode.length > 0

  const selectedCountry = COUNTRIES.find(c => c.code === phoneCountryCode)

  const getFullPhoneNumber = useCallback(() => {
    if (!phoneBody.trim()) return ''
    return `${selectedCountry?.phoneCode || ''}${phoneBody.replace(/[^0-9]/g, '')}`
  }, [phoneBody, selectedCountry])

  // The signup pre-check has three outcomes, not two. "This email already has an
  // account" is not a dead end: the token in hand is proof the caller owns that
  // mailbox, so we log in with it instead of stranding them on a signup screen for
  // an account they already have. /auth/google and /auth/apple attach the credential
  // to the account they find, so from the next sign-in on it matches on the provider
  // id directly and the email never comes into it.
  const ssoVerification = async (
    accessToken: string,
    idToken?: string,
    type: 'google' | 'apple' = 'google',
  ): Promise<'can-signup' | 'logged-in' | 'failed'> => {
    const fallbackKey = type === 'apple' ? 'err_apple_token_invalid' : 'err_google_token_invalid'
    const url = `${authBackendUrl}/auth/verifications/sso`
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ idToken: idToken ?? accessToken, accessToken, type }),
    })
    if (response.ok) return 'can-signup'

    const data = await response.json().catch(() => ({}))
    const { code } = parseApiError(data)
    if (code === 'EMAIL_ALREADY_EXISTS' || code === 'ACCOUNT_ALREADY_EXISTS') {
      try {
        if (type === 'apple') await loginWithApple(idToken ?? accessToken)
        else await loginWithGoogle(accessToken)
        return 'logged-in'
      } catch (e) {
        // Falls through to the login error (e.g. an unverified email, or a private
        // relay address that matches no account) — which is the accurate one.
        setError(messageForApiError(e, fallbackKey))
        return 'failed'
      }
    }
    setError(messageForApiError(data, fallbackKey))
    return 'failed'
  }

  const handleMethodSelect = useCallback((method: 'email' | 'phone') => {
    if (signupMethod === method) return
    setSignupMethod(method)
    setError('')
    setFieldError('')
    setAccountExists(false)
    setCodeSent(false)
    setIsVerified(false)
    setVerificationId('')
    setVerificationCode('')
    setCountdown(0)
  }, [signupMethod])

  const handleGoogleLogin = useCallback(async () => {
    if (loading || socialLoading) return
    setSocialLoading('google')
    setError('')
    try {
      const { accessToken, idToken, profile } = await signInWithGoogle()

      if (await ssoVerification(accessToken, idToken) !== 'can-signup') return

      setSavedGoogleToken(accessToken)
      setSavedGoogleIdToken(idToken)
      setSignupMethod('google')
      setVerificationId('google_sso')
      setCodeSent(true)
      setIsVerified(true)
      setStep('REGION_TERMS')
      setError('')

      if (profile?.email) setEmail(profile.email)
      if (profile?.name && !name) setName(profile.name)
    } catch (e: unknown) {
      const msg = String(e)
      if (!msg.includes('cancelled') && !msg.includes('failed or was cancelled')) {
        setError(i18n.t('err_google_token_invalid'))
      }
    } finally {
      setSocialLoading(null)
    }
  }, [loading, socialLoading, step, name, signInWithGoogle, loginWithGoogle])

  const handleAppleLogin = useCallback(async () => {
    if (loading || socialLoading) return
    if (Platform.OS !== 'ios') return
    setSocialLoading('apple')
    setError('')
    try {
      const result = await getAuthAdapter().signInWithApple()

      if (await ssoVerification(result.identityToken, result.identityToken, 'apple') !== 'can-signup') return

      setSavedAppleIdentityToken(result.identityToken)
      setSignupMethod('apple')
      setVerificationId('apple_sso')
      setCodeSent(true)
      setIsVerified(true)
      setStep('REGION_TERMS')
      setError('')

      if (result.user?.email) setEmail(result.user.email)
      if (result.user?.name && !name) setName(result.user.name)
    } catch (e: unknown) {
      const msg = String(e)
      if (!msg.includes('cancelled') && !msg.includes('ERR_REQUEST_CANCELED')) {
        setError(i18n.t('err_apple_token_invalid'))
      }
    } finally {
      setSocialLoading(null)
    }
  }, [loading, socialLoading, name, loginWithApple])

  useEffect(() => {
    let timer: ReturnType<typeof setInterval>
    if (countdown > 0) {
      timer = setInterval(() => setCountdown(p => p - 1), 1000)
    }
    return () => { if (timer) clearInterval(timer) }
  }, [countdown])

  const handleSendCode = useCallback(async () => {
    setLoading(true)
    setError('')
    setAccountExists(false)
    try {
      const payload: { type: 'email' | 'phone'; event: string; email?: string; phoneNumber?: string } = {
        type: signupMethod as 'email' | 'phone',
        event: 'signup',
      }
      if (signupMethod === 'email') payload.email = email
      else payload.phoneNumber = getFullPhoneNumber()

      const backendUrl = `${authBackendUrl}/auth/verifications/request`
      const resp = await fetch(backendUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (!resp.ok) {
        throw await resp.json().catch(() => ({}))
      }
      const respData = await resp.json() as { verificationId?: string }
      if (respData.verificationId) setVerificationId(respData.verificationId)
      setCountdown(30)
      setCodeSent(true)
      setVerificationCode('')
    } catch (e: unknown) {
      setError(messageForApiError(e, 'error_sending_code'))
      throw e
    } finally {
      setLoading(false)
    }
  }, [signupMethod, email, getFullPhoneNumber])

  const handleVerifyCode = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const payload: { type: 'email' | 'phone'; event: string; code: string; email?: string; phoneNumber?: string } = {
        type: signupMethod as 'email' | 'phone',
        event: 'signup',
        code: verificationCode,
      }
      if (signupMethod === 'email') payload.email = email
      else payload.phoneNumber = getFullPhoneNumber()

      const verifyBackendUrl = `${authBackendUrl}/auth/verifications/verify`
      const resp = await fetch(verifyBackendUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (!resp.ok) {
        throw await resp.json().catch(() => ({}))
      }
      setIsVerified(true)
    } catch (e: unknown) {
      const { code } = parseApiError(e)
      if (code === 'EMAIL_ALREADY_EXISTS' || code === 'PHONE_ALREADY_EXISTS') {
        setAccountExists(true)
      }
      setError(messageForApiError(e, 'invalid_code'))
    } finally {
      setLoading(false)
    }
  }, [signupMethod, verificationCode, email, getFullPhoneNumber])

  // Phase 1: Register with Auth Service (called at end of REGION_TERMS step)
  const handleRegister = useCallback(async () => {
    if (!acceptTerms) {
      setTermsError(i18n.t('error_accept_terms') || 'Please accept the terms')
      return
    }
    if (!acceptAi) {
      setTermsError(i18n.t('error_accept_ai') || 'Please agree to how your voice is processed')
      return
    }
    setTermsError('')
    setLoading(true)
    setError('')
    try {
      const payload: RegisterPayload = {
        name,
        preferredLanguage: i18n.locale,
        termsAndConditionsAccepted: acceptTerms,
        dataRegion,
        aiAccepted: acceptAi,
        marketingAccepted: acceptMarketing,
      }
      if (signupMethod === 'google') {
        payload.googleIdToken = savedGoogleIdToken ?? undefined
        payload.googleAccessToken = savedGoogleToken ?? undefined
        payload.email = email || undefined
      } else if (signupMethod === 'apple') {
        payload.appleIdentityToken = savedAppleIdentityToken ?? undefined
        payload.email = email || undefined
      } else if (signupMethod === 'email') {
        payload.email = email
        payload.emailVerificationId = verificationId
      } else if (signupMethod === 'phone') {
        payload.phoneNumber = getFullPhoneNumber()
        payload.phoneVerificationId = verificationId
      }
      await register(payload)
      // The account exists from here — everything after is profile setup.
      trackSignupCompleted(signupMethod ?? 'email', dataRegion, acceptMarketing)
      setStep('COACH_PREFERENCE')
    } catch (e: unknown) {
      setError(messageForApiError(e, 'registration_failed'))
    } finally {
      setLoading(false)
    }
  }, [acceptTerms, acceptAi, name, signupMethod, savedGoogleIdToken, savedGoogleToken, savedAppleIdentityToken, email, verificationId, getFullPhoneNumber, acceptMarketing, dataRegion, register])

  // Save coach preference and start session (voice session path)
  const handleContinueWithVoice = useCallback(async () => {
    if (!coachGender || !coachVoice) {
      setError(i18n.t('select_coach_preference') || 'Please select a gender and voice')
      return
    }
    setLoading(true)
    setError('')
    try {
      await ensureValidToken()
      await updateUser({ coachGender, coachVoice } as Parameters<typeof updateUser>[0])
      router.replace({ pathname: '/(tabs)/session', params: { autoStart: 'true', sessionType: 'onboarding' } })
    } catch (e: unknown) {
      setError(messageForApiError(e, 'update_failed'))
    } finally {
      setLoading(false)
    }
  }, [coachGender, coachVoice, ensureValidToken, updateUser])

  // Go to PROFILE_DATA without saving (manual path)
  const handleFillInManually = useCallback(() => {
    setStep('PROFILE_DATA')
  }, [])

  // Save profile data with coach fallback guard (manual path completion)
  const handleManualProfileSubmit = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      await ensureValidToken()
      // Coach fallback: if user didn't select coach gender, derive from profile gender
      const resolvedCoachGender = coachGender || (gender as 'male' | 'female') || 'female'
      const resolvedCoachVoice = coachVoice || (resolvedCoachGender === 'female' ? 'gacrux' : 'algieba')
      await updateUser({
        country,
        dateOfBirth,
        gender,
        coachGender: resolvedCoachGender,
        coachVoice: resolvedCoachVoice,
      } as Parameters<typeof updateUser>[0])
      // Profile is complete — the end of the wizard, and the denominator for
      // "how many of the people who signed up ever reached the app".
      trackOnboardingCompleted()
      setStep('SUCCESS')
    } catch (e: unknown) {
      setError(messageForApiError(e, 'update_failed'))
    } finally {
      setLoading(false)
    }
  }, [country, dateOfBirth, gender, coachGender, coachVoice, ensureValidToken, updateUser])

  const handleNext = useCallback(async () => {
    setError('')
    setStepDirection('forward')

    if (step === 'NAME') {
      if (!name.trim()) { setError(i18n.t('required_field') || 'Name is required'); return }
      setStep('METHOD')
    } else if (step === 'METHOD') {
      if (!signupMethod) return
      // For email/phone, validate and send code before moving to VERIFY
      if (signupMethod === 'email') {
        if (!isValidEmail(email)) { setFieldError(i18n.t('invalid_email') || 'Please enter a valid email'); return }
      }
      if (signupMethod === 'phone') {
        if (!isValidPhone(phoneBody)) { setFieldError(i18n.t('invalid_phone') || 'Please enter a valid phone number'); return }
      }
      // Send code and move to VERIFY (which will show OTP input since verificationSent will be true)
      try {
        await handleSendCode()
        setStep('VERIFY')
      } catch {
        // Error already set by handleSendCode, stay on METHOD
      }
    } else if (step === 'VERIFY') {
      if (!verified) {
        if (verificationCode.length < 6) { setError(i18n.t('invalid_code') || 'Enter a valid code'); return }
        await handleVerifyCode()
        return
      }
      setStep('REGION_TERMS')
    } else if (step === 'REGION_TERMS') {
      await handleRegister()
    } else if (step === 'PROFILE_DATA') {
      if (!dateOfBirth || !gender) {
        setError(i18n.t('required_field') || 'Please fill in all required fields')
        return
      }
      await handleManualProfileSubmit()
    }
  }, [step, name, signupMethod, verified, verificationSent, email, phoneBody, verificationCode,
    dateOfBirth, gender, handleSendCode, handleVerifyCode, handleRegister, handleManualProfileSubmit])

  const handleBack = useCallback(() => {
    setError('')
    setFieldError('')
    setStepDirection('backward')

    if (step === 'METHOD') {
      if (signupMethod) {
        // Method selected — just reset it and stay on METHOD to show buttons
        setSignupMethod(null)
        setFieldError('')
      } else {
        // No method selected — go back to NAME
        setSignupMethod(null)
        setCodeSent(false)
        setIsVerified(false)
        setVerificationId('')
        setVerificationCode('')
        setCountdown(0)
        setAccountExists(false)
        setFieldError('')
        setStep('NAME')
      }
    } else if (step === 'VERIFY') {
      // Reset verification state and go to METHOD
      setCodeSent(false)
      setIsVerified(false)
      setVerificationId('')
      setVerificationCode('')
      setCountdown(0)
      setAccountExists(false)
      setSignupMethod(null)
      setStep('METHOD')
    } else if (step === 'REGION_TERMS') {
      // Reset verification and method, go to METHOD
      setCodeSent(false)
      setIsVerified(false)
      setVerificationId('')
      setVerificationCode('')
      setCountdown(0)
      setAccountExists(false)
      setSignupMethod(null)
      setStep('METHOD')
    } else if (step === 'COACH_PREFERENCE') {
      setStep('REGION_TERMS')
    } else if (step === 'PROFILE_DATA') {
      setStep('COACH_PREFERENCE')
    }
  }, [step, signupMethod])


  const getNextLabel = useCallback(() => {
    if (step === 'NAME') return i18n.t('next') || 'Continue'
    if (step === 'METHOD') {
      if (!signupMethod) return i18n.t('continue') || 'Continue'
      if (signupMethod === 'google') return i18n.t('continue') || 'Continue'
      return i18n.t('continue') || 'Continue'
    }
    if (step === 'VERIFY') {
      if (!verified) return i18n.t('verify') || 'Verify'
      return i18n.t('continue') || 'Continue'
    }
    if (step === 'REGION_TERMS') return i18n.t('create_account') || 'Create Account'
    if (step === 'PROFILE_DATA') return i18n.t('done') || 'Done'
    return i18n.t('next') || 'Continue'
  }, [step, signupMethod, verified, verificationSent])

  const handleNameSubmit = useCallback(() => {
    if (!name.trim()) { setError(i18n.t('required_field') || 'Name is required'); return }
    setStep('METHOD')
  }, [name])

  const handleEmailChange = useCallback((t: string) => {
    setEmail(t)
    setFieldError('')
  }, [])

  const handlePhoneBodyChange = useCallback((t: string) => {
    setPhoneBody(t)
    setFieldError('')
  }, [])

  const handleVerificationCodeChange = useCallback((t: string) => {
    setVerificationCode(t)
  }, [])

  return {
    step,
    name,
    signupMethod,
    email,
    phoneBody,
    phoneCountryCode,
    verificationCode,
    verificationSent,
    verified,
    countdown,
    acceptTerms,
    acceptAi,
    acceptMarketing,
    dataRegion,
    termsError,
    loading,
    socialLoading,
    error,
    accountExists,
    fieldError,
    stepDirection,
    selectedCountry,
    // Coach preference
    coachGender,
    coachVoice,
    // Profile data
    country,
    dateOfBirth,
    gender,
    // Handlers
    handleNameSubmit,
    handleMethodSelect,
    handleEmailChange,
    handlePhoneBodyChange,
    setPhoneCountryCode,
    handleVerificationCodeChange,
    handleGoogleLogin,
    handleAppleLogin,
    handleSendCode,
    handleVerifyCode,
    handleRegister,
    handleContinueWithVoice,
    handleFillInManually,
    handleManualProfileSubmit,
    handleNext,
    handleBack,
    getNextLabel,
    setName,
    setAcceptTerms,
    setAcceptAi,
    setAcceptMarketing,
    setDataRegion,
    setTermsError,
    setCoachGender,
    setCoachVoice,
    setCountry,
    setDateOfBirth,
    setGender,
  }
}
