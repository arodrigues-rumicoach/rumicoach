import { memo } from 'react'
import { YStack, XStack, Text } from 'tamagui'
import { Link } from 'expo-router'
import { Checkbox } from '@/components/atoms'
import i18n from '@/i18n'

interface TermsStepProps {
  acceptTerms: boolean
  acceptAi: boolean
  acceptMarketing: boolean
  termsError: string
  onAcceptTermsChange: (value: boolean) => void
  onAcceptAiChange: (value: boolean) => void
  onAcceptMarketingChange: (value: boolean) => void
}

export const TermsStep = memo(function TermsStep({
  acceptTerms, acceptAi, acceptMarketing, termsError,
  onAcceptTermsChange, onAcceptAiChange, onAcceptMarketingChange
}: TermsStepProps) {
  return (
    <YStack gap="$3" marginTop="$2" width="100%">
      <XStack gap="$2" alignItems="flex-start">
        <Checkbox
          checked={acceptTerms}
          onCheckedChange={v => onAcceptTermsChange(v)}
        />
        <Text color="$onGlassSecondary" fontSize={12} flex={1} lineHeight={18}>
          <Text color="$error">* </Text>
          {(i18n.t('i_accept_the') || 'I accept the')}{' '}
          <Link href="/legal/terms"><Text color="$accent" fontSize={12}>{(i18n.t('terms_of_service') || 'Terms of Service')}</Text></Link>
          {' '}{(i18n.t('terms_and') || 'and acknowledge the')}{' '}
          <Link href="/legal/privacy"><Text color="$accent" fontSize={12}>{(i18n.t('privacy_policy') || 'Privacy Policy')}</Text></Link>.
        </Text>
      </XStack>

      {/* Separate from the Terms checkbox on purpose. App Review rejected 1.0 under
          5.1.1(i)/5.1.2(i) and said outright that carrying this in the Terms or the Privacy
          Policy is not sufficient — the user has to be told what leaves the device and who
          receives it, and agree to that specifically. The old wording lived inside the
          Terms line and said only "I understand I am interacting with an AI coaching
          system", which named neither the data nor the recipient.

          Named at the Google Cloud level rather than by model: the speech model can change,
          and a disclosure that goes stale needs a new build to fix. Google is the recipient
          either way, which is what the guideline asks for. */}
      <XStack gap="$2" alignItems="flex-start">
        <Checkbox
          checked={acceptAi}
          onCheckedChange={v => onAcceptAiChange(v)}
        />
        <Text color="$onGlassSecondary" fontSize={12} flex={1} lineHeight={18}>
          <Text color="$error">* </Text>
          {(i18n.t('ai_consent_checkbox')
            || "I understand that I am interacting with an AI coaching system, that its outputs are computer-generated and I agree that the audio of my voice is sent to our server and processed by our services, inside Google Cloud, so Rumi can reply out loud, and that what I say is saved to my profile as memories.")}
        </Text>
      </XStack>

      <XStack gap="$2" alignItems="flex-start">
        <Checkbox
          checked={acceptMarketing}
          onCheckedChange={onAcceptMarketingChange}
        />
        <Text color="$onGlassSecondary" fontSize={12} flex={1} lineHeight={18}>
          {(i18n.t('marketing_consent') || 'Keep me updated with coaching tips and new feature releases')}
        </Text>
      </XStack>

      {termsError ? <Text color="$error" fontSize={12}>{termsError}</Text> : null}
    </YStack>
  )
})
