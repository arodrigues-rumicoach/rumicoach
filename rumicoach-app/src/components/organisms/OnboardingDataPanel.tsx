import { memo, useMemo } from 'react'
import { YStack, Text } from 'tamagui'
import i18n from '@/i18n'
import type { User } from '@/api'
import { Heading } from '@/components/atoms'
import { GlassPanel } from '@/components/molecules/GlassPanel'
import Reanimated, { FadeInDown } from 'react-native-reanimated'
import { getCountryDisplayName } from '@/utils/countries'

interface OnboardingDataPanelProps {
  data: Partial<User>
}

function FieldRow({ label, value }: { label: string; value: string }) {
  return (
    <YStack
      width="100%"
      borderRadius={16}
      paddingHorizontal={16}
      paddingVertical={14}
      gap={6}
      backgroundColor="rgba(0,0,0,0.05)"
    >
      <Text fontSize={12} letterSpacing={1.2} textTransform="uppercase" color="$onGlassSecondary">
        {label}
      </Text>
      <Text fontSize={15} fontWeight="600" color="$onGlass">
        {value}
      </Text>
    </YStack>
  )
}

export const OnboardingDataPanel = memo(function OnboardingDataPanel({ data }: OnboardingDataPanelProps) {
  const fields = useMemo(() => {
    const result: { label: string; value: string }[] = []
    if (data.dateOfBirth) {
      result.push({ label: i18n.t('date_of_birth') || 'Date of Birth', value: data.dateOfBirth })
    }
    if (data.gender) {
      result.push({ label: i18n.t('gender') || 'Gender', value: i18n.t(data.gender) || data.gender })
    }
    if (data.country) {
      result.push({ label: i18n.t('country') || 'Country', value: getCountryDisplayName(data.country) })
    }
    return result
  }, [data.dateOfBirth, data.gender, data.country])

  if (fields.length === 0) return null

  return (
    <Reanimated.View entering={FadeInDown.duration(500).springify().damping(20).stiffness(150)} style={{ width: '100%' }}>
      <GlassPanel variant="light" margin={12}>
        <Heading color="$onGlass">{i18n.t('your_profile') || 'Your Profile'}</Heading>
        <YStack width="100%" gap={10}>
          {fields.map((field) => (
            <FieldRow key={field.label} label={field.label} value={field.value} />
          ))}
        </YStack>
      </GlassPanel>
    </Reanimated.View>
  )
})
