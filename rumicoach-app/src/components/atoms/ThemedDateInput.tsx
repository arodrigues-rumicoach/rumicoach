import { useState, useCallback, useMemo } from 'react'
import { View, Platform, Modal, Pressable, type ViewStyle } from 'react-native'
import { XStack, Text } from 'tamagui'
import { Calendar } from 'lucide-react-native'
import { useI18n } from '@/i18n'
import { Calendar as CalendarPicker } from '@/components/molecules/Calendar'
import { INK } from '@/styles/glass'

interface ThemedDateInputProps {
  value: string
  onChange: (isoDate: string) => void
  placeholder?: string
  maximumDate?: Date
  backgroundColor?: string
  borderRadius?: number
  style?: ViewStyle
}

function formatDateForLocale(date: Date, locale: string): string {
  try {
    return date.toLocaleDateString(locale, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    })
  } catch {
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    })
  }
}

function isoToDate(iso: string): Date | null {
  if (!iso) return null
  const parts = iso.split('-')
  if (parts.length !== 3) return null
  return new Date(+parts[0], +parts[1] - 1, +parts[2])
}

export function ThemedDateInput({
  value,
  onChange,
  placeholder = 'Select Date',
  maximumDate,
  backgroundColor = 'rgba(255,255,255,0.05)',
  borderRadius = 12,
  style,
}: ThemedDateInputProps) {
  const { locale } = useI18n()
  const [showPicker, setShowPicker] = useState(false)

  const displayValue = useMemo(() => {
    const date = isoToDate(value)
    if (!date) return ''
    return formatDateForLocale(date, locale)
  }, [value, locale])

  const handlePress = useCallback(() => {
    setShowPicker((prev) => !prev)
  }, [])

  const handleClose = useCallback(() => {
    setShowPicker(false)
  }, [])

  const containerStyle: ViewStyle = {
    height: 44,
    backgroundColor,
    borderWidth: 1,
    borderColor: INK.primary,
    borderRadius,
    padding: 12,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 12
  }

  return (
    <View>
      <Pressable
        onPress={handlePress}
        tabIndex={-1}
        style={({ pressed }) => [
          Platform.select({
            web: { outline: 'none', cursor: 'pointer', opacity: pressed ? 0.7 : 1 } as any,
            default: { opacity: pressed ? 0.7 : 1 },
          }),
        ]}
      >
        <XStack style={containerStyle}>
          <Calendar size={18} color={INK.primary} />
          <Text
            flex={1}
            color='$onGlass'
            fontSize={14}
          >
            {displayValue || placeholder}
          </Text>
        </XStack>
      </Pressable>

      <Modal
        visible={showPicker}
        transparent
        animationType="fade"
        onRequestClose={handleClose}
      >
        <Pressable
          style={{
            flex: 1,
            backgroundColor: 'rgba(0,0,0,0.5)',
            justifyContent: 'center',
            alignItems: 'center',
          }}
          onPress={handleClose}
        >
          <CalendarPicker
            value={value}
            onChange={onChange}
            maximumDate={maximumDate}
            locale={locale}
            onClose={handleClose}
          />
        </Pressable>
      </Modal>
    </View>
  )
}
