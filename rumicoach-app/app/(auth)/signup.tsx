import { useEffect, useRef } from 'react'
import { KeyboardAvoidingView, ScrollView, StyleSheet } from 'react-native'
import { YStack, XStack } from 'tamagui'
import { Link, useRouter } from 'expo-router'
import { ArrowLeft, ArrowRight } from 'lucide-react-native'
import { useBlurTarget } from '@/context/BlurContext'
import { useSettings } from '@/hooks/useSettings'
import i18n from '@/i18n'
import { ThemedButton, FadeSlideIn, GlassCard } from '@/components/atoms'
import { SignupProgressBar } from '@/components/molecules'
import { InlineToast } from '@/components/molecules'
import {
  NameStep,
  SignupMethodStep,
  SuccessStep,
  RegionTermsStep,
  CoachPreferenceStep,
  ProfileDataStep,
} from '@/components/organisms/auth'
import { useSignupForm } from '@/hooks/useSignupForm'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withSequence,
  withTiming,
  FadeIn,
  FadeOut,
  Easing,
} from 'react-native-reanimated'

export default function SignupScreen() {
  const blurTargetRef = useBlurTarget()
  const { colorScheme } = useSettings()
  const router = useRouter()
  const form = useSignupForm()
  const cardScale = useSharedValue(1)
  const prevStepRef = useRef(form.step)

  useEffect(() => {
    if (form.step !== prevStepRef.current) {
      prevStepRef.current = form.step
      cardScale.value = withSequence(
        withTiming(0.985, { duration: 100, easing: Easing.out(Easing.cubic) }),
        withSpring(1, { damping: 16, stiffness: 240 })
      )
    }
  }, [form.step, cardScale])

  const cardAnimatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: cardScale.value }],
  }))

  if (form.step === 'SUCCESS') {
    return <SuccessStep />
  }

  const isCoachStep = form.step === 'COACH_PREFERENCE'

  return (
    <KeyboardAvoidingView style={styles.keyboardView} behavior="padding">
      <ScrollView
        style={styles.scrollView}
        contentContainerStyle={styles.contentContainer}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
      >
        <YStack padding="$4" width="100%" maxWidth={400}>
          <Reanimated.View style={cardAnimatedStyle}>
            <GlassCard variant='light' blurTarget={blurTargetRef} padding={24} gap={20} borderRadius={24}>
              <SignupProgressBar currentStep={form.step} />

              {form.error && (
                <InlineToast message={form.error} visible={!!form.error} />
              )}

              {form.accountExists && (
                <Reanimated.View
                  entering={FadeIn.duration(220).springify().damping(20).stiffness(200)}
                  exiting={FadeOut.duration(160)}
                >
                  <Link href="/(auth)/signin" asChild>
                    <ThemedButton variant="solid" fullWidth>
                      {(i18n.t('signin') || 'Sign in')}
                    </ThemedButton>
                  </Link>
                </Reanimated.View>
              )}

              <FadeSlideIn key={form.step} direction={form.stepDirection}>
                {form.step === 'NAME' && (
                  <NameStep name={form.name} onNameChange={form.setName} onSubmit={form.handleNameSubmit} />
                )}

                {form.step === 'METHOD' && (
                  <YStack gap="$3" width="100%">
                    <SignupMethodStep
                      name={form.name}
                      signupMethod={form.signupMethod}
                      email={form.email}
                      phoneBody={form.phoneBody}
                      phoneCountryCode={form.phoneCountryCode}
                      verificationCode={form.verificationCode}
                      verificationSent={form.verificationSent}
                      verified={form.verified}
                      countdown={form.countdown}
                      loading={form.loading}
                      socialLoading={form.socialLoading}
                      error={form.error}
                      fieldError={form.fieldError}
                      colorScheme={colorScheme}
                      onMethodSelect={form.handleMethodSelect}
                      onEmailChange={form.handleEmailChange}
                      onPhoneBodyChange={form.handlePhoneBodyChange}
                      onPhoneCountryCodeChange={form.setPhoneCountryCode}
                      onVerificationCodeChange={form.handleVerificationCodeChange}
                      onGoogleLogin={form.handleGoogleLogin}
                      onAppleLogin={form.handleAppleLogin}
                      onResendCode={form.handleSendCode}
                    />
                  </YStack>
                )}

                {form.step === 'VERIFY' && (
                  <YStack gap="$3" width="100%">
                    <SignupMethodStep
                      name={form.name}
                      signupMethod={form.signupMethod}
                      email={form.email}
                      phoneBody={form.phoneBody}
                      phoneCountryCode={form.phoneCountryCode}
                      verificationCode={form.verificationCode}
                      verificationSent={form.verificationSent}
                      verified={form.verified}
                      countdown={form.countdown}
                      loading={form.loading}
                      socialLoading={form.socialLoading}
                      error={form.error}
                      fieldError={form.fieldError}
                      colorScheme={colorScheme}
                      onMethodSelect={form.handleMethodSelect}
                      onEmailChange={form.handleEmailChange}
                      onPhoneBodyChange={form.handlePhoneBodyChange}
                      onPhoneCountryCodeChange={form.setPhoneCountryCode}
                      onVerificationCodeChange={form.handleVerificationCodeChange}
                      onGoogleLogin={form.handleGoogleLogin}
                      onAppleLogin={form.handleAppleLogin}
                      onResendCode={form.handleSendCode}
                      onVerified={() => form.handleNext()}
                    />
                  </YStack>
                )}

                {form.step === 'REGION_TERMS' && (
                  <RegionTermsStep
                    dataRegion={form.dataRegion}
                    acceptTerms={form.acceptTerms}
                    acceptAi={form.acceptAi}
                    acceptMarketing={form.acceptMarketing}
                    termsError={form.termsError}
                    onRegionChange={form.setDataRegion}
                    onAcceptTermsChange={(v) => { form.setAcceptTerms(v); form.setTermsError('') }}
                    onAcceptAiChange={(v) => { form.setAcceptAi(v); form.setTermsError('') }}
                    onAcceptMarketingChange={form.setAcceptMarketing}
                  />
                )}

                {form.step === 'COACH_PREFERENCE' && (
                  <CoachPreferenceStep
                    coachGender={form.coachGender}
                    coachVoice={form.coachVoice}
                    onGenderChange={form.setCoachGender}
                    onVoiceChange={form.setCoachVoice}
                    onContinueWithVoice={form.handleContinueWithVoice}
                    onFillInManually={form.handleFillInManually}
                    loading={form.loading}
                  />
                )}

                {form.step === 'PROFILE_DATA' && (
                  <ProfileDataStep
                    dateOfBirth={form.dateOfBirth}
                    gender={form.gender}
                    country={form.country}
                    onDateOfBirthChange={form.setDateOfBirth}
                    onGenderChange={form.setGender}
                    onCountryChange={form.setCountry}
                  />
                )}
              </FadeSlideIn>

              {/* Bottom buttons - hidden for COACH_PREFERENCE (uses action cards) and SUCCESS */}
              {(form.step as string) !== 'SUCCESS' && !isCoachStep && (
                <XStack width="100%" justifyContent="space-between" marginTop="$4" gap="$3">
                  <ThemedButton
                    variant="solid"
                    onPress={() => {
                      if (form.step === 'NAME') {
                        router.replace('/(auth)/signin')
                      } else {
                        form.handleBack()
                      }
                    }}
                    disabled={form.loading}
                    icon={<ArrowLeft size={16} />}
                  >
                    {i18n.t('back') || 'Back'}
                  </ThemedButton>
                  <ThemedButton
                    variant="solid"
                    glow={form.step === 'REGION_TERMS' && !form.loading && form.acceptTerms && form.acceptAi}
                    onPress={form.handleNext}
                    disabled={form.loading
                      || (form.step === 'METHOD' && !form.signupMethod)
                      || (form.step === 'VERIFY' && !form.verified && form.verificationSent && form.verificationCode.length < 6)
                      || (form.step === 'REGION_TERMS' && (!form.acceptTerms || !form.acceptAi))
                      || (form.step === 'PROFILE_DATA' && (!form.dateOfBirth || !form.gender))
                    }
                    loading={form.loading}
                    iconAfter={<ArrowRight size={16} />}
                  >
                    {form.getNextLabel()}
                  </ThemedButton>
                </XStack>
              )}

              {/* Back button only for COACH_PREFERENCE */}
              {isCoachStep && (
                <XStack width="100%" justifyContent="flex-start" marginTop="$4">
                  <ThemedButton
                    variant="solid"
                    onPress={form.handleBack}
                    disabled={form.loading}
                    icon={<ArrowLeft size={16} />}
                  >
                    {i18n.t('back') || 'Back'}
                  </ThemedButton>
                </XStack>
              )}
            </GlassCard>
          </Reanimated.View>
        </YStack>
      </ScrollView>
    </KeyboardAvoidingView>
  )
}

const styles = StyleSheet.create({
  keyboardView: {
    flex: 1,
    backgroundColor: 'transparent',
  },
  scrollView: {
    backgroundColor: 'transparent',
  },
  contentContainer: {
    flexGrow: 1,
    justifyContent: 'center',
    alignItems: 'center',
  },
})
