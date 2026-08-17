import { useState, useCallback, useMemo } from 'react'
import { Pressable, ScrollView } from 'react-native'
import Animated, { FadeIn, FadeOut } from 'react-native-reanimated'
import { YStack, XStack, Text } from 'tamagui'
import { ChevronLeft, ChevronRight, ChevronDown } from 'lucide-react-native'

interface CalendarProps {
  value: string
  onChange: (isoDate: string) => void
  maximumDate?: Date
  locale?: string
  onClose?: () => void
}

const WEEKDAYS = ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su']

const CARD_BG = '#f2f2f7'
const BUTTON_BG = '#ffffff'
const BUTTON_TEXT = '#1c1c1e'
const ACCENT = '#2563eb'
const CELL_BG = '#ffffff'
const CELL_TEXT = '#1c1c1e'
const OUTSIDE_TEXT = '#c5c5c7'
const DISABLED_OPACITY = 0.35

const CONTENT_ENTER = FadeIn.duration(180)
const CONTENT_EXIT = FadeOut.duration(120)

function getMonthDays(year: number, month: number): Date[] {
  const firstDay = new Date(year, month, 1)
  const lastDay = new Date(year, month + 1, 0)
  const days: Date[] = []

  // Monday-start week padding
  const startPad = (firstDay.getDay() + 6) % 7
  for (let i = 0; i < startPad; i++) {
    days.push(new Date(year, month, -startPad + i + 1))
  }

  for (let i = 1; i <= lastDay.getDate(); i++) {
    days.push(new Date(year, month, i))
  }

  while (days.length < 42) {
    const last = days[days.length - 1]
    days.push(new Date(last.getFullYear(), last.getMonth(), last.getDate() + 1))
  }

  return days
}

function isoToDate(iso: string): Date | null {
  if (!iso) return null
  const parts = iso.split('-')
  if (parts.length !== 3) return null
  return new Date(+parts[0], +parts[1] - 1, +parts[2])
}

function dateToIso(date: Date): string {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  )
}

function formatMonth(date: Date, locale: string): string {
  try {
    return date.toLocaleDateString(locale, { month: 'long' })
  } catch {
    return date.toLocaleDateString('en-US', { month: 'long' })
  }
}

function formatMonthShort(date: Date, locale: string): string {
  try {
    return date.toLocaleDateString(locale, { month: 'short' })
  } catch {
    return date.toLocaleDateString('en-US', { month: 'short' })
  }
}

function formatYear(date: Date, locale: string): string {
  try {
    return date.toLocaleDateString(locale, { year: 'numeric' })
  } catch {
    return date.toLocaleDateString('en-US', { year: 'numeric' })
  }
}

