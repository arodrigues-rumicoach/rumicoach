import { memo } from 'react'
import { YStack, Text } from 'tamagui'
import { router } from 'expo-router'
import { Heading, ThemedButton, GlassCard } from '@/components/atoms'
import i18n from '@/i18n'
import { WebMaxWidth } from '@/components/templates'

export const SuccessStep = memo(function SuccessStep() {
  return (
    <WebMaxWidth>
      <YStack flex={1} backgroundColor="transparent" justifyContent="center" alignItems="center" padding="$4" maxWidth={400} margin="auto">
        <GlassCard variant='light' padding={32} borderRadius={24}>
          <YStack gap={16} alignItems="center">
            <YStack width={56} height={56} borderRadius={28} backgroundColor="rgba(16,185,129,0.15)" justifyContent="center" alignItems="center">
              <Text color="$accent" fontSize={28} fontWeight="bold">✓</Text>
            </YStack>
            <Heading color="$onGlass" fontSize={24}>{(i18n.t('success') || 'Success')}!</Heading>
            <Text color="$onGlass" textAlign="center" fontSize={20} fontWeight="600">
              {i18n.t('signup_success_title') || "You're in. Ready to become your best self?"}
            </Text>
            <Text color="$onGlassSecondary" textAlign="center" fontSize={14}>
              {i18n.t('signup_success_subtitle') || 'Your first session with Rumi is one tap away.'}
            </Text>
            <YStack width="100%" gap="$3" marginTop="$2">
              <ThemedButton
                variant="solid"
                fullWidth
                onPress={() => router.replace({ pathname: '/(tabs)/session', params: { autoStart: 'true' } })}
              >
                {i18n.t('start_first_session') || 'Start my first session'}
              </ThemedButton>
              <ThemedButton
                variant="glass"
                fullWidth
                onPress={() => router.replace('/(tabs)/journey')}
              >
                {i18n.t('skip_to_journey') || 'Skip and go to Journey'}
              </ThemedButton>
            </YStack>
          </YStack>
        </GlassCard>
      </YStack>
    </WebMaxWidth>
  )
})
