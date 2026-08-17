import { memo, useCallback } from 'react'
import { TouchableOpacity, StyleSheet, ActivityIndicator } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { Mic, Mars, Venus, Play, Square, Edit3 } from 'lucide-react-native'
import { Heading, ThemedButton } from '@/components/atoms'
import i18n from '@/i18n'
import { useVoicePreview } from '@/hooks/useVoicePreview'
import { INK } from '@/styles/glass'
import * as Haptics from 'expo-haptics'

interface CoachPreferenceStepProps {
  coachGender: 'male' | 'female' | null
  coachVoice: string | null
  onGenderChange: (gender: 'male' | 'female') => void
  onVoiceChange: (voiceId: string) => void
  onContinueWithVoice: () => void
  onFillInManually: () => void
  loading: boolean
}

const VOICES = {
  female: [
    { id: 'gacrux', label: 'Gacrux' },
    { id: 'aoede', label: 'Aoede' },
    { id: 'vindemiatrix', label: 'Vindemiatrix' },
  ],
  male: [
    { id: 'algieba', label: 'Algieba' },
    { id: 'enceladus', label: 'Enceladus' },
    { id: 'charon', label: 'Charon' },
  ],
}

const FIRST_VOICE: Record<string, string> = {
  female: 'gacrux',
  male: 'algieba',
}

export const CoachPreferenceStep = memo(function CoachPreferenceStep({
  coachGender,
  coachVoice,
  onGenderChange,
  onVoiceChange,
  onContinueWithVoice,
  onFillInManually,
  loading,
}: CoachPreferenceStepProps) {
  const { playingVoice, loadingVoice, play: playVoice } = useVoicePreview()
  const voices = coachGender ? VOICES[coachGender] : []

  const handleGenderSelect = useCallback((gender: 'male' | 'female') => {
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light)
    onGenderChange(gender)
    onVoiceChange(FIRST_VOICE[gender])
  }, [onGenderChange, onVoiceChange])

  const handlePlayVoice = useCallback((voiceId: string) => {
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light)
    playVoice(voiceId)
  }, [playVoice])

  const handleVoiceSelect = useCallback((voiceId: string) => {
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light)
    onVoiceChange(voiceId)
  }, [onVoiceChange])

  const canContinue = !!coachGender && !!coachVoice

  return (
    <YStack gap="$5" width="100%">
      <YStack alignItems="center" gap="$2">
        <Mic size={28} color={INK.accent} />
        <Heading color={INK.primary} fontSize={20} textAlign="center">
          {(i18n.t('choose_your_coach') || 'Choose your coach')}
        </Heading>
        <Text color={INK.tertiary} fontSize={13} textAlign="center" lineHeight={18}>
          {(i18n.t('coach_preference_hint') || 'Select a gender and voice for your AI coach')}
        </Text>
      </YStack>

      {/* Gender Selection */}
      <YStack gap="$2">
        <Text fontSize={11} fontWeight="700" letterSpacing={0.8} color={INK.tertiary} textTransform="uppercase">
          {i18n.t('gender') || 'Gender'}
        </Text>
        <XStack gap="$3">
          {(['male', 'female'] as const).map((g) => (
            <ThemedButton
              key={g}
              variant={coachGender === g ? 'solid' : 'outline'}
              flex={1}
              icon={g === 'male' ? <Mars size={18} /> : <Venus size={18} />}
              onPress={() => handleGenderSelect(g)}
              buttonStyle={{ paddingVertical: 8, paddingHorizontal: 16 }}
            >
              {i18n.t(g) || g}
            </ThemedButton>
          ))}
        </XStack>
      </YStack>

      {/* Voice Selection */}
      <YStack gap="$2">
        <Text fontSize={11} fontWeight="700" letterSpacing={0.8} color={INK.tertiary} textTransform="uppercase">
          {i18n.t('voice') || 'Voice'}
        </Text>
        <YStack gap="$2">
          {voices.length === 0 ? (
            <Text color={INK.tertiary} fontSize={13} textAlign="center" paddingVertical="$3">
              {i18n.t('select_gender_first') || 'Select a gender to see available voices'}
            </Text>
          ) : (
            voices.map((v) => (
              <XStack
                key={v.id}
                alignItems="center"
                gap="$2"
                paddingVertical="$1"
                opacity={!!coachGender ? 1 : 0.4}
              >
                <ThemedButton
                  variant={coachVoice === v.id ? 'solid' : 'outline'}
                  flex={1}
                  icon={<Mic size={16} />}
                  onPress={() => handleVoiceSelect(v.id)}
                  disabled={!coachGender}
                >
                  {v.label}
                </ThemedButton>
                <TouchableOpacity
                  onPress={() => handlePlayVoice(v.id)}
                  disabled={!coachGender || (!!loadingVoice && loadingVoice !== v.id)}
                  style={[
                    styles.playButton,
                    loadingVoice === v.id && styles.playButtonActive,
                    loadingVoice && loadingVoice !== v.id && styles.playButtonDisabled,
                  ]}
                >
                  {loadingVoice === v.id ? (
                    <ActivityIndicator size="small" color={INK.accent} />
                  ) : playingVoice === v.id ? (
                    <Square size={12} color={INK.primary} />
                  ) : (
                    <Play size={12} color={INK.primary} style={{ marginLeft: 2 }} />
                  )}
                </TouchableOpacity>
              </XStack>
            ))
          )}
        </YStack>
      </YStack>

      {/* Continue with voice */}
      <YStack gap={6}>
        <ThemedButton
          variant="solid"
          fullWidth
          icon={<Mic size={18} color="#fff" />}
          onPress={onContinueWithVoice}
          disabled={!canContinue || loading}
          loading={loading}
        >
          {i18n.t('continue_with_voice') || 'Talk to Rumi'}
        </ThemedButton>

        {/* Fill in manually */}
        <ThemedButton
          variant="glass"
          fullWidth
          icon={<Edit3 size={18} color={INK.primary} />}
          onPress={onFillInManually}
          disabled={loading}
        >
          {i18n.t('fill_in_manually') || 'Fill in Manually'}
        </ThemedButton>
      </YStack>
    </YStack>
  )
})

const styles = StyleSheet.create({
  playButton: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: 'rgba(0,0,0,0.06)',
    justifyContent: 'center',
    alignItems: 'center',
  },
  playButtonActive: {
    backgroundColor: 'rgba(16,185,129,0.12)',
  },
  playButtonDisabled: {
    opacity: 0.4,
  },
})
