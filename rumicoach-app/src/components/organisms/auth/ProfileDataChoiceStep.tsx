import { memo, useCallback } from 'react'
import { View, TouchableOpacity, StyleSheet } from 'react-native'
import { YStack, Text } from 'tamagui'
import { MessageCircle, Keyboard } from 'lucide-react-native'
import { Heading } from '@/components/atoms'
import i18n from '@/i18n'
import { useSettings } from '@/hooks/useSettings'

interface ProfileDataChoiceStepProps {
  choice: 'manual' | 'ai' | null
  onChoiceChange: (choice: 'manual' | 'ai') => void
}

export const ProfileDataChoiceStep = memo(function ProfileDataChoiceStep({
  choice,
  onChoiceChange,
}: ProfileDataChoiceStepProps) {
  const { colorScheme } = useSettings()

  return (
    <YStack gap="$4" width="100%">
      <YStack alignItems="center" gap="$2">
        <Heading color="$onGlass" fontSize={20} textAlign="center">
          {(i18n.t('complete_your_profile') || 'Complete your profile')}
        </Heading>
        <Text color="$onGlassSecondary" fontSize={13} textAlign="center">
          {(i18n.t('profile_data_hint') || 'How would you like to fill in your country, birthday, and gender?')}
        </Text>
      </YStack>

      <YStack gap="$3">
        <TouchableOpacity
          style={[styles.choiceOption, choice === 'manual' && styles.choiceOptionSelected]}
          onPress={() => onChoiceChange('manual')}
          activeOpacity={0.7}
        >
          <View style={[styles.choiceIcon, { backgroundColor: 'rgba(0,0,0,0.06)' }]}>
            <Keyboard size={24} color="#262220" />
          </View>
          <YStack flex={1} gap={2}>
            <Text fontSize={15} fontWeight="600" color="$onGlass">
              {i18n.t('fill_manually') || 'Fill in manually'}
            </Text>
            <Text fontSize={13} color="$onGlassSecondary">
              {i18n.t('fill_manually_hint') || 'Enter your details using form fields'}
            </Text>
          </YStack>
        </TouchableOpacity>

        <TouchableOpacity
          style={[styles.choiceOption, choice === 'ai' && styles.choiceOptionSelected]}
          onPress={() => onChoiceChange('ai')}
          activeOpacity={0.7}
        >
          <View style={[styles.choiceIcon, { backgroundColor: `${colorScheme.secondary}20` }]}>
            <MessageCircle size={24} color="#262220" />
          </View>
          <YStack flex={1} gap={2}>
            <Text fontSize={15} fontWeight="600" color="$onGlass">
              {i18n.t('chat_with_rumi') || 'Chat with Rumi'}
            </Text>
            <Text fontSize={13} color="$onGlassSecondary">
              {i18n.t('chat_with_rumi_hint') || 'Let Rumi ask you during your first session'}
            </Text>
          </YStack>
        </TouchableOpacity>
      </YStack>
    </YStack>
  )
})

const styles = StyleSheet.create({
  choiceOption: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 14,
    padding: 16,
    borderRadius: 14,
    borderWidth: 2,
    borderColor: 'rgba(0,0,0,0.10)',
    backgroundColor: 'rgba(0,0,0,0.03)',
  },
  choiceOptionSelected: {
    borderColor: 'rgba(16,185,129,0.40)',
    backgroundColor: 'rgba(16,185,129,0.08)',
  },
  choiceIcon: {
    width: 44,
    height: 44,
    borderRadius: 22,
    justifyContent: 'center',
    alignItems: 'center',
  },
})
