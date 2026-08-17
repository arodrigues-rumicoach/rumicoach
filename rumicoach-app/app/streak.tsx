import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { View, ScrollView, Pressable, StyleSheet, Modal } from 'react-native'
import { Text } from 'tamagui'
import { Stack } from 'expo-router'
import { MessageCircle, Smile, Check, ChevronLeft, ChevronRight, ChevronDown } from 'lucide-react-native'
import { fetchUsageCalendar, type UsageCalendarResponse, type CommunicationSession } from '@/api'
import { useAuth } from '@/hooks/useAuth'
import { GlassCard } from '@/components/atoms'
import { WebMaxWidth } from '@/components/templates'
import { CommunicationSessionCard, PageHeader } from '@/components/organisms'
import i18n from '@/i18n'
import { ScrollNavProvider } from '@/context/ScrollNavContext'
import { haptic } from '@/utils/haptics'

// ── month activity data ────────────────────────────────────────────────────
// TODO(backend): replace the mock generator with a real month-scoped fetch
// (e.g. GET /usage-calendar?month=YYYY-MM) once the endpoint exists. The
// screen is already structured per-month so prev/next navigation and a real
// data source can be wired in without reshaping the UI. Mock pattern follows
// the same precedent as the sessions list in app/(settings)/usage.tsx.
type ActivityKind = 'session' | 'checkin' | 'none'

interface ActivityEntry {
  icon: 'session' | 'mood' | 'check'
  title: string
  meta: string
}

interface DayActivity {
  kind: ActivityKind
  minutes: number
  entries: ActivityEntry[]
  sessions: CommunicationSession[]
}





