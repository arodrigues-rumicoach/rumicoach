import { createContext, useRef, useState, useCallback, useEffect, useMemo, type ReactNode } from 'react'
import { Platform } from 'react-native'

import { useAuth } from '../hooks/useAuth'
import { useAudio } from '../hooks/useAudio'
import { regionWebSocketUrl } from '../api/backend-url'
import { getRegionFromToken } from '../api/jwt'
import { isWeb } from '../adapters/platform'
import { getSessionAudioAdapter } from '../adapters/session-audio'
import { useWebSocket, configureAudioMode } from '../hooks/useWebSocket'
import { useSessionPanels } from '../hooks/useSessionPanels'
import type { MicrophoneDataEvent, VolumeLevelEvent } from '@speechmatics/expo-two-way-audio'
import { trackSessionStarted, trackSessionEnded } from '@/analytics'

export type ChatStatus = 'disconnected' | 'connecting' | 'preparing' | 'listening' | 'thinking' | 'speaking' | 'retrying' | 'ending'

/**
 * The close code the server hangs up with when it will not grant a session — out of
 * minutes, and the free introductory ones already spent. 4402 is in the WebSocket
 * private-use range and echoes the HTTP 402 this refusal used to be, back when it was
 * sent as a failed handshake that no WebSocket client could read.
 *
 * Must match wsCloseInsufficientBalance in the backend's api/routes/chat.go.
 */
const WS_CLOSE_INSUFFICIENT_BALANCE = 4402

export interface ActionPlanItem {
  id?: string
  title: string
  type: 'one_time' | 'recurring'
  days?: number[]
}

export interface ActionPlanData {
  area?: string
  actions: ActionPlanItem[]
}

export interface SessionSummaryData {
  session_id?: string
  session_type?: string
  generated_at?: string
  vision?: string
  priority_area?: {
    name: string
    goal?: string
    score?: number
    max_score?: number
    reasoning?: string
  }
  /** Only the Values session sends this: the user's top values, in their own
   *  words and their own language. Absent on every other session type. */
  values?: string[]
  /** Only the Identity session sends this: the Identity Reflection Card, whose
   *  shape mirrors the printed one (see Filipa's PDF). Absent on every other
   *  session type — and absent on an Identity session that did not get far
   *  enough to build it, which falls back to the standard card. `key_insight`
   *  doubles as that card's KEY INSIGHT line. */
  identity_reflection?: {
    learned_identity: string
    what_it_gave?: string
    what_it_costs?: string
    who_becoming: string
    qualities: string[]
    evidence?: string
  }
  /** Only the Acceptance session sends this: the Acceptance Reflection Card,
   *  whose shape mirrors the printed one (see Filipa's PDF) — the expectation,
   *  the reality, the control split, and the two choices. Absent on every other
   *  session type, and absent on an Acceptance session that did not get far
   *  enough to build it, which falls back to the standard card. `key_insight`
   *  doubles as that card's KEY INSIGHT line. */
  acceptance_reflection?: {
    expected: string
    reality: string
    cannot_control?: string
    can_influence?: string
    choose_to_accept?: string
    where_i_act?: string
    next_step?: string
  }
  /** Only the Movement session sends this: the blocker named during the session
   *  ("what has been blocking me" on Filipa's card spec). */
  main_obstacle?: string
  key_insight?: string
  commitments?: {
    title: string
    type?: string
    date?: string
  }[]
  behavior_plan?: {
    behavior: string
    identity?: string
  }
  next_session?: {
    session_type?: string
    question_key?: string
  }
}

export interface SessionTaskItem {
  id?: string
  title: string
  type?: string
  date?: string
  done?: boolean
}

export interface SessionContextValue {
  status: ChatStatus
  isConnected: boolean
  connect: (isRetry?: boolean, sessionType?: string) => Promise<void>
  disconnect: () => void
  endSession: () => void
  isMuted: boolean
  setIsMuted: (muted: boolean) => void
  toggleMute: () => void
  durationSeconds: number
  formatDuration: (seconds: number) => string
  volume: number
  inputVolume: number
  wheelData: never[]
  showWheel: boolean
  eisenhowerData: Record<string, unknown> | null
  showEisenhower: boolean
  actionPlanData: ActionPlanData | null
  showActionPlan: boolean
  sessionSummaryData: SessionSummaryData | null
  showSessionSummary: boolean
  sessionTasksData: SessionTaskItem[] | null
  showSessionTasks: boolean
  sessionValuesData: string[] | null
  showSessionValues: boolean
  tasksVersion: number
  userWSData: Partial<import('../api').User> | null
  showOnboardingData: boolean
  sessionId: string | null
  // The SERVER-resolved type of the session being held (e.g. 'session_energy',
  // 'checkin') — the conversation screen labels itself with it so the user keeps
  // their "mental GPS" (QA). Updates mid-session when a gateway hands over to the
  // planned session. Null until the server announces it.
  sessionType: string | null
  pendingNavigation: string | null
  clearPendingNavigation: () => void
}

