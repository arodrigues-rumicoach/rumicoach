import { useRef, useCallback, useState, useEffect } from 'react'
import { View, Animated, Pressable, TouchableOpacity, StyleSheet, Dimensions, Platform, ScrollView, ActivityIndicator } from 'react-native'
import { Text } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { GestureDetector, Gesture, GestureHandlerRootView } from 'react-native-gesture-handler'
import { Volume1, Volume2, VolumeX, Play } from 'lucide-react-native'
import { router } from 'expo-router'
import { useSettings } from '../../../src/hooks/useSettings'
import { useAudio } from '../../../src/hooks/useAudio'
import { THEME_ASSETS, getThemeImageSource } from '../../../src/utils/theme'
import i18n from '../../../src/i18n'
import type { ThemeType } from '../../../src/context/SettingsContext'
import { LazyImage, GlassCard } from '@/components/atoms'
import { useBlurTarget } from '@/context/BlurContext'
import { GLASS, INK } from '@/styles/glass'
import { VideoView, useVideoPlayer } from 'expo-video'
import { useThemeAssetUri } from '../../../src/hooks/useThemeAssetUri'
import { isWeb } from '../../../src/adapters/platform'

const THEMES: ThemeType[] = ['lavender', 'fireplace', 'mountain_lake', 'rain', 'sunset_beach', 'waterfall']
const { width: SCREEN_WIDTH } = Dimensions.get('window')
const CARD_GAP = 12
const CARD_SIZE = Platform.OS === 'web' ? 300 : (SCREEN_WIDTH - 48) / 2

function SectionLabel({ children }: { children: string }) {
  return (
    <Text fontSize={12} fontWeight="700" letterSpacing={0.5} color="$onGlassSecondary" textTransform="uppercase">
      {children}
    </Text>
  )
}

