import { memo } from 'react'
import { YStack, XStack, Text } from 'tamagui'
import { Globe } from 'lucide-react-native'
import { Heading, ThemedButton } from '@/components/atoms'
import { TermsStep } from './TermsStep'
import i18n from '@/i18n'

interface RegionTermsStepProps {
  dataRegion: 'eu' | 'us'
  acceptTerms: boolean
  acceptAi: boolean
  acceptMarketing: boolean
  termsError: string
  onRegionChange: (region: 'eu' | 'us') => void
  onAcceptTermsChange: (value: boolean) => void
  onAcceptAiChange: (value: boolean) => void
  onAcceptMarketingChange: (value: boolean) => void
}

export const RegionTermsStep = memo(function RegionTermsStep({
  dataRegion,
  acceptTerms,
  acceptAi,
  acceptMarketing,
  termsError,
  onRegionChange,
  onAcceptTermsChange,
  onAcceptAiChange,
  onAcceptMarketingChange,
}: RegionTermsStepProps) {
  return (
    <YStack gap="$4" width="100%">
      <YStack alignItems="center" gap="$2">
        <Globe size={32} color="#262220" />
        <Heading color="$onGlass" fontSize={20} textAlign="center">
          {(i18n.t('data_region') || 'Where do you want your data to be saved?')}
        </Heading>
        <Text color="$onGlassSecondary" fontSize={13} textAlign="center">
          {(i18n.t('data_region_hint') || 'This is where your data will be stored, and it cannot be changed later.')}
        </Text>
      </YStack>

      <XStack gap="$3" justifyContent='center'>
        {([
          { key: 'eu' as const, label: i18n.t('data_europe') || 'Europe' },
          { key: 'us' as const, label: i18n.t('data_united_states') || 'United States' },
        ]).map(opt => (
          <ThemedButton
            key={opt.key}
            flex={1}
            variant={dataRegion === opt.key ? 'solid' : 'outline'}
            onPress={() => onRegionChange(opt.key)}
          >
            {opt.label}
          </ThemedButton>
        ))}
      </XStack>

      <YStack gap="$2" marginTop="$2">
        <Heading color="$onGlass" fontSize={18} textAlign="center" marginBottom="$1">
          {(i18n.t('terms_and_conditions') || 'Terms and Conditions')}
        </Heading>
        <TermsStep
          acceptTerms={acceptTerms}
          acceptAi={acceptAi}
          acceptMarketing={acceptMarketing}
          termsError={termsError}
          onAcceptTermsChange={onAcceptTermsChange}
          onAcceptAiChange={onAcceptAiChange}
          onAcceptMarketingChange={onAcceptMarketingChange}
        />
      </YStack>
    </YStack>
  )
})
