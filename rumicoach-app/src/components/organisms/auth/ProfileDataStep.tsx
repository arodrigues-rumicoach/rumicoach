import { memo, useMemo } from 'react'
import { TouchableOpacity, StyleSheet } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { Shield } from 'lucide-react-native'
import { Heading, ThemedDateInput, CountryPicker, ThemedButton } from '@/components/atoms'
import i18n from '@/i18n'

const MINIMUM_AGE = 16

interface ProfileDataStepProps {
  dateOfBirth: string
  gender: string
  country: string
  onDateOfBirthChange: (value: string) => void
  onGenderChange: (value: string) => void
  onCountryChange: (value: string) => void
}

export const ProfileDataStep = memo(function ProfileDataStep({
  dateOfBirth,
  gender,
  country,
  onDateOfBirthChange,
  onGenderChange,
  onCountryChange,
}: ProfileDataStepProps) {
  const maxDate = useMemo(() => {
    const d = new Date()
    d.setFullYear(d.getFullYear() - MINIMUM_AGE)
    return d
  }, [])

  return (
    <YStack gap="$4" width="100%">
      <YStack alignItems="center" gap="$2">
        <Shield size={28} color="#262220" />
        <Heading color="$onGlass" fontSize={20} textAlign="center">
          {(i18n.t('manual_entry_title') || 'Your Details')}
        </Heading>
      </YStack>

      {/* Date of Birth */}
      <YStack gap="$2">
        <Text color="$onGlassSecondary" fontSize={12} fontWeight="600">
          {(i18n.t('date_of_birth') || 'Date of Birth')}
        </Text>
        <ThemedDateInput
          value={dateOfBirth}
          onChange={onDateOfBirthChange}
          placeholder={i18n.t('select_date_of_birth') || 'Select Date of Birth'}
          maximumDate={maxDate}
        />
      </YStack>

      {/* Gender */}
      <YStack gap="$2">
        <Text color="$onGlassSecondary" fontSize={12} fontWeight="600">
          {(i18n.t('gender') || 'Gender')}
        </Text>
        <XStack gap="$3" flexWrap='wrap'>
          {['male', 'female', 'other'].map(g => (
            <ThemedButton
              key={g}
              variant={gender === g ? 'solid' : 'outline'}
              onPress={() => onGenderChange(g)}
              buttonStyle={{ paddingVertical: 8, paddingHorizontal: 16 }}
            >
              {i18n.t(g) || g}
            </ThemedButton>
          ))}
        </XStack>
      </YStack>

      {/* Country */}
      <YStack gap="$2">
        <Text color="$onGlassSecondary" fontSize={12} fontWeight="600">
          {(i18n.t('country') || 'Country')}
        </Text>
        <CountryPicker value={country} onChange={onCountryChange} />
      </YStack>
    </YStack>
  )
})

const styles = StyleSheet.create({
  genderOption: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
    padding: 14,
    borderRadius: 12,
    borderWidth: 2,
    borderColor: 'rgba(0,0,0,0.10)',
    backgroundColor: 'rgba(0,0,0,0.03)',
    minHeight: 48,
  },
  genderOptionSelected: {
    borderColor: 'rgba(16,185,129,0.40)',
    backgroundColor: 'rgba(16,185,129,0.08)',
  },
})
