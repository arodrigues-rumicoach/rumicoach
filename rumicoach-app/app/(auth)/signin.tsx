import { useState, useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import { KeyboardAvoidingView, Platform, Pressable, Keyboard, ScrollView } from 'react-native'
import { YStack, XStack, Text, Separator } from 'tamagui'
import { router, Link } from 'expo-router'
import { Mail, Phone, ChevronLeft } from 'lucide-react-native'
import { getLocales } from 'expo-localization'
import { useAuth } from '../../src/hooks/useAuth'
import { useGoogleSignIn } from '../../src/adapters/auth/useGoogleSignIn'
import i18n from '../../src/i18n'
import { messageForApiError } from '../../src/api/errors'
import { ThemedButton, ThemedInput, AppleIcon, GoogleIcon, ThemedIconButton, AnimatedStack, Heading, CodeVerified, CountryCodePicker, GlassCard, BackButton } from '@/components/atoms'
import { InlineToast, OtpInput } from '@/components/molecules'
import { BlurView } from 'expo-blur'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withSequence,
  withTiming,
  Easing,
  FadeInLeft,
  FadeOut,
} from 'react-native-reanimated'
import { useBlurTarget } from '../../src/context/BlurContext'
import { isValidEmail, isValidPhone } from '../../src/utils/validation'
import { COUNTRIES } from '../../src/utils/countries'
import { useSettings } from '@/hooks/useSettings'
import { haptic } from '@/utils/haptics'
import { isWeb } from '@/adapters/platform'

type LoginView = 'main' | 'email' | 'phone' | 'otp'
type ViewDirection = 'forward' | 'backward' | 'up'

const detectedRegion = getLocales()?.[0]?.regionCode
const defaultCountry = COUNTRIES.find(c => c.code === detectedRegion) ? detectedRegion : 'PT'

function getStackDirection(from: LoginView, to: LoginView): ViewDirection {
  if (to === 'otp') return 'up'
  if (from === 'main' || to === 'main') return 'forward'
  return 'backward'
}

