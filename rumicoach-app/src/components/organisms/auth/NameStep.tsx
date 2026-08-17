import { memo } from 'react'
import { YStack } from 'tamagui'
import { User } from 'lucide-react-native'
import { Heading, ThemedInput } from '@/components/atoms'
import i18n from '@/i18n'

interface NameStepProps {
  name: string
  onNameChange: (name: string) => void
  onSubmit: () => void
}

export const NameStep = memo(function NameStep({ name, onNameChange, onSubmit }: NameStepProps) {
  return (
    <YStack gap="$4" alignItems="center" width="100%">
      <User size={36} color="#262220" />
      <Heading color="$onGlass" fontSize={20} textAlign="center">
        {(i18n.t('what_to_call') || 'What do you want to be called?')}
      </Heading>

      <ThemedInput
        variant="light"
        size="$5"
        width="100%"
        placeholder={(i18n.t('name') || 'Name')}
        value={name}
        onChangeText={onNameChange}
        autoCapitalize="words"
        color="$onGlass"
        autoFocus
        onSubmitEditing={onSubmit}
      />
    </YStack>
  )
})
