import { memo } from 'react'
import { YStack, Text, Separator } from 'tamagui'
import { Sparkles } from 'lucide-react-native'
import { Link } from 'expo-router'
import { Heading, ThemedButton } from '@/components/atoms'
import i18n from '@/i18n'

interface InitialStepProps {
  onStart: () => void
}

export const InitialStep = memo(function InitialStep({ onStart }: InitialStepProps) {
  return (
    <YStack gap={24} alignItems="center" width="100%">
      <YStack alignItems="center" gap="$2">
        <Sparkles size={48} color="#262220" />
        <Heading color="$onGlass" fontSize={28} textAlign="center">
          {(i18n.t('create_account') || 'Create Account')}
        </Heading>
        <Text color="$onGlassSecondary" textAlign="center" fontSize={14}>
          {(i18n.t('waitlist_subtitle') || 'Be the first to know when we launch')}
        </Text>
      </YStack>

      <ThemedButton variant="solid" fullWidth onPress={onStart}>
        {(i18n.t('start_journey') || 'Start Your Journey')}
      </ThemedButton>

      <YStack width="100%" gap={16}>
        <Separator borderColor='$onGlassTertiary' />

        <Text textAlign="center" fontSize={13}>
          {(i18n.t('already_have_account') || 'Already have an account?')}
        </Text>

        <Link href="/(auth)/signin" asChild>
          <ThemedButton variant="solid" fullWidth>
            {(i18n.t('signin') || 'Sign in')}
          </ThemedButton>
        </Link>
      </YStack>
    </YStack>
  )
})
