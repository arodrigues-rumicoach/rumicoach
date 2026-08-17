import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Modal, StyleSheet, Platform, useWindowDimensions } from 'react-native'
import { Text, YStack, XStack } from 'tamagui'
import { BlurView } from 'expo-blur'
import { Mail, Phone, ArrowLeft } from 'lucide-react-native'
import Reanimated, {
  FadeIn,
  FadeOut,
  ZoomIn,
  ZoomOut,
} from 'react-native-reanimated'
import { ThemedInput, ThemedButton, CodeVerified, CountryCodePicker } from '@/components/atoms'
import { OtpInput } from '@/components/molecules/OtpInput'
import { useVerification } from '@/hooks/useVerification'
import { useSettings } from '@/hooks/useSettings'
import { useAuth } from '@/hooks/useAuth'
import i18n from '@/i18n'
import { COUNTRIES } from '@/utils/countries'
import type { User } from '@/api'

type Step = 'input' | 'otp' | 'success'

interface VerificationModalProps {
  visible: boolean
  type: 'email' | 'phone'
  initialValue?: string
  title?: string
  subtitle?: string
  onVerified: (updatedUser: User) => void
  onCancel: () => void
}

const VERIFIED_DISPLAY_MS = 1500

const getDefaultCountry = () => {
  try {
    const { getLocales } = require('expo-localization')
    const region = getLocales()?.[0]?.regionCode
    return COUNTRIES.find(c => c.code === region) ? region : 'PT'
  } catch {
    return 'PT'
  }
}

