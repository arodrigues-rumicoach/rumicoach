import { useState, useCallback, useMemo } from 'react'
import { Pressable, ScrollView } from 'react-native'
import { YStack, XStack, Text, Button } from 'tamagui'
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-react-native'

interface WebCalendarProps {
  value: string
  onChange: (isoDate: string) => void
  maximumDate?: Date
  locale?: string
  onClose?: () => void
}

const WEEKDAYS = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa']

function getMonthDays(year: number, month: number): Date[] {
  const firstDay = new Date(year, month, 1)
  const lastDay = new Date(year, month + 1, 0)
  const days: Date[] = []

  // Pad start with empty slots for days before the 1st
  const startPad = firstDay.getDay()
  for (let i = 0; i < startPad; i++) {
    const d = new Date(year, month, -startPad + i + 1)
    days.push(d)
  }

  // Actual month days
  for (let i = 1; i <= lastDay.getDate(); i++) {
    days.push(new Date(year, month, i))
  }

  // Pad end to complete the grid (always show 6 rows = 42 cells)
  while (days.length < 42) {
    const last = days[days.length - 1]
    days.push(new Date(last.getFullYear(), last.getMonth(), last.getDate() + 1))
  }

  return days
}

function formatMonthYear(date: Date, locale: string): string {
  try {
    return date.toLocaleDateString(locale, { month: 'long', year: 'numeric' })
  } catch {
    return date.toLocaleDateString('en-US', { month: 'long', year: 'numeric' })
  }
}

