import { memo, useCallback, useLayoutEffect, useRef, useState } from 'react'
import { Platform } from 'react-native'
import { YStack, XStack, Text, Separator } from 'tamagui'
import { Mail, Phone } from 'lucide-react-native'
import { Heading, ThemedButton, ThemedInput, AppleIcon, GoogleIcon, CountryCodePicker, AnimatedStack, CodeVerified } from '@/components/atoms'
import { InlineToast, OtpInput } from '@/components/molecules'
import i18n from '@/i18n'
import { haptic } from '@/utils/haptics'
import type { ColorScheme } from '@/context/SettingsContext'
import { INK } from '@/styles/glass'

interface SignupMethodStepProps {
  name: string
  signupMethod: 'email' | 'phone' | 'google' | 'apple' | null
  email: string
  phoneBody: string
  phoneCountryCode: string | null
  verificationCode: string
  verificationSent: boolean
  verified: boolean
  countdown: number
  loading: boolean
  socialLoading: 'google' | 'apple' | null
  error: string
  fieldError: string
  colorScheme: ColorScheme
  onMethodSelect: (method: 'email' | 'phone') => void
  onEmailChange: (email: string) => void
  onPhoneBodyChange: (phone: string) => void
  onPhoneCountryCodeChange: (code: string) => void
  onVerificationCodeChange: (code: string) => void
  onGoogleLogin: () => void
  onAppleLogin: () => void
  onResendCode: () => void
  /**
   * Fires once the `Code verified` success animation has finished playing
   * after the OTP was accepted by the backend. The parent uses this to
   * advance the signup flow (e.g. show the terms step) only AFTER the
   * user has been visually informed of the success.
   */
  onVerified?: () => void
}

const VERIFIED_DISPLAY_MS = 2000

