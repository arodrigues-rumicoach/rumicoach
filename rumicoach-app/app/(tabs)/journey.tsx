import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { Pressable } from 'react-native'
import { YStack, Text, H1 } from 'tamagui'
import { router, useFocusEffect } from 'expo-router'
import { useBlurTarget } from '@/context/BlurContext'
import { useAuth } from '@/hooks/useAuth'
import { api, type JourneyData, type Action, type MemoryPaginatedResponse, type Memory } from '@/api'
import { ContentLayout, TabScreenWrapper } from '@/components/templates'
import i18n from '@/i18n'
import { GlassCard, ThemedSpinner } from '@/components/atoms'
import { useSession } from '@/hooks/useSession'
import { QuoteCard, ActionsCard, CheckinCard, SessionCarousel, getSessionDetails, getGreeting } from '@/components/organisms/journey'
import type { CarouselItem } from '@/components/organisms/journey/SessionCarouselCard'
import { useSettings } from '@/hooks/useSettings'
import Reanimated, { FadeInDown } from 'react-native-reanimated'
import { trackCommitmentToggled, trackSessionStartTapped } from '@/analytics'

function JourneyScreen() {
  const blurTargetRef = useBlurTarget()
  const { isConnected, tasksVersion } = useSession()
  const { user, appLanguage, ensureValidToken } = useAuth()
  const { applyRemoteTheme } = useSettings()
  const [journeyData, setJourneyData] = useState<JourneyData | null>(null)
  const [commitments, setCommitments] = useState<Action[]>([])
  const [latestInsight, setLatestInsight] = useState<Memory | null>(null)
  const [loading, setLoading] = useState(true)

  const userName = user?.name || ''

  const getWeekdayNames = useCallback((days?: number[]) => {
    if (!days || days.length === 0) return ''
    const locale = appLanguage || 'en-US'
    return days.map((d) => {
      try {
        const base = new Date(2024, 0, d)
        return base.toLocaleDateString(locale, { weekday: 'short' })
      } catch {
        return ''
      }
    }).filter(Boolean).join(', ')
  }, [appLanguage])

  const formatGoalDate = useCallback((dateString?: string) => {
    if (!dateString) return ''
    try {
      return new Date(dateString).toLocaleDateString(appLanguage || 'en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      })
    } catch {
      return dateString || ''
    }
  }, [appLanguage])

  const fetchJourneyData = useCallback(async () => {
    try {
      await ensureValidToken()
      // Commitments are their own resource now (/journey no longer carries
      // them); it returns present and future ones already ordered by next occurrence.
      const [journeyRes, commitmentsRes, memoriesRes] = await Promise.allSettled([
        api.get<JourneyData>('/journey'),
        api.get<Action[]>('/commitments'),
        api.get<MemoryPaginatedResponse>('/memories'),
      ])
      if (journeyRes.status === 'fulfilled') {
        const data = journeyRes.value.data
        setJourneyData(data)
        // The backend picks a growth theme at the end of each session; apply it
        // without persisting it as the user's manual settings choice.
        if (data.theme) applyRemoteTheme(data.theme)
      } else if (__DEV__) {
        console.error('Failed to fetch growth data', journeyRes.reason)
      }
      if (commitmentsRes.status === 'fulfilled') {
        setCommitments(commitmentsRes.value.data ?? [])
      } else if (__DEV__) {
        console.error('Failed to fetch commitments', commitmentsRes.reason)
      }
      if (memoriesRes.status === 'fulfilled') {
        setLatestInsight(memoriesRes.value.data.items.find((m) => m.category === 'insight') ?? null)
      }
    } catch (err) {
      if (__DEV__) console.error('Failed to fetch growth data', err)
    } finally {
      setLoading(false)
    }
  }, [ensureValidToken, applyRemoteTheme])

  // No key-based remount here: tabs stay mounted on native (see the tabs
  // layout), and forcing a remount on focus restarts every entering
  // animation a frame after the content has painted — visible as a flash
  // on Android. The refetch alone keeps the screen fresh.
  useFocusEffect(
    useCallback(() => {
      fetchJourneyData()
    }, [fetchJourneyData]),
  )

  const mountedRef = useRef(false)
  useEffect(() => {
    if (!mountedRef.current) {
      mountedRef.current = true
      return
    }
    fetchJourneyData()
  }, [fetchJourneyData, tasksVersion])

  const toggleAction = useCallback(async (actionId: string, currentStatus: string) => {
    const isCompleted = currentStatus === 'completed'
    const newDone = !isCompleted
    const optimistic = (done: boolean) => setCommitments(prev => prev.map(a =>
      a.id === actionId ? { ...a, status: done ? 'completed' as const : 'pending' as const } : a
    ))

    optimistic(newDone)

    try {
      await ensureValidToken()
      // PATCH /commitments/{id} returns the updated commitment, not the whole screen.
      const { data } = await api.patch<Action>(`/commitments/${actionId}`, { done: newDone })
      setCommitments(prev => prev.map(a => (a.id === actionId ? data : a)))
      // Only after the server agrees — the optimistic flip above gets rolled back
      // on failure, and an event for a toggle that never landed is a lie.
      trackCommitmentToggled(newDone, data.type)
    } catch (err) {
      if (__DEV__) console.error('Failed to update commitment status', err)
      optimistic(isCompleted)
    }
  }, [ensureValidToken])

  // `origin` separates the two entry points, which look identical downstream but
  // mean different things: the carousel is a deliberate pick, the check-in card
  // is the daily nudge.
  const handleStartSession = useCallback((sessionType: string, origin = 'carousel') => {
    trackSessionStartTapped(sessionType, origin)
    router.push({ pathname: '/(tabs)/session', params: { autoStart: 'true', sessionType } })
  }, [])

  const handleStartCheckin = useCallback(() => {
    handleStartSession('checkin', 'checkin_card')
  }, [handleStartSession])

  const [now] = useState(() => Date.now())

  const carouselItems = useMemo(() => {
    const items: CarouselItem[] = []

    const todaySession = journeyData?.session
    if (todaySession) {
      const details = getSessionDetails(todaySession)
      if (details) {
        items.push({
          session: todaySession,
          title: details.title,
          subtitle: details.subtitle,
          actionText: journeyData?.mode === 'resume'
            ? (i18n.t('resume') || 'Resume')
            : details.actionText,
          imageUrl: details.imageUrl,
          isAvailable: true,
        })
      }
    }

    if (journeyData?.sessions) {
      for (const sessionInfo of journeyData.sessions) {
        if (sessionInfo.session === todaySession) continue
        const details = getSessionDetails(sessionInfo.session)
        if (details) {
          const isAvailable = new Date(sessionInfo.availableAt).getTime() <= now
          items.push({
            session: sessionInfo.session,
            title: details.title,
            subtitle: details.subtitle,
            actionText: details.actionText,
            imageUrl: details.imageUrl,
            isAvailable,
            availableAt: sessionInfo.availableAt,
          })
        }
      }
    }

    return items
  }, [journeyData, now])

  if (loading) {
    return (
      <YStack flex={1} backgroundColor="transparent" justifyContent="center" alignItems="center">
        <ThemedSpinner size="large" />
      </YStack>
    )
  }

  return (
    <ContentLayout scrollable showsVerticalScrollIndicator={false} showsHorizontalScrollIndicator={false}>
      <YStack marginTop={isConnected ? 16 : 0}>
        <YStack gap="$6" padding="$4">
          <Reanimated.View entering={FadeInDown.duration(400).springify().damping(20).stiffness(200)}>
            {/* The video has no scrim, so even the greeting sits on glass —
              nothing floats bare on the video (see src/styles/glass.ts). */}
            <GlassCard variant="light" borderRadius={20} padding={18} gap={2} blurTarget={blurTargetRef}>
              <H1 color="$onGlass" fontSize={28} lineHeight={36}>
                {getGreeting()}{userName ? `, ${userName}` : ''}.
              </H1>
              <Text color="$onGlassSecondary" fontSize={14}>
                {journeyData?.session === 'onboarding' || journeyData?.session === 'session_vision'
                  ? (i18n.t('journey_sanctuary_ready_first') || 'Your space for clearer thinking is ready.')
                  : (i18n.t('journey_sanctuary_ready_next') || "Let's take another step in the life you are building.")}
              </Text>
            </GlassCard>
          </Reanimated.View>
        </YStack>

        <YStack gap="$6" padding="$4">
          {journeyData?.quote ? (
            <Reanimated.View entering={FadeInDown.delay(100).duration(400).springify().damping(20).stiffness(200)}>
              <QuoteCard
                quote={journeyData.quote.quote}
                author={journeyData.quote.author}
                blurTargetRef={blurTargetRef}
              />
            </Reanimated.View>
          ) : null}
        </YStack>

        <Reanimated.View entering={FadeInDown.delay(200).duration(400).springify().damping(20).stiffness(200)}>
          <YStack gap="$3">
            {carouselItems.length > 0 && (
              <SessionCarousel
                items={carouselItems}
                onStartSession={handleStartSession}
                blurTargetRef={blurTargetRef}
              />
            )}

            <YStack gap="$6" padding="$4">
              <ActionsCard
                actions={commitments}
                onToggleAction={toggleAction}
                getWeekdayNames={getWeekdayNames}
                formatDate={formatGoalDate}
                blurTargetRef={blurTargetRef}
              />

              {carouselItems.length === 0 && (
                <CheckinCard onStart={handleStartCheckin} blurTargetRef={blurTargetRef} />
              )}
            </YStack>
          </YStack>
        </Reanimated.View>

        <YStack gap="$6" padding="$4">
          {latestInsight ? (
            <Reanimated.View entering={FadeInDown.delay(300).duration(400).springify().damping(20).stiffness(200)}>
              <GlassCard variant="light" gap={14} blurTarget={blurTargetRef}>
                {/* Same ink treatment as every other card label — no accent colour. */}
                <Text fontSize={12} fontWeight="700" letterSpacing={1} color="$onGlassSecondary" textTransform="uppercase">
                  {i18n.t('journey_from_insight') || 'From insight'}
                </Text>
                <Text fontSize={17} lineHeight={26} color="$onGlass">
                  “{latestInsight.content}”
                </Text>
                <Pressable
                  onPress={() => router.push({ pathname: '/(tabs)/memories', params: { category: 'insight' } })}
                  hitSlop={8}
                  accessibilityRole="button"
                  style={{ alignSelf: 'flex-start' }}
                >
                  {/* Underline carries the link affordance now that colour doesn't. */}
                  <Text fontSize={14} fontWeight="600" color="$onGlass" textDecorationLine="underline">
                    {(i18n.t('journey_view_in_memories') || 'View in Memories') + ' →'}
                  </Text>
                </Pressable>
              </GlassCard>
            </Reanimated.View>
          ) : null}
        </YStack>

      </YStack>
    </ContentLayout>
  )
}

export default function JourneyScreenWrapper() {
  return (
    <TabScreenWrapper>
      <JourneyScreen />
    </TabScreenWrapper>
  )
}
