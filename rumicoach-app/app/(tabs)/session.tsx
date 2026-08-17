import { useRef, useState, useCallback } from 'react'
import { YStack, Text } from 'tamagui'
import { useLocalSearchParams, router, useFocusEffect, useIsFocused } from 'expo-router'
import { useSession } from '../../src/hooks/useSession'
import i18n from '../../src/i18n'
import { SessionFeedbackModal } from '@/components/organisms'
import { ThemedButton, Heading } from '@/components/atoms'
import { ContentLayout, TabScreenWrapper } from '@/components/templates'
import Reanimated, { FadeInDown, FadeIn, FadeOut } from 'react-native-reanimated'
import { GlassPanel } from '@/components/molecules/GlassPanel'
import { PlayCircle, MessageCircle, Mic } from 'lucide-react-native'
import { haptic } from '@/utils/haptics'
import { useAuth } from '@/hooks/useAuth'
import { api, type JourneyData } from '@/api'

import { useSessionFeedbackTracker } from '../../src/hooks/useSessionFeedbackTracker'
import { ActiveSessionCard } from '@/components/organisms/ActiveSessionCard'
import { WheelPanel } from '@/components/organisms/WheelPanel'
import { EisenhowerPanel } from '@/components/organisms/EisenhowerPanel'
import { ActionPlanPanel } from '@/components/organisms/ActionPlanPanel'
import { SessionSummaryPanel } from '@/components/organisms/SessionSummaryPanel'
import { SessionTasksPanel } from '@/components/organisms/SessionTasksPanel'
import { SessionValuesPanel } from '@/components/organisms/SessionValuesPanel'
import { OnboardingDataPanel } from '@/components/organisms/OnboardingDataPanel'

const ROTATING_PROMPTS = [
  { titleKey: 'session_mind_prompt_1_title', subtitleKey: 'session_mind_prompt_1_subtitle' },
  { titleKey: 'session_mind_prompt_2_title', subtitleKey: 'session_mind_prompt_2_subtitle' },
  { titleKey: 'session_mind_prompt_3_title', subtitleKey: 'session_mind_prompt_3_subtitle' },
  { titleKey: 'session_mind_prompt_4_title', subtitleKey: 'session_mind_prompt_4_subtitle' },
  { titleKey: 'session_mind_prompt_5_title', subtitleKey: 'session_mind_prompt_5_subtitle' },
]