export const VerificationModal = memo(function VerificationModal({
  visible,
  type,
  initialValue = '',
  title,
  subtitle,
  onVerified,
  onCancel,
}: VerificationModalProps) {
  const { width } = useWindowDimensions()
  const { colorScheme } = useSettings()
  const { refreshUser } = useAuth()
  const isWeb = Platform.OS === 'web'
  const cardWidth = isWeb ? Math.min(width * 0.6, 480) : '100%'

  const [step, setStep] = useState<Step>('input')
  const [inputValue, setInputValue] = useState(initialValue)
  const [phoneCountryCode, setPhoneCountryCode] = useState(getDefaultCountry())
  const [phoneBody, setPhoneBody] = useState('')
  const [otp, setOtp] = useState('')

  const identifier = useMemo(() => {
    if (type === 'email') return inputValue
    const selected = COUNTRIES.find(c => c.code === phoneCountryCode)
    const digits = phoneBody.replace(/[^0-9]/g, '')
    return `${selected?.phoneCode || ''}${digits}`
  }, [type, inputValue, phoneCountryCode, phoneBody])

  const { state, countdown, error, sendCode, verifyAndUpdate, reset } = useVerification({
    type,
    identifier,
    refreshUser,
    onVerified: (updatedUser) => {
      setStep('success')
      onVerified(updatedUser)
    },
  })

  const prevVisibleRef = useRef(visible)
  useEffect(() => {
    if (visible && !prevVisibleRef.current) {
      setStep('input')
      setInputValue(initialValue)
      setPhoneCountryCode(getDefaultCountry())
      setPhoneBody('')
      setOtp('')
      reset()
    }
    prevVisibleRef.current = visible
  }, [visible, initialValue, reset])

  useEffect(() => {
    if (state === 'sent' && step === 'input') {
      setStep('otp')
    }
  }, [state, step])

  const handleOtpComplete = useCallback(
    (code: string) => {
      verifyAndUpdate(code)
    },
    [verifyAndUpdate]
  )

  const handleResend = useCallback(() => {
    if (countdown === 0) {
      setOtp('')
      sendCode()
    }
  }, [countdown, sendCode])

  const handleBackToInput = useCallback(() => {
    setStep('input')
    setOtp('')
    reset()
  }, [reset])

  const handleClose = useCallback(() => {
    reset()
    setStep('input')
    setOtp('')
    onCancel()
  }, [reset, onCancel])

  const handleVerifiedComplete = useCallback(() => {
    handleClose()
  }, [handleClose])

  const isEmail = type === 'email'
  const Icon = isEmail ? Mail : Phone
  const isVerifying = state === 'verifying'
  const canSend = isEmail ? inputValue.trim() : phoneBody.trim()
  const displayIdentifier = isEmail ? inputValue : identifier

  const handleSendCode = useCallback(async () => {
    if (!canSend) return
    await sendCode()
  }, [canSend, sendCode])

  const cardContent = (
    <>
      {step === 'input' && (
        <YStack gap={20} alignItems="center">
          <YStack width={48} height={48} borderRadius={24} backgroundColor="rgba(16,185,129,0.15)" justifyContent="center" alignItems="center">
            <Icon size={24} color="#262220" />
          </YStack>
          <YStack gap={8} alignItems="center">
            <Text style={styles.title}>
              {title ?? (isEmail
                ? (i18n.t('update_email') || 'Update Email')
                : (i18n.t('update_phone') || 'Update Phone Number'))}
            </Text>
            <Text style={styles.message}>
              {subtitle ?? (isEmail
                ? (i18n.t('enter_new_email') || 'Enter your new email address')
                : (i18n.t('enter_new_phone') || 'Enter your new phone number'))}
            </Text>
          </YStack>

          {isEmail ? (
            <ThemedInput
              variant="light"
              value={inputValue}
              onChangeText={setInputValue}
              placeholder="email@example.com"
              keyboardType="email-address"
              autoCapitalize="none"
              autoCorrect={false}
              color="#262220"
              fontSize={15}
              borderRadius={12}
              paddingHorizontal={0}
              width="100%"
            />
          ) : (
            <XStack gap="$2" alignItems="center" width="100%">
              <CountryCodePicker
                value={phoneCountryCode}
                onChange={setPhoneCountryCode}
              />
              <ThemedInput
                variant="light"
                flex={1}
                placeholder={i18n.t('phone_placeholder') || '234 567 8900'}
                value={phoneBody}
                onChangeText={setPhoneBody}
                keyboardType="phone-pad"
                color="#262220"
                fontSize={15}
                borderRadius={12}
                paddingHorizontal={0}
              />
            </XStack>
          )}

          {error ? <Text style={styles.errorText}>{error}</Text> : null}

          <YStack gap={8} width="100%">
            <ThemedButton
              variant="solid"
              fullWidth
              onPress={handleSendCode}
              disabled={!canSend || state === 'sending'}
              loading={state === 'sending'}
            >
              {i18n.t('send_code') || 'Send Code'}
            </ThemedButton>
            <ThemedButton variant="error" fullWidth onPress={handleClose}>
              {i18n.t('cancel') || 'Cancel'}
            </ThemedButton>
          </YStack>
        </YStack>
      )}

      {step === 'otp' && (
        <YStack gap={20} alignItems="center">
          <XStack alignItems="center" gap={8} width="100%">
            <ThemedButton variant="ghost" onPress={handleBackToInput} padding={0}>
              <ArrowLeft size={20} color="#262220" />
            </ThemedButton>
            <YStack width={48} height={48} borderRadius={24} backgroundColor="rgba(16,185,129,0.15)" justifyContent="center" alignItems="center">
              <Icon size={24} color="#262220" />
            </YStack>
          </XStack>

          <YStack gap={8} alignItems="center" width="100%">
            <Text style={styles.title}>
              {i18n.t('verification') || 'Verification'}
            </Text>
            <Text style={styles.message}>
              {i18n.t('enter_code_sent_to') || 'We sent a code to'}
            </Text>
            <Text style={styles.identifier}>{displayIdentifier}</Text>
          </YStack>

          <OtpInput
            value={otp}
            onChange={(v) => setOtp(v.slice(0, 6))}
            onComplete={handleOtpComplete}
            error={!!error}
            autoFocus
            variant="light"
            disabled={isVerifying}
          />

          {error ? <Text style={styles.errorText}>{error}</Text> : null}

          {isVerifying && (
            <Text style={styles.verifyingText}>
              {i18n.t('verifying') || 'Verifying...'}
            </Text>
          )}

          <Text
            onPress={handleResend}
            style={[
              styles.resendText,
              { color: countdown > 0 ? '#524B46' : colorScheme.primary },
            ]}
          >
            {countdown > 0
              ? `${i18n.t('resend_code_in') || 'Resend code in'} ${countdown}s`
              : (i18n.t('resend_code') || 'Resend Code')}
          </Text>

          <ThemedButton variant="error" fullWidth onPress={handleClose}>
            {i18n.t('cancel') || 'Cancel'}
          </ThemedButton>
        </YStack>
      )}

      {step === 'success' && (
        <CodeVerified
          title={i18n.t('verified') || 'Verified'}
          subtitle={isEmail
            ? (i18n.t('email_verified') || 'Your email has been updated')
            : (i18n.t('phone_verified') || 'Your phone number has been updated')}
          accentColor={colorScheme.accent}
          durationMs={VERIFIED_DISPLAY_MS}
          onComplete={handleVerifiedComplete}
        />
      )}
    </>
  )

  return (
    <Modal
      visible={visible}
      transparent
      animationType="none"
      onRequestClose={handleClose}
    >
      <Reanimated.View
        entering={FadeIn.duration(250)}
        exiting={FadeOut.duration(200)}
        style={styles.overlay}
      >
        <Reanimated.View
          entering={ZoomIn.duration(350).springify().damping(25).stiffness(200)}
          exiting={ZoomOut.duration(200)}
          style={[styles.card, { width: cardWidth }]}
        >
          {isWeb ? (
            <Reanimated.View
              entering={FadeIn.duration(300).delay(150)}
              exiting={FadeOut.duration(150)}
              style={styles.cardContent}
            >
              {cardContent}
            </Reanimated.View>
          ) : (
            <BlurView style={styles.blurCard} tint="light" intensity={40}>
              <Reanimated.View
                entering={FadeIn.duration(300).delay(150)}
                exiting={FadeOut.duration(150)}
                style={{ backgroundColor: 'transparent' }}
              >
                {cardContent}
              </Reanimated.View>
            </BlurView>
          )}
        </Reanimated.View>
      </Reanimated.View>
    </Modal>
  )
})