function getMonthActivity(calendar: UsageCalendarResponse, year: number, month: number, today: number): Record<number, DayActivity> {
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  const activity: Record<number, DayActivity> = {}

  for (let d = 1; d <= daysInMonth; d++) {
    const dateStr = `${year}-${String(month + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    const dayData = calendar.days?.[dateStr]

    const kind: ActivityKind = dayData?.kind || 'none'
    const entries: ActivityEntry[] = []
    const sessions = dayData?.sessions || []
    let minutes = 0

    if (sessions.length > 0) {
      sessions.forEach(s => {
        const dur = Math.round((s.duration || 0) / 60)
        minutes += dur
      })
    }

    if (kind === 'checkin') {
      minutes += 2 // Give 2 min for checkin to show something
      entries.push({
        icon: 'mood',
        title: `Daily Check-in`,
        meta: i18n.t('streak_daily_checkin') || 'Daily check-in',
      })
    }

    activity[d] = { kind, minutes, entries, sessions }
  }
  return activity
}

// ── design tokens (see design/design_handoff_streak_calendar/README.md) ────
const ACTIVITY_BLUE = (a: number) => `rgba(62,107,153,${a})`
const STREAK_ORANGE_LIGHT = '#fdba74'

const ENTRY_ICONS = {
  session: MessageCircle,
  mood: Smile,
  check: Check,
} as const

function StatCard({ value, label }: { value: string; label: string }) {
  return (
    <GlassCard variant="light" padding={14} borderRadius={14} style={{ flex: 1, alignItems: 'center', justifyContent: 'center' }}>
      <Text fontSize={24} fontWeight="700" color="$onGlass" textAlign='center'>
        {value}
      </Text>
      <Text fontSize={11} color="$onGlassSecondary" marginTop={2} textAlign='center'>
        {label}
      </Text>
    </GlassCard>
  )
}

export default function StreakScreen() {
  const { user, appLanguage, ensureValidToken } = useAuth()
  const [calendar, setCalendar] = useState<UsageCalendarResponse | null>(null)
  const now = useMemo(() => new Date(), [])
  const [viewYear, setViewYear] = useState(now.getFullYear())
  const [viewMonth, setViewMonth] = useState(now.getMonth())
  const today = now.getDate()
  const [selectedDay, setSelectedDay] = useState(today)
  const [pickerMode, setPickerMode] = useState<'none' | 'month' | 'year'>('none')
  const monthScrollRef = useRef<ScrollView>(null)
  const yearScrollRef = useRef<ScrollView>(null)

  useEffect(() => {
    let mounted = true
      ; (async () => {
        try {
          await ensureValidToken()
          const monthStr = `${viewYear}-${String(viewMonth + 1).padStart(2, '0')}`
          const data = await fetchUsageCalendar(monthStr)
          if (mounted) setCalendar(data)
        } catch (err) {
          if (__DEV__) console.error('Failed to fetch calendar', err)
        }
      })()
    return () => {
      mounted = false
    }
  }, [ensureValidToken, viewYear, viewMonth])

  const activity = useMemo(() => calendar ? getMonthActivity(calendar, viewYear, viewMonth, today) : {}, [calendar, viewYear, viewMonth, today])

  const daysInMonth = new Date(viewYear, viewMonth + 1, 0).getDate()
  // Monday-start offset: JS getDay() is Sunday-first.
  const leadingBlanks = (new Date(viewYear, viewMonth, 1).getDay() + 6) % 7

  const locale = appLanguage || 'en-US'
  const currentMonthDate = new Date(viewYear, viewMonth, 1)
  const monthTitle = currentMonthDate.toLocaleDateString(locale, { month: 'long' })
  const yearTitle = currentMonthDate.toLocaleDateString(locale, { year: 'numeric' })

  const sessionsThisMonth = calendar?.sessionsCount || 0
  const totalHoursFloat = calendar?.hours || 0
  const hours = Math.floor(totalHoursFloat)
  const mins = Math.round((totalHoursFloat - hours) * 60)
  const timeLabel = hours > 0 ? `${hours}h ${mins}m` : `${mins}m`

  const weekdayLabels = useMemo(() => {
    // Monday-first single-letter labels in the user's locale (Jan 2024: the
    // 1st was a Monday).
    return Array.from({ length: 7 }, (_, i) =>
      new Date(2024, 0, 1 + i).toLocaleDateString(locale, { weekday: 'narrow' }),
    )
  }, [locale])

  const selected = activity[selectedDay]
  const selectedDate = new Date(viewYear, viewMonth, selectedDay)
  const dayTitle = selectedDate.toLocaleDateString(locale, { weekday: 'long', month: 'long', day: 'numeric' })
  const dayTotal = selected && selected.minutes > 0
    ? (i18n.t('streak_min_total', { min: selected.minutes }) || `${selected.minutes} min total`)
    : ''

  const onSelectDay = useCallback((d: number) => setSelectedDay(d), [])

  const goToPrevMonth = useCallback(() => {
    haptic.light()
    if (viewMonth === 0) {
      setViewMonth(11)
      setViewYear((y) => y - 1)
    } else {
      setViewMonth((m) => m - 1)
    }
    setSelectedDay(1)
  }, [viewMonth])

  const goToNextMonth = useCallback(() => {
    haptic.light()
    if (viewMonth === 11) {
      setViewMonth(0)
      setViewYear((y) => y + 1)
    } else {
      setViewMonth((m) => m + 1)
    }
    setSelectedDay(1)
  }, [viewMonth])

  const isCurrentMonth = viewYear === now.getFullYear() && viewMonth === now.getMonth()

  const MONTH_NAMES = useMemo(() => {
    return Array.from({ length: 12 }, (_, i) => {
      const d = new Date(2021, i, 1)
      try {
        return d.toLocaleDateString(locale, { month: 'long' })
      } catch {
        return d.toLocaleDateString('en-US', { month: 'long' })
      }
    })
  }, [locale])

  const YEARS = useMemo(() => {
    const startYear = 2026
    const list: number[] = []
    for (let y = startYear; y < startYear + 10; y++) list.push(y)
    return list
  }, [])

  // Auto-scroll to selected item when picker opens
  useEffect(() => {
    if (pickerMode === 'month') {
      const offset = Math.max(0, viewMonth * 52 - 140)
      setTimeout(() => monthScrollRef.current?.scrollTo({ y: offset, animated: false }), 50)
    } else if (pickerMode === 'year') {
      const index = YEARS.indexOf(viewYear)
      if (index >= 0) {
        const offset = Math.max(0, index * 52 - 140)
        setTimeout(() => yearScrollRef.current?.scrollTo({ y: offset, animated: false }), 50)
      }
    }
  }, [pickerMode, viewMonth, viewYear, YEARS])

  return (
    <ScrollNavProvider>
      <Stack.Screen options={{ headerShown: false }} />
      <WebMaxWidth>
        <PageHeader title={i18n.t('streak_title') || 'Your Streak'} canGoBack />
        <ScrollView
          showsVerticalScrollIndicator={false}
          contentContainerStyle={[styles.content]}
        >
          {/* Month navigator */}
          <GlassCard variant="light" borderRadius={14} padding={0} style={{ marginBottom: 16 }}>
            <View style={styles.monthNav}>
              <Pressable
                onPress={goToPrevMonth}
                hitSlop={8}
                style={styles.monthNavBtn}
                accessibilityRole="button"
                accessibilityLabel="Previous month"
              >
                <ChevronLeft size={18} color="rgba(0,0,0,0.5)" />
              </Pressable>

              <View style={styles.monthNavCenter}>
                <Pressable
                  onPress={() => setPickerMode(pickerMode === 'month' ? 'none' : 'month')}
                  hitSlop={4}
                  style={styles.monthNavLabel}
                >
                  <Text fontSize={15} fontWeight="600" color="$onGlass" style={{ textTransform: 'capitalize' }}>
                    {monthTitle}
                  </Text>
                  <ChevronDown size={12} color="rgba(0,0,0,0.35)" />
                </Pressable>

                <Pressable
                  onPress={() => setPickerMode(pickerMode === 'year' ? 'none' : 'year')}
                  hitSlop={4}
                  style={styles.monthNavLabel}
                >
                  <Text fontSize={15} fontWeight="600" color="$onGlass">
                    {yearTitle}
                  </Text>
                  <ChevronDown size={12} color="rgba(0,0,0,0.35)" />
                </Pressable>
              </View>

              <Pressable
                onPress={goToNextMonth}
                hitSlop={8}
                style={styles.monthNavBtn}
                disabled={isCurrentMonth}
                accessibilityRole="button"
                accessibilityLabel="Next month"
              >
                <ChevronRight size={18} color={isCurrentMonth ? 'rgba(0,0,0,0.15)' : 'rgba(0,0,0,0.5)'} />
              </Pressable>
            </View>
          </GlassCard>

          {/* Month picker modal */}
          <Modal visible={pickerMode === 'month'} transparent animationType="fade" onRequestClose={() => setPickerMode('none')}>
            <Pressable style={styles.pickerBackdrop} onPress={() => setPickerMode('none')}>
              <Pressable style={styles.pickerCard} onPress={(e) => e.stopPropagation()}>
                <Text fontSize={15} fontWeight="700" color="$onGlass" textAlign="center" marginBottom={12}>
                  {i18n.t('streak_select_month') || 'Select month'}
                </Text>
                <ScrollView ref={monthScrollRef} style={{ maxHeight: 340 }} showsVerticalScrollIndicator={false}>
                  <View style={styles.scrollList}>
                    {MONTH_NAMES.map((name, index) => {
                      const isSelected = index === viewMonth
                      return (
                        <Pressable
                          key={index}
                          onPress={() => {
                            haptic.selection()
                            setViewMonth(index)
                            setSelectedDay(1)
                            setPickerMode('none')
                          }}
                          style={[
                            styles.scrollItem,
                            isSelected && styles.scrollItemSelected,
                          ]}
                        >
                          <Text
                            fontSize={16}
                            fontWeight={isSelected ? '700' : '500'}
                            color={isSelected ? '#fff' : 'rgba(0,0,0,0.7)'}
                            style={{ textTransform: 'capitalize', width: "80%" }}
                          >
                            {name}
                          </Text>
                          {isSelected && (
                            <View style={styles.scrollItemCheck}>
                              <Text fontSize={14} fontWeight="700" color="#fff">✓</Text>
                            </View>
                          )}
                        </Pressable>
                      )
                    })}
                  </View>
                </ScrollView>
              </Pressable>
            </Pressable>
          </Modal>

          {/* Year picker modal */}
          <Modal visible={pickerMode === 'year'} transparent animationType="fade" onRequestClose={() => setPickerMode('none')}>
            <Pressable style={styles.pickerBackdrop} onPress={() => setPickerMode('none')}>
              <Pressable style={styles.pickerCard} onPress={(e) => e.stopPropagation()}>
                <Text fontSize={15} fontWeight="700" color="$onGlass" textAlign="center" marginBottom={12}>
                  {i18n.t('streak_select_year') || 'Select year'}
                </Text>
                <ScrollView ref={yearScrollRef} style={{ maxHeight: 340 }} showsVerticalScrollIndicator={false}>
                  <View style={styles.scrollList}>
                    {YEARS.map((year) => {
                      const isSelected = year === viewYear
                      return (
                        <Pressable
                          key={year}
                          onPress={() => {
                            haptic.selection()
                            setViewYear(year)
                            setSelectedDay(1)
                            setPickerMode('none')
                          }}
                          style={[
                            styles.scrollItem,
                            isSelected && styles.scrollItemSelected,
                          ]}
                        >
                          <Text
                            fontSize={16}
                            fontWeight={isSelected ? '700' : '500'}
                            color={isSelected ? '#fff' : 'rgba(0,0,0,0.7)'}
                          >
                            {year}
                          </Text>
                          {isSelected && (
                            <View style={styles.scrollItemCheck}>
                              <Text fontSize={14} fontWeight="700" color="#fff">✓</Text>
                            </View>
                          )}
                        </Pressable>
                      )
                    })}
                  </View>
                </ScrollView>
              </Pressable>
            </Pressable>
          </Modal>

          {/* Stats row */}
          <View style={styles.statsRow}>
            <StatCard
              value={calendar === null ? '—' : String(calendar.dayStreak || 0)}
              label={i18n.t('streak_day_streak') || 'day streak'}
            />
            <StatCard value={String(sessionsThisMonth)} label={i18n.t('streak_sessions') || 'sessions'} />
            <StatCard value={timeLabel} label={i18n.t('streak_this_month') || 'this month'} />
          </View>

          {/* Calendar card */}
          <GlassCard variant="light" borderRadius={18} padding={12} style={styles.card}>
            <View style={styles.weekdayRow}>
              {weekdayLabels.map((w, i) => (
                <View key={i} style={styles.gridCell}>
                  <Text fontSize={11} fontWeight="600" color="$onGlassSecondary" textAlign="center">
                    {w}
                  </Text>
                </View>
              ))}
            </View>
            <View style={styles.grid}>
              {Array.from({ length: leadingBlanks }, (_, i) => (
                <View key={`blank-${i}`} style={styles.gridCell} />
              ))}
              {Array.from({ length: daysInMonth }, (_, i) => {
                const d = i + 1
                const isFuture = d > today
                const isSelected = d === selectedDay
                const kind = activity[d]?.kind ?? 'none'
                return (
                  <View key={d} style={styles.gridCell}>
                    <Pressable
                      disabled={isFuture}
                      onPress={() => onSelectDay(d)}
                      style={[
                        styles.dayCircle,
                        kind === 'session' && !isFuture && { backgroundColor: ACTIVITY_BLUE(0.6) },
                        kind === 'checkin' && !isFuture && { backgroundColor: ACTIVITY_BLUE(0.22) },
                        isSelected && styles.daySelected,
                      ]}
                      accessibilityRole="button"
                      accessibilityState={{ selected: isSelected, disabled: isFuture }}
                    >
                      <Text
                        fontSize={12}
                        fontWeight={isSelected ? '700' : kind === 'session' ? '600' : '400'}
                        color={
                          isFuture
                            ? '$onGlassTertiary'
                            : kind === 'session' || isSelected
                              ? '#fff'
                              : kind === 'checkin'
                                ? '$onGlass'
                                : '$onGlassSecondary'
                        }
                      >
                        {d}
                      </Text>
                    </Pressable>
                  </View>
                )
              })}
            </View>
            <View style={styles.legend}>
              <View style={styles.legendItem}>
                <View style={[styles.legendDot, { backgroundColor: ACTIVITY_BLUE(0.6) }]} />
                <Text fontSize={11} color="$onGlassSecondary">{i18n.t('streak_legend_session') || 'Session'}</Text>
              </View>
              <View style={styles.legendItem}>
                <View style={[styles.legendDot, { backgroundColor: ACTIVITY_BLUE(0.22) }]} />
                <Text fontSize={11} color="$onGlassSecondary">{i18n.t('streak_legend_checkin') || 'Check-in only'}</Text>
              </View>
              <View style={styles.legendItem}>
                <View style={[styles.legendDot, styles.legendDotEmpty]} />
                <Text fontSize={11} color="$onGlassSecondary">{i18n.t('streak_legend_none') || 'No activity'}</Text>
              </View>
            </View>
          </GlassCard>

          {/* Day detail card — instant swap on selection per the handoff */}
          <View key={selectedDay}>
            <GlassCard variant="light" borderRadius={18} padding={16} style={styles.card}>
              <View style={styles.detailHeader}>
                <Text fontSize={15} fontWeight="700" color="$onGlass">
                  {dayTitle}
                </Text>
                {dayTotal ? (
                  <Text fontSize={12} color="$onGlassSecondary">
                    {dayTotal}
                  </Text>
                ) : null}
              </View>
              {selected && (selected.sessions.length > 0 || selected.entries.length > 0) ? (
                <View style={styles.entries}>
                  {selected.sessions.map((s, i) => (
                    <CommunicationSessionCard key={`session-${i}`} item={s} userLanguage={user?.preferredLanguage} />
                  ))}
                  {selected.entries.map((e, i) => {
                    const Icon = ENTRY_ICONS[e.icon]
                    return (
                      <View key={`entry-${i}`} style={styles.entryRow}>
                        <View style={styles.entryAvatar}>
                          <Icon size={15} color="#b5d2ee" strokeWidth={2.5} />
                        </View>
                        <View style={styles.entryBody}>
                          <Text fontSize={13.5} fontWeight="600" color="$onGlass">
                            {e.title}
                          </Text>
                          <Text fontSize={11.5} color="$onGlassSecondary" marginTop={1}>
                            {e.meta}
                          </Text>
                        </View>
                      </View>
                    )
                  })}
                </View>
              ) : (
                <View style={styles.emptyState}>
                  <Text fontSize={13} color="$onGlassSecondary" textAlign="center" lineHeight={19.5}>
                    {i18n.t('streak_no_activity') ||
                      'No activity this day. Streaks grow one day at a time — a 2-minute check-in counts.'}
                  </Text>
                </View>
              )}
            </GlassCard>
          </View>
        </ScrollView>
      </WebMaxWidth>
    </ScrollNavProvider>
  )
}

const styles = StyleSheet.create({
  gradient: {
    flex: 1,
  },
  content: {
    width: '100%',
    alignSelf: 'center',
    paddingHorizontal: 16,
  },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 14,
    marginBottom: 22,
  },
  backButton: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: 'rgba(46,74,105,0.55)',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.14)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  statsRow: {
    flexDirection: 'row',
    gap: 8,
    marginBottom: 16,
  },
  card: {
    marginBottom: 12,
  },
  weekdayRow: {
    flexDirection: 'row',
    marginBottom: 4,
  },
  grid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
  },
  gridCell: {
    width: `${100 / 7}%`,
    padding: 3,
    alignItems: 'center',
    justifyContent: 'center',
  },
  dayCircle: {
    width: 36,
    height: 36,
    borderRadius: 18,
    alignItems: 'center',
    justifyContent: 'center',
  },
  daySelected: {
    backgroundColor: ACTIVITY_BLUE(0.65),
    borderWidth: 2,
    borderColor: STREAK_ORANGE_LIGHT,
  },
  legend: {
    flexDirection: 'row',
    justifyContent: 'center',
    gap: 12,
    marginTop: 8,
    paddingTop: 8,
    borderTopWidth: 1,
    borderTopColor: 'rgba(0,0,0,0.08)',
  },
  legendItem: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  legendDot: {
    width: 10,
    height: 10,
    borderRadius: 5,
  },
  legendDotEmpty: {
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.25)',
  },
  detailHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'baseline',
    marginBottom: 12,
  },
  entries: {
    gap: 8,
  },
  entryRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    backgroundColor: 'rgba(0,0,0,0.03)',
    borderRadius: 12,
    paddingVertical: 11,
    paddingHorizontal: 12,
  },
  entryAvatar: {
    width: 34,
    height: 34,
    borderRadius: 17,
    backgroundColor: 'rgba(62,107,153,0.4)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  entryBody: {
    flex: 1,
    minWidth: 0,
  },
  emptyState: {
    paddingVertical: 14,
    paddingHorizontal: 10,
  },
  monthNav: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  monthNavBtn: {
    width: 32,
    height: 32,
    borderRadius: 16,
    alignItems: 'center',
    justifyContent: 'center',
  },
  monthNavCenter: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  monthNavLabel: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
  },
  pickerBackdrop: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.4)',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 32,
  },
  pickerCard: {
    width: '100%',
    maxWidth: 340,
    borderRadius: 18,
    padding: 20,
    backgroundColor: '#f4f4f5',
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.08)',
  },
  scrollList: {
    gap: 4,
  },
  scrollItem: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingVertical: 14,
    paddingHorizontal: 16,
    borderRadius: 12,
    backgroundColor: 'rgba(0,0,0,0.04)',
  },
  scrollItemSelected: {
    backgroundColor: 'rgba(62,107,153,0.85)',
  },
  scrollItemCheck: {
    width: 22,
    height: 22,
    borderRadius: 11,
    backgroundColor: 'rgba(255,255,255,0.25)',
    alignItems: 'center',
    justifyContent: 'center',
  },
})