export const SessionContext = createContext<SessionContextValue | null>(null)

const formatDuration = (seconds: number): string => {
  const mins = Math.floor(seconds / 60)
  const secs = seconds % 60
  return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const { token, ensureValidToken, updateUser, user, refreshUser } = useAuth()
  const { isMusicEnabled, fadeOut, fadeIn, setMusicEnabled, pauseAmbient } = useAudio()
  const fadeInRef = useRef(fadeIn)
  useEffect(() => { fadeInRef.current = fadeIn }, [fadeIn])

  const [status, setStatus] = useState<ChatStatus>('disconnected')
  const [isMuted, setIsMuted] = useState(false)
  const [durationSeconds, setDurationSeconds] = useState(0)
  const [volume, setVolume] = useState(0)
  const [inputVolume, setInputVolume] = useState(0)
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [sessionType, setSessionType] = useState<string | null>(null)
  const [pendingNavigation, setPendingNavigation] = useState<string | null>(null)

  const wsRef = useRef<WebSocket | null>(null)
  const statusRef = useRef<ChatStatus>('disconnected')
  const isMutedRef = useRef(false)
  const durationIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const wasMusicEnabledRef = useRef(false)
  const playBackTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const playbackEndTimestampRef = useRef(0)
  const tearDownTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const endSessionTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Screen the backend asked us to land on once the session closes (currently the Journey
  // screen at the end of the intro). Navigating the moment it arrives would pull the user
  // off the session screen while the goodbye is still playing, so it is held until the
  // same playback-aware teardown that owns the close.
  const navigateOnCloseRef = useRef<string | null>(null)
  // Set when the server refuses this attempt for balance — by the error frame, the
  // close code, or both. A ref rather than state because the close handler reads it
  // during teardown, before any re-render could deliver it.
  const refusedRef = useRef(false)

  const panels = useSessionPanels()
  const panelsRef = useRef(panels)
  panelsRef.current = panels

  // Held in a ref so the teardown below can refresh the profile without taking
  // useAuth's identity into its dependencies and re-running on every auth render.
  const refreshUserRef = useRef(refreshUser)
  refreshUserRef.current = refreshUser

  useEffect(() => { statusRef.current = status }, [status])
  useEffect(() => { isMutedRef.current = isMuted }, [isMuted])
  useEffect(() => { getSessionAudioAdapter().setMuted(isMuted) }, [isMuted])

  const handleWsMessage = useCallback(async (data: string | ArrayBuffer) => {
    if (data instanceof ArrayBuffer) {
      const pcm24k = new Int16Array(data)
      const outputLen = Math.floor(pcm24k.length * (16000 / 24000))
      const pcm16k = new Int16Array(outputLen)
      const ratio = 24000 / 16000
      for (let i = 0; i < outputLen; i++) {
        const inputIdx = i * ratio
        const idxFloor = Math.floor(inputIdx)
        const idxCeil = Math.min(idxFloor + 1, pcm24k.length - 1)
        const fraction = inputIdx - idxFloor
        pcm16k[i] = pcm24k[idxFloor] + (pcm24k[idxCeil] - pcm24k[idxFloor]) * fraction
      }
      getSessionAudioAdapter().playPCMData(new Uint8Array(pcm16k.buffer))
      setStatus('speaking')

      const samples = pcm16k.length
      const durationMs = (samples / 16000) * 1000
      const now = Date.now()
      if (playbackEndTimestampRef.current < now) {
        playbackEndTimestampRef.current = now
      }
      playbackEndTimestampRef.current += durationMs
      const timeRemaining = playbackEndTimestampRef.current - now

      if (playBackTimeoutRef.current) clearTimeout(playBackTimeoutRef.current)
      playBackTimeoutRef.current = setTimeout(() => {
        setStatus('listening')
      }, timeRemaining + 200)
      return
    }

    if (typeof data === 'string') {
      const msg = JSON.parse(data)
      const p = panelsRef.current

      switch (msg.type) {
        case 'interrupt':
          if (playBackTimeoutRef.current) {
            clearTimeout(playBackTimeoutRef.current)
            playBackTimeoutRef.current = null
          }
          playbackEndTimestampRef.current = 0
          getSessionAudioAdapter().stopPlayback()
          setStatus('listening')
          break

        case 'wheel_of_life_update':
          if (Array.isArray(msg.data?.categories)) p.handleWheelUpdate(msg.data.categories)
          break

        case 'eisenhower_matrix_update':
          p.handleEisenhowerUpdate(msg.data)
          break

        case 'action_plan_update':
          if (msg.data) p.handleActionPlanUpdate(msg.data)
          break

        case 'session_summary':
          if (msg.data) p.handleSessionSummaryUpdate(msg.data)
          break

        case 'setup_complete':
          p.hideAllPanels()
          break

        case 'show_screen':
          if (msg.data?.screen === 'memories') {
            setPendingNavigation('memories')
            p.hideAllPanels()
          } else if (msg.data?.screen === 'wheel_of_life') {
            setPendingNavigation('wheel_of_life')
            p.hideAllPanels()
          } else if (msg.data?.screen === 'session') {
            setPendingNavigation('session')
            p.hideAllPanels()
          } else if (msg.data?.screen === 'tasks') {
            setPendingNavigation('tasks')
            p.hideAllPanels()
          } else if (msg.data?.screen === 'profile') {
            setPendingNavigation('profile')
            p.hideAllPanels()
          } else if (msg.data?.screen === 'journey' || msg.data?.screen === 'growth') {
            // 'growth' is the backend's internal id for this tab (the intro's decline
            // branch sends show_screen{screen:'growth', at:'session_end'}) — without the
            // alias that landing was silently dropped and the user stayed parked on the
            // session tab. The intro tour opens this screen as it is spoken; the same
            // message with at === 'session_end' is instead the landing place for when
            // the session closes, and must not pull the user away while the goodbye
            // still plays.
            if (msg.data?.at === 'session_end') {
              navigateOnCloseRef.current = 'journey'
            } else {
              setPendingNavigation('journey')
              p.hideAllPanels()
            }
          }
          break

        case 'onboarding':
          if (msg.data) {
            setPendingNavigation('onboarding')
            p.handleOnboardingUpdate(msg.data)
            updateUser({
              dateOfBirth: msg.data.dateOfBirth,
              gender: msg.data.gender,
              country: msg.data.country,
            }).catch(() => {})
          }
          break

        case 'session_tasks_update':
          if (Array.isArray(msg.data?.tasks)) p.handleSessionTasksUpdate(msg.data.tasks)
          break

        case 'session_values_update':
          if (Array.isArray(msg.data?.values)) p.handleSessionValuesUpdate(msg.data.values)
          break

        case 'tasks_updated':
          p.handleTasksUpdated()
          break

        case 'session_created':
          if (msg.session_id) setSessionId(msg.session_id)
          if (typeof msg.session_type === 'string') setSessionType(msg.session_type)
          break

        case 'session_type_update':
          // A gateway session (intro, check-in) handed over to the planned session
          // mid-connection — relabel the screen with what is actually being held.
          if (typeof msg.data?.session_type === 'string') setSessionType(msg.data.session_type)
          break

        case 'error':
          // The server refuses a session it will not give — out of minutes, and the
          // introductory ones already used. It says so over the socket rather than by
          // failing the handshake, because an HTTP status on a failed upgrade is
          // invisible to a WebSocket client: this is the only form the refusal can
          // arrive in that we can actually read. The rule itself lives on the server
          // and only there; the app's job is to turn this into a paywall.
          // Only recorded here. The navigation happens in handleWsClose, after
          // disconnect() has run — disconnect clears pendingNavigation, so routing
          // from this frame (which arrives just before the close) would be undone a
          // moment later and the user would sit on a dead session screen.
          if (msg.code === 'INSUFFICIENT_BALANCE') {
            refusedRef.current = true
          } else if (__DEV__) {
            console.error('Session error from server:', msg.code, msg.message)
          }
          break

        case 'session_terminated':
          // The backend sends this while the goodbye audio is still buffered locally —
          // the contract is that WE own the close. Disconnecting now would cut the
          // farewell mid-sentence and pop the feedback modal over it (QA), so wait for
          // the remaining buffered playback (plus a small grace) before tearing down.
          {
            const closeSession = () => {
              disconnect()
              if (navigateOnCloseRef.current) {
                setPendingNavigation(navigateOnCloseRef.current)
                navigateOnCloseRef.current = null
              }
            }
            const remainingMs = Math.max(0, playbackEndTimestampRef.current - Date.now())
            if (remainingMs > 0) {
              setTimeout(closeSession, remainingMs + 700)
            } else {
              closeSession()
            }
          }
          break

        default:
          break
      }
    }
  }, [updateUser])

  const handleWsOpen = useCallback(() => {
    getSessionAudioAdapter().startRecording()
    if (Platform.OS === 'android') configureAudioMode()
    setStatus('listening')
  }, [])

  const handleWsClose = useCallback((event?: { code?: number }) => {
    // A refusal arrives as an error frame and then a close carrying the same reason in
    // its code, so a socket torn down before the frame was read still says why. Either
    // signal is enough.
    const refused = refusedRef.current || event?.code === WS_CLOSE_INSUFFICIENT_BALANCE

    disconnect()

    // After disconnect, which clears pendingNavigation as part of resetting the
    // session. The paywall is where this attempt was heading all along, so it is set
    // once the teardown that would have wiped it is done.
    if (refused) {
      refusedRef.current = true
      setPendingNavigation('paywall')
    }
  }, [])

  const handleWsError = useCallback((event: Event) => {
    if (__DEV__) console.error('WebSocket error', event.type)
  }, [])

  const handleSetInputVolume = useCallback((level: number) => {
    setInputVolume(level)
    if (level > 0.08 && statusRef.current === 'speaking') {
      setStatus('listening')
    }
  }, [])

  const { audioOptions, setupNativeListeners, cleanupWs } = useWebSocket({
    wsRef,
    onMessage: handleWsMessage,
    onOpen: handleWsOpen,
    onClose: handleWsClose,
    onError: handleWsError,
    setInputVolume: handleSetInputVolume,
    setVolume,
    statusRef,
  })

  useEffect(() => setupNativeListeners(), [setupNativeListeners])

  const isConnected = status !== 'disconnected'

  // Analytics needs these at disconnect time, and disconnect() is a useCallback
  // that would close over stale state. Refs keep the last known values without
  // adding re-render churn to a screen that already ticks every second.
  const analyticsTypeRef = useRef<string | undefined>(undefined)
  const analyticsDurationRef = useRef(0)
  const analyticsGracefulRef = useRef(false)
  const analyticsStartedRef = useRef(false)
  useEffect(() => { analyticsDurationRef.current = durationSeconds }, [durationSeconds])

  useEffect(() => {
    if (isConnected) {
      durationIntervalRef.current = setInterval(() => setDurationSeconds(prev => prev + 1), 1000)
    } else {
      if (durationIntervalRef.current) clearInterval(durationIntervalRef.current)
      durationIntervalRef.current = null
      setDurationSeconds(0)
    }
    return () => { if (durationIntervalRef.current) clearInterval(durationIntervalRef.current) }
  }, [isConnected])

  const disconnect = useCallback(() => {
    if (endSessionTimeoutRef.current) {
      clearTimeout(endSessionTimeoutRef.current)
      endSessionTimeoutRef.current = null
    }

    if (tearDownTimeoutRef.current) {
      clearTimeout(tearDownTimeoutRef.current)
      tearDownTimeoutRef.current = null
    }

    if (playBackTimeoutRef.current) {
      clearTimeout(playBackTimeoutRef.current)
      playBackTimeoutRef.current = null
    }
    playbackEndTimestampRef.current = 0

    if (durationIntervalRef.current) {
      clearInterval(durationIntervalRef.current)
      durationIntervalRef.current = null
    }

    cleanupWs()

    // Report before the state below is cleared. `completed` separates someone who
    // let the conversation finish (endSession) from a drop — navigating away, a
    // dead socket, backgrounding. Both look the same afterwards, and the second
    // is the one worth chasing.
    if (analyticsStartedRef.current) {
      analyticsStartedRef.current = false
      trackSessionEnded(
        analyticsTypeRef.current,
        analyticsDurationRef.current,
        analyticsGracefulRef.current,
      )
      // The server debits the session's minutes as it closes, so from here the cached
      // profile's balance is wrong. The session-start path used to refetch /me and
      // incidentally kept this fresh; it no longer runs, and the balance the settings
      // screen shows would otherwise be a session behind. Failure is not worth
      // handling: the next screen that needs it refetches on its own.
      refreshUserRef.current().catch(() => {})
    }
    analyticsGracefulRef.current = false

    setStatus('disconnected')
    setDurationSeconds(0)
    panelsRef.current.handleDisconnect()
    setVolume(0)
    setSessionId(null)
    setSessionType(null)
    setPendingNavigation(null)

    getSessionAudioAdapter().teardown()

    tearDownTimeoutRef.current = setTimeout(() => {
      if (isWeb) {
        setMusicEnabled(wasMusicEnabledRef.current)
      } else if (wasMusicEnabledRef.current) {
        fadeInRef.current(2000)
      }
      tearDownTimeoutRef.current = null
    }, 0)
  }, [cleanupWs, setMusicEnabled])

  useEffect(() => () => disconnect(), [disconnect])

  const endSession = useCallback(() => {
    if (status === 'ending' || status === 'disconnected') return
    // The only path that counts as a finished conversation; disconnect() reads
    // this to decide `completed`.
    analyticsGracefulRef.current = true
    setStatus('ending')
    endSessionTimeoutRef.current = setTimeout(() => {
      endSessionTimeoutRef.current = null
      disconnect()
    }, 1000)
  }, [status, disconnect])

  const isMusicEnabledRef = useRef(isMusicEnabled)
  const tokenRef = useRef(token)
  useEffect(() => { isMusicEnabledRef.current = isMusicEnabled }, [isMusicEnabled])
  useEffect(() => { tokenRef.current = token }, [token])

  /**
   * There is no balance check here.
   *
   * There used to be: connect() refetched /me and ran the server's own rule against it,
   * because a refused handshake came back as an HTTP 402 that no WebSocket client can
   * read, and without the pre-check the user just watched the session fail to connect.
   * Keeping a second copy of a billing rule in the client is what that bought, and the
   * two copies drifted — the app offered sessions the socket then refused, and refused
   * sessions the socket would have granted, which is how users mid-onboarding ended up
   * looking at a paywall.
   *
   * The server now refuses over the socket instead (a typed error frame and close code
   * 4402), so the rule can live in one place. The app asks, and handles being told no:
   * see the 'error' case in handleWsMessage and WS_CLOSE_INSUFFICIENT_BALANCE.
   *
   * Which of the two paywalls to show turns on the RevenueCat entitlement, which this
   * provider deliberately does not reach for: importing the SDK here would pull the
   * whole purchases stack into the session path, which is native-only and heavy. The
   * navigation handler resolves 'paywall' into a route where that is already loaded.
   */
  const connect = useCallback(async (isRetry = false, sessionType?: string) => {
    if (endSessionTimeoutRef.current) {
      clearTimeout(endSessionTimeoutRef.current)
      endSessionTimeoutRef.current = null
    }

    if (tearDownTimeoutRef.current) {
      clearTimeout(tearDownTimeoutRef.current)
      tearDownTimeoutRef.current = null
    }

    if (!isRetry) {
      setDurationSeconds(0)
      setIsMuted(false)
      setSessionId(null)
      setSessionType(null)
      panelsRef.current.handleNewSession()
      wasMusicEnabledRef.current = isMusicEnabledRef.current
      // A retry is reconnecting a session that was already granted, so it keeps the
      // previous verdict; a fresh attempt asks again and must not inherit an old
      // refusal, or the paywall would be suppressed after a top-up.
      refusedRef.current = false
    }

    // A retry keeps the original type: connect() is called again without it.
    if (sessionType) analyticsTypeRef.current = sessionType
    analyticsStartedRef.current = true
    trackSessionStarted(analyticsTypeRef.current ?? 'unknown', isRetry)

    if (isMusicEnabledRef.current) {
      if (isWeb) {
        setMusicEnabled(false)
      } else if (Platform.OS === 'ios') {
        // iOS: expo-audio must be fully idle before the speechmatics engine takes
        // over the AVAudioSession — a fade's delayed pause() landing after engine
        // start can deactivate the session and kill the engine.
        pauseAmbient()
        // Yield so the native player finishes async teardown and releases the
        // AVAudioSession before we reconfigure it for recording.
        await new Promise<void>(r => setTimeout(r, 150))
      } else {
        fadeOut(2000)
      }
    }

    if (!isWeb) {
      const { getMicrophonePermissionsAsync, requestMicrophonePermissionsAsync } = require('@speechmatics/expo-two-way-audio')
      const permission = await getMicrophonePermissionsAsync()
      if (!permission.granted) {
        const result = await requestMicrophonePermissionsAsync()
        if (!result.granted) return
      }
    }

    setStatus(isRetry ? 'retrying' : 'connecting')

    if (!isWeb) {
      // Both platforms: tear down any previous engine first.
      getSessionAudioAdapter().teardown()

      // Configure expo-audio's AVAudioSession before the Speechmatics engine takes
      // ownership.  On Android this sets the recording mode the engine needs.  On iOS
      // it ensures expo-audio releases its playback-oriented session so the engine's
      // own .playAndRecord / .voiceChat category isn't blocked by a stale session.
      // (Calling this AFTER the engine would overwrite the engine's session — only
      // safe here, before initialize().)
      await configureAudioMode()
    }

    await getSessionAudioAdapter().initialize(audioOptions)

    const validToken = await ensureValidToken()
    const tokenToUse = validToken || tokenRef.current
    if (!tokenToUse) {
      setStatus('disconnected')
      return
    }

    const region = getRegionFromToken(tokenToUse)
    let wsUrl = regionWebSocketUrl(region)
    if (sessionType) wsUrl += `?session_type=${sessionType}`

    cleanupWs()

    const ws = new WebSocket(wsUrl, ['rumi-auth', tokenToUse])
    wsRef.current = ws
    ws.binaryType = 'arraybuffer'

    ws.onopen = () => {
      if (wsRef.current !== ws) return
      handleWsOpen()
    }

    ws.onmessage = async (event) => {
      if (wsRef.current !== ws) return
      try {
        await handleWsMessage(event.data)
      } catch (e) {
        if (__DEV__) console.error('ws.onmessage error:', e)
      }
    }

    ws.onclose = (event) => {
      if (wsRef.current !== ws) return
      handleWsClose(event)
    }

    ws.onerror = (event) => {
      handleWsError(event)
    }
  }, [ensureValidToken, disconnect, setMusicEnabled, fadeOut, pauseAmbient, audioOptions, cleanupWs, handleWsOpen, handleWsClose, handleWsError, handleWsMessage])

  const toggleMute = useCallback(() => setIsMuted(prev => !prev), [])

  const clearPendingNavigation = useCallback(() => setPendingNavigation(null), [])

  const value = useMemo<SessionContextValue>(() => ({
    status, isConnected, connect, disconnect, endSession,
    isMuted, setIsMuted, toggleMute,
    durationSeconds, formatDuration, volume, inputVolume,
    wheelData: panels.wheelData, showWheel: panels.showWheel,
    eisenhowerData: panels.eisenhowerData, showEisenhower: panels.showEisenhower,
    actionPlanData: panels.actionPlanData, showActionPlan: panels.showActionPlan,
    sessionSummaryData: panels.sessionSummaryData, showSessionSummary: panels.showSessionSummary,
    sessionTasksData: panels.sessionTasksData, showSessionTasks: panels.showSessionTasks,
    sessionValuesData: panels.sessionValuesData, showSessionValues: panels.showSessionValues,
    tasksVersion: panels.tasksVersion,
    userWSData: panels.userWSData, showOnboardingData: panels.showOnboardingData,
    sessionId, sessionType, pendingNavigation, clearPendingNavigation,
  }), [status, isConnected, connect, disconnect, endSession, isMuted, setIsMuted, toggleMute,
    durationSeconds, volume, inputVolume,
    panels.wheelData, panels.showWheel,
    panels.eisenhowerData, panels.showEisenhower,
    panels.actionPlanData, panels.showActionPlan,
    panels.sessionSummaryData, panels.showSessionSummary,
    panels.sessionTasksData, panels.showSessionTasks,
    panels.sessionValuesData, panels.showSessionValues,
    panels.tasksVersion, panels.userWSData, panels.showOnboardingData,
    sessionId, sessionType, pendingNavigation, clearPendingNavigation])

  return (
    <SessionContext.Provider value={value}>
      {children}
    </SessionContext.Provider>
  )
}
