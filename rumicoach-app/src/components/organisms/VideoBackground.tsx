import { type ReactNode, useEffect, useMemo } from 'react'
import { View, StyleSheet, AppState, Image, Platform } from 'react-native'
import { VideoView, useVideoPlayer } from 'expo-video'
import { useSettings } from '@/hooks/useSettings'
import { useSession } from '@/hooks/useSession'
import { useThemeAssetUri } from '@/hooks/useThemeAssetUri'
import { isIOS, isWeb } from '@/adapters/platform'
import * as Device from 'expo-device'

const LOW_END_MEMORY_THRESHOLD = 3 * 1024 * 1024 * 1024;

function isLowEndDevice(): boolean {
  if (isWeb || isIOS) return false;
  try {
    const totalMemory = (Device as any).totalMemory ?? 0
    return totalMemory > 0 && totalMemory < LOW_END_MEMORY_THRESHOLD
  } catch {
    return false
  }
}

export function VideoBackground({ children, disableVideo = false, active = true }: { children: ReactNode, disableVideo?: boolean, active?: boolean }) {
  const { theme } = useSettings()
  const { videoUri, imageUri, isLoading } = useThemeAssetUri(theme)
  const showImage = useMemo(() => isLowEndDevice() || disableVideo || !active, [disableVideo, active])

  const player = useVideoPlayer(null, player => {
    player.loop = true
    player.volume = 0
  })

  useEffect(() => {
    if (isLoading || !videoUri) return

    let cancelled = false
    player.replaceAsync(videoUri).then(() => {
      if (!cancelled) player.play()
    }).catch(() => { })

    return () => { cancelled = true }
  }, [player, videoUri, isLoading])

  const { status } = useSession()
  const isActiveSession = status !== 'disconnected'

  useEffect(() => {
    if (isActiveSession) {
      player.pause()
    } else {
      player.play()
    }
  }, [player, isActiveSession])

  useEffect(() => {
    const sub = AppState.addEventListener('change', (state) => {
      if (state === 'active' && !isActiveSession) {
        player.play()
      }
    })
    return () => sub.remove()
  }, [player, isActiveSession])

  return (
    <View style={styles.container}>
      {imageUri ? (
        <Image
          source={{ uri: imageUri }}
          style={styles.media}
          resizeMode="cover"
        />
      ) : null}
      {/* textureView (Android only, ignored on iOS): the default SurfaceView
          composites on a separate window layer that trails behind during
          screen/tab transitions — visible as jank over the video. TextureView
          composites inside the normal view hierarchy, so transitions stay
          smooth. Low-end devices never see the video (isLowEndDevice above). */}
      <VideoView
        player={player}
        style={[styles.media, { opacity: showImage ? 0 : 1 }]}
        contentFit="cover"
        nativeControls={false}
        surfaceType="textureView"
      />
      {/* No scrim: the video runs at full brightness. Legibility is carried
          entirely by the glass surfaces (light material + dark ink) — no text
          may float bare on the video. See src/styles/glass.ts. */}
      {children}
    </View>
  )
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#000',
    position: 'relative',
  },
  media: {
    position: 'absolute',
    top: 0,
    left: 0,
    width: '100%',
    height: '100%',
  },
})