export const SignupMethodStep = memo(function SignupMethodStep({
  name, signupMethod, email, phoneBody, phoneCountryCode, verificationCode,
  verificationSent, verified, countdown, loading, socialLoading, error, fieldError, colorScheme,
  onMethodSelect, onEmailChange, onPhoneBodyChange, onPhoneCountryCodeChange,
  onVerificationCodeChange, onGoogleLogin, onAppleLogin, onResendCode, onVerified
}: SignupMethodStepProps) {
  const [showVerified, setShowVerified] = useState(false)
  const prevVerifiedRef = useRef(verified)

  useLayoutEffect(() => {
    if (!prevVerifiedRef.current && verified) {
      setShowVerified(true)
    } else if (prevVerifiedRef.current && !verified) {
      setShowVerified(false)
    }
    prevVerifiedRef.current = verified
  }, [verified])

  const handleMethodSelect = useCallback((method: 'email' | 'phone') => {
    haptic.light()
    onMethodSelect(method)
  }, [onMethodSelect])

  const handleGoogleLogin = useCallback(() => {
    haptic.medium()
    onGoogleLogin()
  }, [onGoogleLogin])

  const handleAppleLogin = useCallback(() => {
    haptic.medium()
    onAppleLogin()
  }, [onAppleLogin])

  const handleResendCode = useCallback(() => {
    if (countdown === 0) {
      haptic.light()
      onResendCode()
    }
  }, [countdown, onResendCode])

  const handleVerifiedComplete = useCallback(() => {
    setShowVerified(false)
    onVerified?.()
  }, [onVerified])

  const showMethodSelection = !verificationSent
  const showOtpInput = signupMethod !== null
    && signupMethod !== 'google'
    && verificationSent
    && !verified
    && !showVerified
  const showOtpSuccess = signupMethod !== null
    && signupMethod !== 'google'
    && verificationSent
    && showVerified

  return (
    <YStack gap="$3" width="100%">
      {showMethodSelection && (
        <>
          <Heading color="$onGlass" fontSize={20} textAlign="center" marginBottom="$2">
            {(i18n.t('hello_greeting') || 'Hello')}, <Text color="$accent">{name}</Text>!
          </Heading>
          <Text color="$onGlassSecondary" textAlign="center" fontSize={14} marginBottom="$2">
            {(i18n.t('how_to_signup') || 'How do you want to sign up?')}
          </Text>

          <AnimatedStack key={`method-${signupMethod ?? 'none'}`} direction="forward">
            <YStack gap="$4">
              {(!signupMethod || signupMethod === 'email') && (
                <YStack gap="$2">
                  <ThemedButton
                    variant='solid'
                    fullWidth
                    onPress={() => handleMethodSelect('email')}
                    style={{
                      backgroundColor: signupMethod === 'email' ? colorScheme.navIconBlur : colorScheme.primary,
                      borderWidth: signupMethod === 'email' ? 2 : 0,
                      borderColor: signupMethod === 'email' ? '$accent' : 'transparent',
                    }}
                    icon={<Mail size={20} color="#fff" />}
                  >
                    {(i18n.t('signup_with_email') || 'Sign up with Email')}
                  </ThemedButton>
                  {signupMethod === 'email' && (
                    <YStack gap="$1" width="100%">
                      <ThemedInput
                        variant="light"
                        size="$5"
                        width="100%"
                        placeholder={i18n.t('email_placeholder') || 'name@example.com'}
                        value={email}
                        onChangeText={(t) => onEmailChange(t)}
                        keyboardType="email-address"
                        autoCapitalize="none"
                        color="$onGlass"
                        autoFocus
                        icon={<Mail size={20} color="#4A4540" />}
                        inputSyle={{ padding: 0 }}
                      />
                      {fieldError && signupMethod === 'email' && (
                        <InlineToast message={fieldError} visible={!!fieldError} />
                      )}
                    </YStack>
                  )}
                </YStack>
              )}

              {(!signupMethod || signupMethod === 'phone') && (
                <YStack gap="$2">
                  <ThemedButton
                    variant='solid'
                    style={{
                      backgroundColor: signupMethod === 'phone' ? colorScheme.navIconBlur : colorScheme.primary,
                      borderWidth: signupMethod === 'phone' ? 2 : 0,
                      borderColor: signupMethod === 'phone' ? '$accent' : 'transparent',
                    }}
                    fullWidth
                    icon={<Phone size={20} color="#fff" />}
                    onPress={() => handleMethodSelect('phone')}
                  >
                    {(i18n.t('signup_with_phone') || 'Sign up with Phone')}
                  </ThemedButton>
                  {signupMethod === 'phone' && (
                    <YStack gap="$1" width="100%">
                      <XStack gap="$2" alignItems="center">
                        <CountryCodePicker
                          value={phoneCountryCode ?? ''}
                          onChange={onPhoneCountryCodeChange}
                        />
                        <ThemedInput
                          variant="light"
                          flex={1}
                          size="$5"
                          placeholder={i18n.t('phone_placeholder') || '234 567 8900'}
                          value={phoneBody}
                          onChangeText={(t) => onPhoneBodyChange(t)}
                          keyboardType="phone-pad"
                          color="$onGlass"
                          autoFocus
                          inputSyle={{ paddingLeft: 0, paddingRight: 0 }}
                          icon={<Phone size={20} color="#4A4540" />}
                        />
                      </XStack>
                      {fieldError && signupMethod === 'phone' && (
                        <InlineToast message={fieldError} visible={!!fieldError} />
                      )}
                    </YStack>
                  )}
                </YStack>
              )}

              {!signupMethod && (
                <>
                  <Separator borderColor='$onGlassTertiary' />
                  <ThemedButton variant="solid" fullWidth icon={<GoogleIcon size={20} />} onPress={handleGoogleLogin} loading={socialLoading === 'google'} disabled={loading} style={{ backgroundColor: colorScheme.primary }}>
                    {(i18n.t('signup_google') || 'Sign up with Google')}
                  </ThemedButton>
                  {Platform.OS === 'ios' && (
                    <ThemedButton variant="solid" fullWidth icon={<AppleIcon size={20} />} onPress={handleAppleLogin} loading={socialLoading === 'apple'} disabled={loading} style={{ backgroundColor: colorScheme.primary }}>
                      {(i18n.t('signup_apple') || 'Sign up with Apple')}
                    </ThemedButton>
                  )}
                </>
              )}
            </YStack>
          </AnimatedStack>
        </>
      )}

      {(showOtpInput || showOtpSuccess) && (
        <AnimatedStack key={showOtpSuccess ? 'signup-otp-success' : 'signup-otp-input'} direction="up">
          {showOtpSuccess ? (
            <CodeVerified
              title={i18n.t('code_verified') || 'Code verified'}
              subtitle={i18n.t('continue_to_terms') || 'One more step to finish'}
              accentColor={colorScheme.accent}
              durationMs={VERIFIED_DISPLAY_MS}
              onComplete={handleVerifiedComplete}
            />
          ) : (
            <YStack gap="$3" alignItems="center" width="100%">
              <YStack width={48} height={48} borderRadius={24} backgroundColor="rgba(16,185,129,0.15)" justifyContent="center" alignItems="center">
                {signupMethod === 'email' ? <Mail size={24} color="#262220" /> : <Phone size={24} color="#262220" />}
              </YStack>
              <Text color="$onGlass" fontSize={18} fontWeight="bold">
                {(i18n.t('verification') || 'Verification')}
              </Text>
              <Text color="$onGlassSecondary" fontSize={13} textAlign="center">
                {(i18n.t('enter_code_sent_to') || 'We sent a code to')}
              </Text>
              <Text color="$onGlass" fontWeight="bold" fontSize={14}>
                {signupMethod === 'email' ? email : phoneBody}
              </Text>

              <OtpInput
                value={verificationCode}
                onChange={(v) => onVerificationCodeChange(v.slice(0, 6))}
                error={!!error}
                autoFocus
                variant="light"
              />

              <Text
                color={countdown > 0 ? INK.primary : colorScheme.primary}
                textAlign="center"
                fontSize={13}
                onPress={handleResendCode}
                style={{
                  fontWeight: countdown > 0 ? 'normal' : 'bold',
                  textDecorationLine: countdown > 0 ? 'none' : 'underline',
                  cursor: countdown > 0 ? 'default' : 'pointer',
                }}
              >
                {countdown > 0
                  ? `${i18n.t('resend_code_in') || 'Resend code in'} ${countdown}s`
                  : (i18n.t('resend_code') || 'Resend Code')}
              </Text>
            </YStack>
          )}
        </AnimatedStack>
      )}
    </YStack>
  )
})