function SessionScreen() {
  const params = useLocalSearchParams<{ autoStart?: string; sessionType?: string }>()
  const { status, isConnected, connect, disconnect, durationSeconds, inputVolume, showWheel, wheelData, showEisenhower, eisenhowerData, showActionPlan, actionPlanData, showSessionSummary, sessionSummaryData, showSessionTasks, sessionTasksData, showSessionValues, sessionValuesData, showOnboardingData, userWSData, sessionId, sessionType } = useSession()
  const { ensureValidToken } = useAuth()

  const isConnectingRef = useRef(false)
  // Hold the rating modal while the session-end summary card is on screen and
  // the user is actually looking at it — it used to pop right on top of the
  // card they had just been invited to read (QA). It releases when the card
  // goes away or the user navigates on.
  const isFocused = useIsFocused()
  const { lastActiveSessionId, showFeedback, dismissFeedback } = useSessionFeedbackTracker(
    status, sessionId, durationSeconds, showSessionSummary && isFocused)

  // The idle card's copy depends on whether the onboarding session is still
  // pending — /journey reports it as the next session until it's done.
  // null = not yet known, so the card waits rather than flashing wrong copy.
  const [needsOnboarding, setNeedsOnboarding] = useState<boolean | null>(null)

  const promptIndexRef = useRef(Math.floor(Math.random() * ROTATING_PROMPTS.length))
  const [promptIndex, setPromptIndex] = useState(promptIndexRef.current)

  // The tab stays mounted between visits, so randomising on mount would only
  // pick an opening prompt once per app launch. Re-roll on every focus, with an
  // offset of at least 1 so reopening never lands on the prompt already shown.
  useFocusEffect(
    useCallback(() => {
      const offset = 1 + Math.floor(Math.random() * (ROTATING_PROMPTS.length - 1))
      promptIndexRef.current = (promptIndexRef.current + offset) % ROTATING_PROMPTS.length
      setPromptIndex(promptIndexRef.current)

      const interval = setInterval(() => {
        promptIndexRef.current = (promptIndexRef.current + 1) % ROTATING_PROMPTS.length
        setPromptIndex(promptIndexRef.current)
      }, 7000)

      return () => clearInterval(interval)
    }, []),
  )

  const fetchNextSession = useCallback(async () => {
    try {
      await ensureValidToken()
      const { data } = await api.get<JourneyData>('/journey')
      // The opening arc is the intro plus the Vision session it hands over to; both
      // are a first-time user's experience, so both get the first-time copy.
      setNeedsOnboarding(data.session === 'onboarding' || data.session === 'session_vision')
    } catch (err) {
      if (__DEV__) console.error('Failed to fetch next session', err)
      // Fall back to the returning-user copy rather than blocking the CTA.
      setNeedsOnboarding(false)
    }
  }, [ensureValidToken])

  // Refetch on focus and whenever a session ends — finishing onboarding is
  // exactly the moment this copy needs to change.
  useFocusEffect(
    useCallback(() => {
      if (!isConnected) fetchNextSession()
    }, [fetchNextSession, isConnected]),
  )

  useFocusEffect(
    useCallback(() => {
      if (params.autoStart === 'true' && status === 'disconnected' && !isConnectingRef.current) {
        isConnectingRef.current = true
        connect(false, params.sessionType)
        router.setParams({ autoStart: '', sessionType: '' })
      }
    }, [params.autoStart, params.sessionType, status, connect]),
  )

  const showingResultsPanel = showWheel || showEisenhower || showActionPlan || showSessionSummary || showSessionTasks || showSessionValues || showOnboardingData

  // The session's name, shown as a small header while connected — the user's "mental
  // GPS" for which session (structured or free) they are in (QA). The server announces
  // the resolved type on session_created and again when a gateway hands over, so this
  // relabels itself mid-session. Unknown/missing type renders nothing.
  const sessionName = sessionType
    ? i18n.t(`session_name_${sessionType.replace(/^session_/, '')}`, { defaultValue: '' })
    : ''

  return (
    // Results panels (the wheel mid-session, the summary card at session end) can grow
    // taller than the screen — the summary card's blocks were unreachable below the
    // fold with no way to scroll (QA). The idle/active states stay centered and fixed.
    <ContentLayout scrollable={showingResultsPanel} centered={!showingResultsPanel}>
      {isConnected && sessionName !== '' && (
        <Reanimated.View entering={FadeIn.duration(400)} style={{ width: '100%', alignItems: 'center' }}>
          <Text
            fontSize={12}
            letterSpacing={1.5}
            textTransform="uppercase"
            color="$onGlassSecondary"
            marginBottom="$3"
          >
            {sessionName}
          </Text>
        </Reanimated.View>
      )}
      {!isConnected ? (
        <>
          {!showFeedback && needsOnboarding !== null && (
            <Reanimated.View entering={FadeInDown.duration(400).springify().damping(20).stiffness(200)} style={{ width: '100%' }}>
              <GlassPanel variant="light">
                <YStack alignItems="center" gap="$4">
                  {needsOnboarding ? (
                    <>
                      <Heading color="$onGlass" fontSize={22} fontWeight="600" textAlign="center">
                        {(i18n.t('session_vision_title') || 'Your vision')}
                      </Heading>
                      <Text color="$onGlassSecondary" fontSize={14} textAlign="center">
                        {(i18n.t('session_vision_subtitle') || 'Build a clear vision of the life you want to live')}
                      </Text>
                    </>
                  ) : (
                    <Reanimated.View key={promptIndex} entering={FadeIn.duration(500)} exiting={FadeOut.duration(300)} style={{ alignItems: 'center' }}>
                      <Heading color="$onGlass" fontSize={22} fontWeight="600" textAlign="center">
                        {i18n.t(ROTATING_PROMPTS[promptIndex].titleKey)}
                      </Heading>
                      <Text color="$onGlassSecondary" fontSize={14} textAlign="center">
                        {i18n.t(ROTATING_PROMPTS[promptIndex].subtitleKey)}
                      </Text>
                    </Reanimated.View>
                  )}
                  <ThemedButton
                    variant="solid"
                    onPress={() => {
                      haptic.medium()
                      connect()
                    }}
                    icon={<Mic size={20} color="#fff" />}
                    glow
                  >
                    {needsOnboarding
                      ? (i18n.t('start_journey') || 'Start Your Journey')
                      : (i18n.t('session_talk_to_rumi') || 'Talk to Rumi')}
                  </ThemedButton>
                </YStack>
              </GlassPanel>
            </Reanimated.View>
          )}
        </>
      ) : (
        !showingResultsPanel && (
          <ActiveSessionCard
            status={status}
            inputVolume={inputVolume}
            onStop={disconnect}
          />
        )
      )}
      {showingResultsPanel &&
        <YStack alignItems='center' justifyContent='center' marginTop='$8' width='100%'>
          {showWheel && <WheelPanel data={wheelData} />}
          {showEisenhower && eisenhowerData && <EisenhowerPanel data={eisenhowerData} />}
          {showActionPlan && actionPlanData && <ActionPlanPanel data={actionPlanData} />}
          {showSessionTasks && sessionTasksData && <SessionTasksPanel tasks={sessionTasksData} />}
          {showSessionValues && sessionValuesData && <SessionValuesPanel values={sessionValuesData} />}
          {showSessionSummary && sessionSummaryData && <SessionSummaryPanel data={sessionSummaryData} />}
          {showOnboardingData && userWSData && <OnboardingDataPanel data={userWSData} />}
        </YStack>
      }

      {
        lastActiveSessionId && (
          <SessionFeedbackModal
            visible={showFeedback}
            onClose={dismissFeedback}
            sessionId={lastActiveSessionId ?? ''}
          />
        )
      }
    </ContentLayout >
  )
}

export default function SessionScreenWrapper() {
  return (
    <TabScreenWrapper>
      <SessionScreen />
    </TabScreenWrapper>
  )
}
