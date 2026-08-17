import { useEffect, useCallback, useState } from 'react'
import { View, TouchableOpacity, StyleSheet, Pressable } from 'react-native'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { isWeb } from '@/adapters/platform'
import { WEB_MAX_WIDTH, WEB_HORIZONTAL_PADDING } from '@/utils/constants'
import { Text } from '@tamagui/core'
import { router, useSegments } from 'expo-router'
import { Mic, MicOff, Palette, CircleDot, Settings, Volume2, VolumeX, X, PhoneOff } from 'lucide-react-native'
import { useSettings } from '@/hooks/useSettings'
import { useAudio } from '@/hooks/useAudio'
import { useSession } from '@/hooks/useSession'
import i18n from '@/i18n'
import { ThemedIconButton, ThemedButton } from '@/components/atoms'
import { LanguageSwitcher } from '@/components/molecules'
import { GLASS, INK } from '@/styles/glass'
import { BlurView } from 'expo-blur'
import { useBlurTarget } from '@/context/BlurContext'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withTiming,
  interpolate,
  Extrapolation,
} from 'react-native-reanimated'

interface TopControlsProps {
  hideSettingsButton?: boolean
  showLanguageSwitcher?: boolean
  onOpenThemeSelector?: () => void
  onOpenSettings?: () => void
}