export function Calendar({
  value,
  onChange,
  maximumDate,
  locale = 'en-US',
  onClose,
}: CalendarProps) {
  const selectedDate = useMemo(() => isoToDate(value), [value])
  const today = useMemo(() => new Date(), [])

  const initial = selectedDate || maximumDate || today
  const [viewMonth, setViewMonth] = useState(initial.getMonth())
  const [viewYear, setViewYear] = useState(initial.getFullYear())
  const [pickerMode, setPickerMode] = useState<'days' | 'months' | 'years'>('days')

  const days = useMemo(() => getMonthDays(viewYear, viewMonth), [viewYear, viewMonth])

  const goToPrevMonth = useCallback(() => {
    if (viewMonth === 0) {
      setViewMonth(11)
      setViewYear((y) => y - 1)
    } else {
      setViewMonth((m) => m - 1)
    }
  }, [viewMonth])

  const goToNextMonth = useCallback(() => {
    if (viewMonth === 11) {
      setViewMonth(0)
      setViewYear((y) => y + 1)
    } else {
      setViewMonth((m) => m + 1)
    }
  }, [viewMonth])

  const handleSelectDay = useCallback(
    (date: Date) => {
      onChange(dateToIso(date))
      onClose?.()
    },
    [onChange, onClose]
  )

  const handleSelectMonth = useCallback((monthIndex: number) => {
    setViewMonth(monthIndex)
    setPickerMode('days')
  }, [])

  const handleSelectYear = useCallback((year: number) => {
    setViewYear(year)
    setPickerMode('days')
  }, [])

  const toggleMonthPicker = useCallback(() => {
    setPickerMode((mode) => (mode === 'months' ? 'days' : 'months'))
  }, [])

  const toggleYearPicker = useCallback(() => {
    setPickerMode((mode) => (mode === 'years' ? 'days' : 'years'))
  }, [])

  const currentMonthDate = new Date(viewYear, viewMonth, 1)

  const maxYear = maximumDate ? maximumDate.getFullYear() : today.getFullYear()
  const minYear = maxYear - 100
  const years = useMemo(() => {
    const list: number[] = []
    for (let y = maxYear; y >= minYear; y--) {
      list.push(y)
    }
    return list
  }, [maxYear, minYear])

  const months = useMemo(() => {
    return Array.from({ length: 12 }, (_, i) => new Date(2021, i, 1))
  }, [])

  return (
    <YStack
      backgroundColor={CARD_BG}
      borderRadius={16}
      padding={16}
      width={320}
      maxWidth="92%"
      boxShadow="0px 8px 24px rgba(0,0,0,0.3)"
      elevation={12}
      onPress={(e) => e.stopPropagation()}
    >
      {/* Header: navigation + month/year selectors */}
      <XStack alignItems="center" justifyContent="space-between" marginBottom={16} gap={8}>
        <Pressable
          onPress={goToPrevMonth}
          hitSlop={8}
          style={{
            width: 32,
            height: 32,
            borderRadius: 16,
            backgroundColor: BUTTON_BG,
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <ChevronLeft size={18} color={BUTTON_TEXT} />
        </Pressable>

        <XStack gap={8}>
          <Pressable
            onPress={toggleMonthPicker}
            style={{
              flexDirection: 'row',
              alignItems: 'center',
              gap: 4,
              backgroundColor: BUTTON_BG,
              paddingHorizontal: 12,
              paddingVertical: 6,
              borderRadius: 8,
            }}
          >
            <Text color={BUTTON_TEXT} fontSize={15} fontWeight="600">
              {formatMonth(currentMonthDate, locale)}
            </Text>
            <ChevronDown size={12} color={ACCENT} />
          </Pressable>

          <Pressable
            onPress={toggleYearPicker}
            style={{
              flexDirection: 'row',
              alignItems: 'center',
              gap: 4,
              backgroundColor: BUTTON_BG,
              paddingHorizontal: 12,
              paddingVertical: 6,
              borderRadius: 8,
            }}
          >
            <Text color={BUTTON_TEXT} fontSize={15} fontWeight="600">
              {formatYear(currentMonthDate, locale)}
            </Text>
            <ChevronDown size={12} color={ACCENT} />
          </Pressable>
        </XStack>

        <Pressable
          onPress={goToNextMonth}
          hitSlop={8}
          style={{
            width: 32,
            height: 32,
            borderRadius: 16,
            backgroundColor: BUTTON_BG,
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <ChevronRight size={18} color={BUTTON_TEXT} />
        </Pressable>
      </XStack>

      {pickerMode === 'months' && (
        <Animated.View entering={CONTENT_ENTER} exiting={CONTENT_EXIT} style={{ width: '100%' }}>
          <XStack flexWrap="wrap" justifyContent="space-between" paddingBottom={4}>
            {months.map((monthDate, index) => {
              const isSelected = index === viewMonth
              return (
                <Pressable
                  key={index}
                  onPress={() => handleSelectMonth(index)}
                  style={{
                    width: 84,
                    height: 44,
                    marginBottom: 8,
                    borderRadius: 10,
                    backgroundColor: isSelected ? ACCENT : BUTTON_BG,
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <Text color={isSelected ? '#fff' : BUTTON_TEXT} fontSize={14} fontWeight={isSelected ? '700' : '500'}>
                    {formatMonthShort(monthDate, locale)}
                  </Text>
                </Pressable>
              )
            })}
          </XStack>
        </Animated.View>
      )}

      {pickerMode === 'years' && (
        <Animated.View entering={CONTENT_ENTER} exiting={CONTENT_EXIT} style={{ width: '100%' }}>
          <ScrollView style={{ height: 280 }} showsVerticalScrollIndicator>
            <YStack gap={4} paddingBottom={4}>
              {years.map((year) => {
                const isSelected = year === viewYear
                return (
                  <Pressable
                    key={year}
                    onPress={() => handleSelectYear(year)}
                    style={{
                      height: 40,
                      borderRadius: 10,
                      backgroundColor: isSelected ? ACCENT : BUTTON_BG,
                      alignItems: 'center',
                      justifyContent: 'center',
                    }}
                  >
                    <Text color={isSelected ? '#fff' : BUTTON_TEXT} fontSize={14} fontWeight={isSelected ? '700' : '500'}>
                      {year}
                    </Text>
                  </Pressable>
                )
              })}
            </YStack>
          </ScrollView>
        </Animated.View>
      )}

      {pickerMode === 'days' && (
        <Animated.View entering={CONTENT_ENTER} exiting={CONTENT_EXIT} style={{ width: '100%' }}>
          {/* Weekday headers */}
          <XStack justifyContent="space-between" marginBottom={8}>
            {WEEKDAYS.map((day) => (
              <Text
                key={day}
                color={BUTTON_TEXT}
                fontSize={13}
                fontWeight="600"
                width={36}
                textAlign="center"
                opacity={0.7}
              >
                {day}
              </Text>
            ))}
          </XStack>

          {/* Day grid */}
          <YStack gap={4}>
            {Array.from({ length: 6 }, (_, weekIndex) => (
              <XStack key={weekIndex} justifyContent="space-between">
                {days.slice(weekIndex * 7, weekIndex * 7 + 7).map((day, dayIndex) => {
                  const isCurrentMonth = day.getMonth() === viewMonth
                  const isSelected = selectedDate ? isSameDay(day, selectedDate) : false
                  const isDisabled = maximumDate ? day > maximumDate : false
                  const canSelect = isCurrentMonth && !isDisabled

                  return (
                    <Pressable
                      key={dayIndex}
                      disabled={!canSelect}
                      onPress={canSelect ? () => handleSelectDay(day) : undefined}
                      style={{ width: 36, height: 36, opacity: isDisabled ? DISABLED_OPACITY : 1 }}
                    >
                      <YStack
                        width={36}
                        height={36}
                        alignItems="center"
                        justifyContent="center"
                        borderRadius={10}
                        backgroundColor={isSelected ? ACCENT : CELL_BG}
                      >
                        <Text
                          color={isSelected ? '#fff' : !isCurrentMonth ? OUTSIDE_TEXT : CELL_TEXT}
                          fontSize={15}
                          fontWeight={isSelected ? '700' : '500'}
                        >
                          {day.getDate()}
                        </Text>
                      </YStack>
                    </Pressable>
                  )
                })}
              </XStack>
            ))}
          </YStack>
        </Animated.View>
      )}
    </YStack>
  )
}