const styles = StyleSheet.create({
  overlay: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: 20,
    backgroundColor: 'rgba(0,0,0,0.4)',
  },
  card: {
    borderRadius: 20,
    overflow: 'hidden',
    boxShadow: '0px 8px 24px rgba(0,0,0,0.15)',
  },
  cardContent: {
    width: '100%',
    borderRadius: 20,
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.4)',
    padding: 24,
    gap: 4,
    backgroundColor: 'rgba(255, 255, 255, 0.95)',
    ...(Platform.OS === 'web' ? { backdropFilter: 'blur(16px)' } : {}),
  },
  blurCard: {
    width: '100%',
    borderRadius: 20,
    overflow: 'hidden',
    padding: 24,
    backgroundColor: 'rgba(255, 255, 255, 0.92)',
  },
  title: {
    fontSize: 18,
    fontWeight: '700',
    textAlign: 'center',
    color: '#262220',
    lineHeight: 24,
  },
  message: {
    fontSize: 14,
    textAlign: 'center',
    lineHeight: 20,
    color: '#524B46',
  },
  identifier: {
    fontSize: 14,
    fontWeight: '600',
    textAlign: 'center',
    color: '#262220',
  },
  errorText: {
    color: '#ef4444',
    fontSize: 13,
    textAlign: 'center',
  },
  verifyingText: {
    color: '#524B46',
    fontSize: 13,
    textAlign: 'center',
  },
  resendText: {
    fontSize: 13,
    textAlign: 'center',
  },
})