export function TopControls({
  hideSettingsButton = false,
  showLanguageSwitcher = false,
  onOpenThemeSelector,
  onOpenSettings,
}: TopControlsProps) {
  const handleOpenSettings = useCallback(() => {
    if (onOpenSettings) {
      onOpenSettings()
    } else {
      router.push('/(settings)')
    }
  }, [onOpenSettings])

  const handleOpenTheme = useCallback(() => {
    if (onOpenThemeSelector) {
      onOpenThemeSelector()
    } else {
      router.push('/(settings)/app/theme')
    }
  }, [onOpenThemeSelector])
  const blurTargetRef = useBlurTarget()
  const insets = useSafeAreaInsets()
  const segments = useSegments()
  const { colorScheme } = useSettings()

  const isProfilePage = segments.includes('profile')
  const isAuthRoute = segments.includes('(auth)')
  const { isMusicEnabled, toggleMusic } = useAudio()
  const { status, disconnect, endSession, isMuted, toggleMute, durationSeconds, formatDuration } = useSession()

  const isConnected = status !== 'disconnected'
  const isEnding = status === 'ending'
  const [barFading, setBarFading] = useState(false)
  const showConnected = isConnected && !barFading
  const animValue = useSharedValue(isConnected ? 1 : 0)

  useEffect(() => {
    if (isEnding) {
      const timer = setTimeout(() => setBarFading(true), 500)
      return () => clearTimeout(timer)
    } else {
      setBarFading(false)
    }
  }, [isEnding])

  useEffect(() => {
    animValue.value = withTiming(showConnected ? 1 : 0, { duration: 250 })
  }, [showConnected])

  const disconnectedStyle = useAnimatedStyle(() => ({
    opacity: interpolate(animValue.value, [0, 1], [1, 0], Extrapolation.CLAMP),
    transform: [{ translateY: interpolate(animValue.value, [0, 1], [0, -30], Extrapolation.CLAMP) }],
    pointerEvents: showConnected || isEnding ? ('none' as const) : ('auto' as const),
  }))

  const connectedStyle = useAnimatedStyle(() => ({
    opacity: interpolate(animValue.value, [0, 1], [0, 1], Extrapolation.CLAMP),
    transform: [{ translateY: interpolate(animValue.value, [0, 1], [30, 0], Extrapolation.CLAMP) }],
    pointerEvents: showConnected ? ('auto' as const) : ('none' as const),
  }))

  return (
    <>
      <View style={[styles.wrapper, isWeb && styles.webWrapper, { pointerEvents: 'box-none' }]}>

        {/* 1. DISCONNECTED BAR */}
        <Reanimated.View
          style={[
            styles.bar,
            styles.disconnectedBar,
            isWeb && styles.webBar,
            {
              top: isWeb ? 24 : insets.top,
            },
            disconnectedStyle,
          ]}
        >
          <View style={styles.leftControls}>
            {showLanguageSwitcher && <LanguageSwitcher variant="minimal" color="#fff" align="left" />}
            {/* <ThemedIconButton
              onPress={() => isProfilePage ? handleOpenSettings() : handleOpenTheme()}
              accessibilityLabel={isProfilePage
                ? (i18n.t('a11y_open_settings') || 'Open settings')
                : (i18n.t('a11y_change_theme') || 'Change background theme')}
            >
              {isProfilePage ? <Settings size={20} color={INK.primary} /> : <Palette size={20} color={INK.primary} />}
            </ThemedIconButton> */}
            {!hideSettingsButton &&
              <ThemedIconButton
                onPress={() => handleOpenSettings()}
                accessibilityLabel={i18n.t('a11y_open_settings') || 'Open settings'}
              >
                <Settings size={20} color={INK.primary} />
              </ThemedIconButton>
            }
          </View>
          <View style={styles.rightControls}>
            <ThemedIconButton
              onPress={() => handleOpenTheme()}
              accessibilityLabel={i18n.t('a11y_change_theme') || 'Change background theme'}
            >
              <Palette size={20} color={INK.primary} />
            </ThemedIconButton>
            {!isAuthRoute && (
              <ThemedIconButton
                onPress={toggleMusic}
                accessibilityLabel={isMusicEnabled
                  ? (i18n.t('a11y_music_off') || 'Turn ambient sound off')
                  : (i18n.t('a11y_music_on') || 'Turn ambient sound on')}
                accessibilityState={{ checked: isMusicEnabled }}
              >
                {isMusicEnabled ? <Volume2 size={20} color={INK.primary} /> : <VolumeX size={20} color={INK.primary} />}
              </ThemedIconButton>
            )}
          </View>
        </Reanimated.View>

        {/* 2. CONNECTED BAR */}
        <Reanimated.View
          style={[
            styles.bar,
            styles.connectedBar,
            isWeb && styles.webBar,
            {
              top: isWeb ? 24 : insets.top,
            },
            connectedStyle,
          ]}
        >
          <BlurView
            blurTarget={blurTargetRef}
            style={{
              borderRadius: GLASS.radius.pill,
              overflow: 'hidden',
              borderWidth: GLASS.borderWidth,
              borderColor: GLASS.borderColor,
              flexDirection: 'row',
              justifyContent: 'space-between',
              alignItems: 'center',
              paddingHorizontal: 16,
              paddingVertical: 8,
              width: '100%',
            }}
            tint={GLASS.tint}
            intensity={GLASS.intensity}
          >
            <View style={[StyleSheet.absoluteFill, { pointerEvents: 'none', backgroundColor: GLASS.baseFill }]} />
            <View style={styles.sessionBarLeft}>
              <Pressable onPress={toggleMute} style={{ borderRadius: 99, overflow: 'hidden', backgroundColor: isMuted ? '#FEE2E2' : '#fff', justifyContent: 'center', alignItems: 'center', width: 32, height: 32 }} >
                {isMuted ? <MicOff size={20} color='#EF4444' /> : <Mic size={20} color={INK.primary} />}
              </Pressable>
              <Text style={[styles.miniTimer, { color: INK.primary }]}>
                {formatDuration(durationSeconds)}
              </Text>
            </View>
            <ThemedButton
              variant='error'
              onPress={endSession}
              icon={<PhoneOff size={18} />}

            >
              <Text style={[styles.endBtnText]}>{i18n.t('end_session') || 'End Session'}</Text>
            </ThemedButton>
          </BlurView>
        </Reanimated.View>
      </View>
    </>
  )
}

const styles = StyleSheet.create({
  wrapper: {
    position: 'absolute',
    top: 0,
    left: 0,
    right: 0,
    zIndex: 50,
    paddingTop: 56,
  },
  webWrapper: {
    paddingTop: 24,
  },
  bar: {
    position: 'absolute',
    width: '100%',
    display: 'flex',
    flexDirection: 'row',
    borderRadius: 9999,
    paddingHorizontal: 16,
    paddingVertical: 10,
    justifyContent: 'space-between',
  },
  webBar: {
    maxWidth: WEB_MAX_WIDTH,
    paddingHorizontal: WEB_HORIZONTAL_PADDING,
    alignSelf: 'center',
  },
  disconnectedBar: {
    justifyContent: 'space-between',
  },
  connectedBar: {
    alignItems: 'center',
  },
  leftControls: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  rightControls: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  sessionBarLeft: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  miniTimer: {
    fontSize: 16,
    fontWeight: '600',
    fontVariant: ['tabular-nums'],
  },
  miniEndBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 9999,
  },
  endBtnText: {
    fontWeight: '600',
  },
})