export default function SettingsThemeScreen() {
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const { theme, setTheme, colorScheme } = useSettings()
  const { isMusicEnabled, volume, setVolume, setMusicEnabled, fadeOut, fadeIn } = useAudio()
  const thumbScale = useRef(new Animated.Value(1)).current

  const trackWidth = useRef(0)
  const volumeRef = useRef(volume)
  volumeRef.current = volume
  const isMusicEnabledRef = useRef(isMusicEnabled)
  isMusicEnabledRef.current = isMusicEnabled
  const gestureStartVolume = useRef(volume)

  const applyVolume = useCallback((newVol: number) => {
    const clamped = Math.max(0, Math.min(1, Math.round(newVol * 100) / 100))
    setVolume(clamped)
    if (clamped > 0 && !isMusicEnabledRef.current) setMusicEnabled(true)
    else if (clamped === 0) setMusicEnabled(false)
  }, [setVolume, setMusicEnabled])

  const panGesture = Gesture.Pan()
    .runOnJS(true)
    .onBegin((e) => {
      gestureStartVolume.current = trackWidth.current > 0
        ? Math.max(0, Math.min(1, e.x / trackWidth.current))
        : volumeRef.current
      Animated.spring(thumbScale, { toValue: 1.35, useNativeDriver: Platform.OS !== 'web', friction: 8 }).start()
      applyVolume(gestureStartVolume.current)
    })
    .onUpdate((e) => {
      const delta = trackWidth.current > 0 ? e.translationX / trackWidth.current : 0
      applyVolume(gestureStartVolume.current + delta)
    })
    .onEnd(() => {
      Animated.spring(thumbScale, { toValue: 1, useNativeDriver: Platform.OS !== 'web', friction: 5 }).start()
    })
    .onFinalize(() => {
      Animated.spring(thumbScale, { toValue: 1, useNativeDriver: Platform.OS !== 'web', friction: 5 }).start()
    })

  const displayVolume = isMusicEnabled ? volume : 0
  const fillPercent = displayVolume * 100
  const VolumeIcon = !isMusicEnabled || volume === 0 ? VolumeX : volume < 0.5 ? Volume1 : Volume2

  const [savingTheme, setSavingTheme] = useState<ThemeType | null>(null)

  const handleSelectTheme = useCallback(async (themeKey: ThemeType) => {
    if (savingTheme) return
    setSavingTheme(themeKey)
    try {
      await setTheme(themeKey)
    } finally {
      router.back()
    }
  }, [savingTheme, setTheme])

  // ── Theme preview (long-press) ────────────────────────────────────────
  const [previewTheme, setPreviewTheme] = useState<ThemeType | null>(null)
  const [pressedTheme, setPressedTheme] = useState<ThemeType | null>(null)
  const isLongPressRef = useRef(false)
  const audioRef = useRef<any>(null)
  const previewAssets = useThemeAssetUri(previewTheme ?? theme)

  // Video player — created once, source swapped via effect
  const videoPlayer = useVideoPlayer(null, (player) => {
    player.loop = true
    player.volume = 0
  })

  // Swap video source when preview changes
  useEffect(() => {
    if (!previewAssets.videoUri) return
    let cancelled = false
    videoPlayer.replaceAsync(previewAssets.videoUri).then(() => {
      if (!cancelled) videoPlayer.play()
    }).catch(() => {})
    return () => { cancelled = true }
  }, [previewAssets.videoUri, videoPlayer])

  // Stop video when preview ends
  useEffect(() => {
    if (!previewTheme) {
      videoPlayer.pause()
    }
  }, [previewTheme, videoPlayer])

  // Preview audio — dynamic import of expo-audio
  useEffect(() => {
    if (!previewTheme || !previewAssets.audioUri) return

    const startPreview = async () => {
      // Pause global ambient audio
      fadeOut(200)

      if (isWeb) {
        const audio = new Audio(previewAssets.audioUri!)
        audio.loop = true
        audioRef.current = audio
        await audio.play()
      } else {
        try {
          const { createAudioPlayer, setAudioModeAsync } = await import('expo-audio')
          await setAudioModeAsync({ playsInSilentMode: true })
          const player = createAudioPlayer({ uri: previewAssets.audioUri! })
          player.volume = 1
          audioRef.current = player
          player.play()
        } catch {
          if (__DEV__) console.warn('[THEME_PREVIEW] audio failed')
        }
      }
    }

    startPreview()

    return () => {
      if (audioRef.current) {
        try { audioRef.current.pause() } catch {}
        if (!isWeb) {
          try { audioRef.current.remove?.() } catch {}
        }
        audioRef.current = null
      }
      // Resume global ambient audio
      fadeIn(300)
    }
  }, [previewTheme, previewAssets.audioUri, fadeOut, fadeIn])

  const handlePreviewStart = useCallback((themeKey: ThemeType) => {
    isLongPressRef.current = true
    setPreviewTheme(themeKey)
  }, [])

  const handlePreviewEnd = useCallback(() => {
    setPreviewTheme(null)
  }, [])

  return (
    <GestureHandlerRootView style={styles.container}>
      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 32 }]}
      >
        <GlassCard variant="light" borderRadius={18} padding={16} gap={14} blurTarget={blurTargetRef}>
          <SectionLabel>{i18n.t('volume') || 'Volume'}</SectionLabel>

          <View style={styles.volumeRow}>
            <TouchableOpacity
              onPress={() => isMusicEnabled ? setMusicEnabled(false) : setMusicEnabled(true)}
              style={styles.volumeIconBtn}
              hitSlop={{ top: 10, bottom: 10, left: 10, right: 10 }}
            >
              <VolumeIcon size={20} color={INK.primary} />
            </TouchableOpacity>

            <GestureDetector gesture={panGesture}>
              <View
                style={styles.sliderTrack}
                onLayout={(e) => { trackWidth.current = e.nativeEvent.layout.width }}
              >
                <View style={styles.sliderRail} />
                <View
                  style={[
                    styles.sliderFill,
                    { width: `${fillPercent}%` as any, backgroundColor: colorScheme.primary },
                  ]}
                />
                <Animated.View
                  style={[
                    styles.sliderThumb,
                    {
                      left: `${fillPercent}%` as any,
                      transform: [{ translateX: -12 }, { scale: thumbScale }],
                      backgroundColor: '#fff',
                    },
                  ]}
                />
              </View>
            </GestureDetector>

            <TouchableOpacity
              onPress={() => { setMusicEnabled(true); setVolume(1) }}
              style={styles.volumeIconBtn}
              hitSlop={{ top: 10, bottom: 10, left: 10, right: 10 }}
            >
              <Volume2 size={20} color={INK.primary} />
            </TouchableOpacity>
          </View>
        </GlassCard>

        <View style={{ height: 16 }} />

        <View style={styles.grid}>
          {THEMES.map((themeKey) => {
            const assets = THEME_ASSETS[themeKey]
            const isActive = theme === themeKey
            const isPreviewing = previewTheme === themeKey
            const isPressed = pressedTheme === themeKey
            const label = i18n.t(`theme_${themeKey}`) || {
              lavender: 'Lavender',
              fireplace: 'Fireplace',
              mountain_lake: 'Mountain Lake',
              rain: 'Rain',
              sunset_beach: 'Sunset Beach',
              waterfall: 'The Waterfall',
            }[themeKey]
            return (
              <Pressable
                key={themeKey}
                style={({ pressed }) => [
                  styles.card,
                  isActive && { borderColor: colorScheme.secondary, borderWidth: 2 },
                  savingTheme && savingTheme !== themeKey && styles.cardDimmed,
                  isPreviewing && styles.cardPreviewing,
                  (pressed || isPressed) && { opacity: 0.8 },
                ]}
                onPressIn={() => {
                  setPressedTheme(themeKey)
                }}
                onPressOut={() => {
                  setPressedTheme(null)
                  handlePreviewEnd()
                }}
                onLongPress={() => handlePreviewStart(themeKey)}
                delayLongPress={400}
                onPress={() => {
                  if (isLongPressRef.current) {
                    isLongPressRef.current = false
                    return
                  }
                  handleSelectTheme(themeKey)
                }}
                disabled={savingTheme !== null}
              >
                <LazyImage source={getThemeImageSource(themeKey)} style={styles.cardImage} resizeMode="cover" />

                {/* Video preview overlay */}
                {isPreviewing && (
                  <VideoView
                    player={videoPlayer}
                    style={styles.videoOverlay}
                    contentFit="cover"
                    nativeControls={false}
                    pointerEvents="none"
                  />
                )}

                {savingTheme === themeKey && (
                  <View style={styles.cardSpinner}>
                    <ActivityIndicator color={assets.colors.secondary} />
                  </View>
                )}

                {/* Long-press hint */}
                {!isPreviewing && !savingTheme && (
                  <View style={styles.previewHint} pointerEvents="none">
                    <Play size={12} color="#fff" />
                  </View>
                )}

                {/* Preview indicator */}
                {isPreviewing && (
                  <View style={styles.previewIndicator} pointerEvents="none">
                    <View style={styles.previewPulse} />
                  </View>
                )}

                <View style={styles.cardLabelWrapper}>
                  <Text style={[styles.cardLabel, { color: INK.primary }]}>{label}</Text>
                </View>
              </Pressable>
            )
          })}
        </View>
      </ScrollView>
    </GestureHandlerRootView>
  )
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 100,
  },
  volumeRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  volumeIconBtn: {
    width: 36,
    height: 36,
    justifyContent: 'center',
    alignItems: 'center',
    borderRadius: 18,
    backgroundColor: GLASS.pressedFill,
  },
  sliderTrack: {
    flex: 1,
    height: 44,
    justifyContent: 'center',
  },
  sliderRail: {
    position: 'absolute',
    left: 0,
    right: 0,
    height: 4,
    borderRadius: 2,
    backgroundColor: GLASS.separator,
  },
  sliderFill: {
    position: 'absolute',
    left: 0,
    height: 4,
    borderRadius: 2,
  },
  sliderThumb: {
    position: 'absolute',
    width: 24,
    height: 24,
    borderRadius: 12,
    top: '50%',
    marginTop: -12,
    borderWidth: 2,
    borderColor: GLASS.borderColor,
    elevation: 3,
  },
  grid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    justifyContent: 'center',
    gap: CARD_GAP,
  },
  card: {
    width: CARD_SIZE,
    height: CARD_SIZE * 0.75,
    borderRadius: GLASS.radius.card,
    justifyContent: 'flex-end',
    overflow: 'hidden',
    borderWidth: 1,
    borderColor: GLASS.borderColor,
    backgroundColor: GLASS.baseFill,
  },
  cardDimmed: {
    opacity: 0.5,
  },
  cardPreviewing: {
    borderColor: '#fff',
    borderWidth: 2,
  },
  cardSpinner: {
    ...StyleSheet.absoluteFill,
    alignItems: 'center',
    justifyContent: 'center',
  },
  cardImage: {
    ...StyleSheet.absoluteFill,
    borderRadius: GLASS.radius.card,
    width: '100%',
    height: '100%',
  },
  videoOverlay: {
    ...StyleSheet.absoluteFill,
    borderRadius: GLASS.radius.card,
  },
  previewHint: {
    position: 'absolute',
    top: 8,
    right: 8,
    width: 24,
    height: 24,
    borderRadius: 12,
    backgroundColor: 'rgba(0,0,0,0.4)',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 2,
  },
  previewIndicator: {
    position: 'absolute',
    top: 8,
    right: 8,
    width: 24,
    height: 24,
    borderRadius: 12,
    backgroundColor: 'rgba(255,255,255,0.9)',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 2,
  },
  previewPulse: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: '#ef4444',
  },
  cardLabelWrapper: {
    position: 'relative',
    zIndex: 1,
    padding: 10,
    backgroundColor: 'rgba(255,255,255,0.75)',
    borderTopWidth: 1,
    borderTopColor: GLASS.separator,
  },
  cardLabel: {
    fontSize: 13,
    fontWeight: '600',
  },
})