export default function SigninScreen() {
  const blurTargetRef = useBlurTarget()
  const { colorScheme } = useSettings()
  const { user, token, isLoading, loginWithVerificationCode, requestVerificationCode, loginWithGoogle, loginWithApple } = useAuth()
  const signInWithGoogle = useGoogleSignIn()
  const [view, setView] = useState<LoginView>('main')
  const [viewDirection, setViewDirection] = useState<ViewDirection>('up')
  const prevViewRef = useRef<LoginView>('main')
  const [email, setEmail] = useState('')
  const [phoneCountryCode, setPhoneCountryCode] = useState(defaultCountry)
  const [phoneBody, setPhoneBody] = useState('')
  const [otp, setOtp] = useState('')
  const [loading, setLoading] = useState(false)
  const [socialLoading, setSocialLoading] = useState<'google' | 'apple' | null>(null)
  const [error, setError] = useState('')
  const [fieldError, setFieldError] = useState('')
  const [codeType, setCodeType] = useState<'email' | 'phone'>('email')
  const [successKey, setSuccessKey] = useState(0)
  const [showVerified, setShowVerified] = useState(false)
  const prevUserRef = useRef<typeof user>(null)
  const isSubmittingRef = useRef(false)

  const cardScale = useSharedValue(1)

  useEffect(() => {
    if (view !== prevViewRef.current) {
      setViewDirection(getStackDirection(prevViewRef.current, view))
      prevViewRef.current = view
      cardScale.value = withSequence(
        withTiming(0.985, { duration: 100, easing: Easing.out(Easing.cubic) }),
        withSpring(1, { damping: 16, stiffness: 240 })
      )
    }
  }, [view, cardScale])

  useLayoutEffect(() => {
    if (!prevUserRef.current && user) {
      setShowVerified(true)
      setSuccessKey(k => k + 1)
    } else if (prevUserRef.current && !user) {
      setShowVerified(false)
    }
    prevUserRef.current = user
  }, [user])

  const handleVerifiedComplete = useCallback(() => {
    setShowVerified(false)
  }, [])

  const selectedCountry = COUNTRIES.find(c => c.code === phoneCountryCode)

  const getFullPhoneNumber = useCallback(() => {
    if (!phoneBody.trim()) return ''
    return `${selectedCountry?.phoneCode || ''}${phoneBody.replace(/[^0-9]/g, '')}`
  }, [phoneBody, selectedCountry])

  const handleGoogleLogin = async () => {
    if (loading || socialLoading) return
    if (__DEV__) console.log('[LOGIN] handleGoogleLogin started')
    setSocialLoading('google')
    setError('')
    try {
      const result = await signInWithGoogle()
      if (__DEV__) {
        console.log('[LOGIN] signInWithGoogle success accessTokenPresent=', !!result.accessToken)
        console.log('[LOGIN] signInWithGoogle success idTokenPresent=', !!result.idToken)
      }
      await loginWithGoogle(result.accessToken)
      if (__DEV__) console.log('[LOGIN] loginWithGoogle completed')
    } catch (e: unknown) {
      if (__DEV__) console.error('[LOGIN] handleGoogleLogin error:', e)
      const msg = String(e)
      if (!msg.includes('cancelled') && !msg.includes('failed or was cancelled')) {
        setError(messageForApiError(e, 'login_failed'))
      }
    } finally {
      setSocialLoading(null)
    }
  }

  const handleAppleLogin = async () => {
    if (loading || socialLoading) return
    if (__DEV__) console.log('[LOGIN] handleAppleLogin started')
    setSocialLoading('apple')
    setError('')
    try {
      const { getAuthAdapter } = await import('../../src/adapters/auth')
      const result = await getAuthAdapter().signInWithApple()
      if (__DEV__) {
        console.log('[LOGIN] signInWithApple success identityTokenPresent=', !!result.identityToken)
      }
      await loginWithApple(result.identityToken, result.user?.email, result.user?.name)
      if (__DEV__) console.log('[LOGIN] loginWithApple completed')
    } catch (e: unknown) {
      if (__DEV__) console.error('[LOGIN] handleAppleLogin error:', e)
      const msg = String(e)
      if (!msg.includes('cancelled') && !msg.includes('ERR_REQUEST_CANCELED')) {
        setError(messageForApiError(e, 'login_failed'))
      }
    } finally {
      setSocialLoading(null)
    }
  }

  const goBack = () => {
    haptic.selection()
    setError('')
    setFieldError('')
    if (view === 'otp') {
      setView(codeType === 'email' ? 'email' : 'phone')
      setOtp('')
    } else {
      setView('main')
      setEmail('')
      setPhoneBody('')
    }
  }

  const goToContact = (type: 'email' | 'phone') => {
    haptic.light()
    setCodeType(type)
    setView(type)
  }

  const handleSendCode = async () => {
    const identifier = codeType === 'email' ? email : getFullPhoneNumber()
    if (codeType === 'email' && !isValidEmail(email)) {
      setFieldError(i18n.t('invalid_email') || 'Please enter a valid email')
      return
    }
    if (codeType === 'phone' && !isValidPhone(phoneBody)) {
      setFieldError(i18n.t('invalid_phone') || 'Please enter a valid phone number')
      return
    }
    setLoading(true)
    setError('')
    try {
      await requestVerificationCode(codeType, identifier)
      // Clear any previous digits: the codes are single-use, so leaving a stale one in the
      // boxes lets "resend, then verify" silently resubmit the code that just failed.
      setOtp('')
      setView('otp')
    } catch (e: unknown) {
      setError(messageForApiError(e, 'error_sending_code'))
    } finally {
      setLoading(false)
    }
  }

  const handleVerifyCode = useCallback(async () => {
    if (isSubmittingRef.current) return
    isSubmittingRef.current = true
    setLoading(true)
    setError('')
    const identifier = codeType === 'email' ? email : getFullPhoneNumber()
    try {
      await loginWithVerificationCode(codeType, identifier, otp)
      setOtp('')
    } catch (e: unknown) {
      setError(messageForApiError(e, 'invalid_code'))
    } finally {
      setLoading(false)
      isSubmittingRef.current = false
    }
  }, [codeType, email, otp, getFullPhoneNumber, loginWithVerificationCode])

  const identifier = codeType === 'email' ? email : getFullPhoneNumber()

  const cardAnimatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: cardScale.value }],
  }))

  const headingTitle = view === 'otp' ? (i18n.t('verification') || 'Verification') : (i18n.t('signin') || 'Sign In')

  return (
    <KeyboardAvoidingView style={{ flex: 1, backgroundColor: 'transparent' }} behavior="padding">
      <ScrollView
        style={{ backgroundColor: 'transparent' }}
        contentContainerStyle={{ flexGrow: 1, justifyContent: 'center', alignItems: 'center', padding: 16 }}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
      >
        <Reanimated.View style={[cardAnimatedStyle, { width: '100%', maxWidth: 400, borderRadius: 24 }]}>
          <GlassCard variant='light' blurTarget={blurTargetRef} padding={24} gap={20} borderRadius={24}>
            <XStack
              width="100%"
              alignItems="center"
              justifyContent="center"
              position="relative"
              minHeight={44}
            >
              {view !== 'main' && (
                <Reanimated.View
                  entering={isWeb ? undefined : FadeInLeft.duration(260).springify().damping(20).stiffness(200)}
                  exiting={isWeb ? undefined : FadeOut.duration(140)}
                  style={{ position: 'absolute', left: 0, top: 0, bottom: 0, justifyContent: 'center' }}
                >
                  <BackButton onPress={goBack} />
                </Reanimated.View>
              )}
              <Heading color="$onGlass" fontSize={26} fontWeight="bold" letterSpacing={1} textAlign="center">
                {headingTitle}
              </Heading>
            </XStack>

            {error && (
              <InlineToast message={error} visible={!!error} />
            )}

            {view === 'main' && (
              <AnimatedStack key="main" direction="forward">
                <YStack gap='$6'>
                  <YStack gap={12} width="100%">
                    <ThemedButton variant="solid" fullWidth icon={<GoogleIcon size={20} />} onPress={handleGoogleLogin} disabled={loading} loading={socialLoading === 'google'}>
                      {i18n.t('continue_with_google') || 'Continue with Google'}
                    </ThemedButton>

                    {Platform.OS === 'ios' && (
                      <ThemedButton variant="solid" fullWidth icon={<AppleIcon size={20} />} onPress={handleAppleLogin} disabled={loading} loading={socialLoading === 'apple'}>
                        {i18n.t('continue_with_apple') || 'Continue with Apple'}
                      </ThemedButton>
                    )}

                    <ThemedButton variant="solid" fullWidth icon={<Mail size={20} />} onPress={() => goToContact('email')}>
                      {i18n.t('use_email') || 'Use Email'}
                    </ThemedButton>

                    <ThemedButton variant="solid" fullWidth icon={<Phone size={20} />} onPress={() => goToContact('phone')}>
                      {i18n.t('use_phone') || 'Use phone number'}
                    </ThemedButton>
                  </YStack>

                  <YStack width="100%" gap={16}>

                    <Separator borderColor='$onGlassTertiary' />
                    <Text color="$onGlassSecondary" textAlign="center" fontSize={13}>
                      {i18n.t('dont_have_account') || "Don't have an account?"}
                    </Text>

                    <Link href="/(auth)/signup" asChild>
                      <ThemedButton variant="solid" fullWidth>
                        {i18n.t('register') || 'Sign Up'}
                      </ThemedButton>
                    </Link>
                  </YStack>
                </YStack>
              </AnimatedStack>
            )}

            {(view === 'email' || view === 'phone') && (
              <AnimatedStack key="contact" direction={viewDirection}>
                <YStack gap="$4" width="100%">
                  {view === 'email' ? (
                    <ThemedInput
                      variant="light"
                      placeholder={i18n.t('email_placeholder') || 'name@example.com'}
                      value={email}
                      onChangeText={(t) => { setEmail(t); setFieldError('') }}
                      keyboardType="email-address"
                      autoCapitalize="none"
                      color="$onGlass" autoFocus
                      icon={<Mail size={20} color="#4A4540" />}
                    />
                  ) : (
                    <YStack gap="$2" width="100%">
                      <XStack gap="$2" alignItems="center">
                        <CountryCodePicker
                          value={phoneCountryCode ?? ''}
                          onChange={setPhoneCountryCode}
                          style={{ minWidth: 120 }}
                        />
                        <ThemedInput
                          variant="light"
                          flex={1}
                          placeholder={i18n.t('phone_placeholder') || '234 567 8900'}
                          value={phoneBody}
                          onChangeText={(t) => { setPhoneBody(t); setFieldError('') }}
                          keyboardType="phone-pad"
                          color="$onGlass"
                          icon={<Phone size={20} color="#4A4540" />}
                        />
                      </XStack>
                      {fieldError && (
                        <InlineToast message={fieldError} visible={!!fieldError} />
                      )}
                    </YStack>
                  )}

                  {view === 'email' && fieldError && (
                    <InlineToast message={fieldError} visible={!!fieldError} />
                  )}

                  <ThemedButton variant="solid" fullWidth onPress={handleSendCode} disabled={loading} loading={loading}>
                    {i18n.t('continue') || 'Continue'}
                  </ThemedButton>
                </YStack>
              </AnimatedStack>
            )}

            {view === 'otp' && (
              <AnimatedStack key={showVerified ? 'otp-success' : 'otp-input'} direction="up">
                {showVerified ? (
                  <CodeVerified
                    title={i18n.t('code_verified') || 'Code verified'}
                    subtitle={i18n.t('signing_you_in') || 'Signing you in...'}
                    accentColor={colorScheme.accent}
                    durationMs={900}
                    onComplete={handleVerifiedComplete}
                  />
                ) : (
                  <YStack gap="$4" width="100%">
                    <Text color="$onGlassSecondary" textAlign="center" fontSize={14}>
                      {(i18n.t('enter_code_sent_to') || 'Enter the code sent to')}
                    </Text>
                    <Text color="$onGlass" textAlign="center" fontWeight="bold" fontSize={15}>
                      {identifier}
                    </Text>

                    <OtpInput
                      value={otp}
                      onChange={setOtp}
                      error={!!error}
                      autoFocus
                      onComplete={handleVerifyCode}
                      variant="light"
                    />

                    <ThemedButton
                      variant="solid"
                      fullWidth
                      glow={otp.length === 6 && !loading}
                      onPress={handleVerifyCode}
                      disabled={otp.length !== 6 || loading}
                      loading={loading}
                    >
                      {i18n.t('verify_login') || 'Verify & Sign In'}
                    </ThemedButton>
                  </YStack>
                )}
              </AnimatedStack>
            )}
          </GlassCard>
        </Reanimated.View>
      </ScrollView>
    </KeyboardAvoidingView>
  )
}