function formatMonth(date: Date, locale: string): string {
  try {
    return date.toLocaleDateString(locale, { month: 'long' })
  } catch {
    return date.toLocaleDateString('en-US', { month: 'long' })
  }
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

function isoToDate(iso: string): Date | null {
  if (!iso) return null
  const parts = iso.split('-')
  if (parts.length !== 3) return null
  return new Date(+parts[0], +parts[1] - 1, +parts[2])
}

export function WebCalendar({
  value,
  onChange,
  maximumDate,
  locale = 'en-US',
  onClose,
}: WebCalendarProps) {
  const selectedDate = useMemo(() => isoToDate(value), [value])
  const today = useMemo(() => new Date(), [])

  const initial = selectedDate || maximumDate || today
  const [viewMonth, setViewMonth] = useState(initial.getMonth())
  const [viewYear, setViewYear] = useState(initial.getFullYear())
  const [showYearPicker, setShowYearPicker] = useState(false)

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

  const goToPrevYear = useCallback(() => {
    setViewYear((y) => y - 1)
  }, [])

  const goToNextYear = useCallback(() => {
    setViewYear((y) => y + 1)
  }, [])

  const handleSelect = useCallback(
    (date: Date) => {
      onChange(dateToIso(date))
      onClose?.()
    },
    [onChange, onClose]
  )

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

  const handleSelectYear = useCallback((year: number) => {
    setViewYear(year)
    setShowYearPicker(false)
  }, [])

  return (
    <YStack
      backgroundColor="rgba(0,0,0,0.85)"
      borderRadius={16}
      borderWidth={1}
      borderColor="rgba(255,255,255,0.12)"
      padding={16}
      width={320}
      boxShadow="0px 8px 16px rgba(0,0,0,0.4)"
      elevation={8}
      onPress={(e) => e.stopPropagation()}
    >
      {/* Month/Year header with navigation */}
      <XStack alignItems="center" justifyContent="space-between" marginBottom={16}>
        <XStack gap="$2">
          <Button
            size="$2"
            circular
            chromeless
            onPress={goToPrevYear}
            hoverStyle={{ backgroundColor: 'rgba(255,255,255,0.1)' }}
            pressStyle={{ backgroundColor: 'rgba(255,255,255,0.15)' }}
          >
            <ChevronsLeft size={18} color="rgba(255,255,255,0.7)" />
          </Button>

          <Button
            size="$2"
            circular
            chromeless
            onPress={goToPrevMonth}
            hoverStyle={{ backgroundColor: 'rgba(255,255,255,0.1)' }}
            pressStyle={{ backgroundColor: 'rgba(255,255,255,0.15)' }}
          >
            <ChevronLeft size={18} color="rgba(255,255,255,0.7)" />
          </Button>
        </XStack>

        <Button
          size="$2"
          chromeless
          onPress={() => setShowYearPicker((s) => !s)}
          hoverStyle={{ backgroundColor: 'rgba(255,255,255,0.1)' }}
          pressStyle={{ backgroundColor: 'rgba(255,255,255,0.15)' }}
        >
          <Text
            color="#fff"
            fontSize={16}
            fontWeight="600"
          >
            {showYearPicker ? formatMonth(currentMonthDate, locale) : formatMonthYear(currentMonthDate, locale)}
          </Text>
        </Button>

        <XStack gap="$2">
          <Button
            size="$2"
            circular
            chromeless
            onPress={goToNextMonth}
            hoverStyle={{ backgroundColor: 'rgba(255,255,255,0.1)' }}
            pressStyle={{ backgroundColor: 'rgba(255,255,255,0.15)' }}
          >
            <ChevronRight size={18} color="rgba(255,255,255,0.7)" />
          </Button>

          <Button
            size="$2"
            circular
            chromeless
            onPress={goToNextYear}
            hoverStyle={{ backgroundColor: 'rgba(255,255,255,0.1)' }}
            pressStyle={{ backgroundColor: 'rgba(255,255,255,0.15)' }}
          >
            <ChevronsRight size={18} color="rgba(255,255,255,0.7)" />
          </Button>
        </XStack>
      </XStack>

      {showYearPicker ? (
        <ScrollView style={{ height: 280 }} showsVerticalScrollIndicator>
          <XStack flexWrap="wrap" justifyContent="space-between" paddingBottom="$2">
            {years.map((year) => (
              <Button
                key={year}
                size="$3"
                width={72}
                marginBottom="$2"
                chromeless
                backgroundColor={year === viewYear ? '#10b981' : 'transparent'}
                onPress={() => handleSelectYear(year)}
                hoverStyle={{ backgroundColor: year === viewYear ? '#10b981' : 'rgba(255,255,255,0.1)' }}
                pressStyle={{ backgroundColor: year === viewYear ? '#10b981' : 'rgba(255,255,255,0.15)' }}
              >
                <Text color={year === viewYear ? '#000' : '#fff'} fontSize={14} fontWeight={year === viewYear ? '700' : '400'}>
                  {year}
                </Text>
              </Button>
            ))}
          </XStack>
        </ScrollView>
      ) : (
        <>
          {/* Weekday headers */}
          <XStack justifyContent="space-around" marginBottom={8}>
            {WEEKDAYS.map((day) => (
              <Text
                key={day}
                color="rgba(255,255,255,0.4)"
                fontSize={12}
                fontWeight="500"
                width={36}
                textAlign="center"
              >
                {day}
              </Text>
            ))}
          </XStack>

          {/* Day grid */}
          <YStack gap={4}>
            {Array.from({ length: 6 }, (_, weekIndex) => (
              <XStack key={weekIndex} justifyContent="space-around">
                {days.slice(weekIndex * 7, weekIndex * 7 + 7).map((day, dayIndex) => {
                  const isCurrentMonth = day.getMonth() === viewMonth
                  const isSelected = selectedDate ? isSameDay(day, selectedDate) : false
                  const isToday = isSameDay(day, today)
                  const isDisabled = maximumDate ? day > maximumDate : false
                  const canSelect = isCurrentMonth && !isDisabled

                  return (
                    <Pressable
                      key={dayIndex}
                      disabled={!canSelect}
                      onPress={canSelect ? () => handleSelect(day) : undefined}
                      style={{ width: 36, height: 36 }}
                    >
                      <YStack
                        width={36}
                        height={36}
                        alignItems="center"
                        justifyContent="center"
                        borderRadius={10}
                        backgroundColor={
                          isSelected
                            ? '#10b981'
                            : isToday
                              ? 'rgba(255,255,255,0.08)'
                              : 'transparent'
                        }
                        opacity={
                          !isCurrentMonth
                            ? 0.2
                            : isDisabled
                              ? 0.3
                              : 1
                        }
                        hoverStyle={
                          canSelect && !isSelected
                            ? { backgroundColor: 'rgba(255,255,255,0.1)' }
                            : undefined
                        }
                        pressStyle={
                          canSelect && !isSelected
                            ? { backgroundColor: 'rgba(255,255,255,0.15)' }
                            : undefined
                        }
                      >
                        <Text
                          color={isSelected ? '#000' : '#fff'}
                          fontSize={14}
                          fontWeight={isSelected || isToday ? '700' : '400'}
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
        </>
      )}
    </YStack>
  )
}
