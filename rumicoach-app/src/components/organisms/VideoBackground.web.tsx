import { type ReactNode, useRef, useState, useEffect } from 'react'
import { View, StyleSheet, Image } from 'react-native'
import { useSettings } from '@/hooks/useSettings'
import { useSession } from '@/hooks/useSession'
import { useThemeAssetUri } from '@/hooks/useThemeAssetUri'

function attachInteractionListener(video: HTMLVideoElement) {
  const startOnInteraction = () => {
    video.play().catch(() => { })
    document.removeEventListener('click', startOnInteraction, true)
    document.removeEventListener('keydown', startOnInteraction, true)
    document.removeEventListener('touchstart', startOnInteraction, true)
    document.removeEventListener('pointerdown', startOnInteraction, true)
  }
  document.addEventListener('click', startOnInteraction, { capture: true, once: true })
  document.addEventListener('keydown', startOnInteraction, { capture: true, once: true })
  document.addEventListener('touchstart', startOnInteraction, { capture: true, once: true })
  document.addEventListener('pointerdown', startOnInteraction, { capture: true, once: true })
}

export function VideoBackground({ children, disableVideo = false, active = true }: { children: ReactNode, disableVideo?: boolean, active?: boolean }) {
  const { theme } = useSettings()
  const { videoUri, imageUri, isLoading } = useThemeAssetUri(theme)
  const videoRef = useRef<HTMLVideoElement>(null)
  const [videoError, setVideoError] = useState(false)
  const showImage = disableVideo || !active

  useEffect(() => {
    setVideoError(false)
  }, [theme])

  const { status } = useSession()
  const isActiveSession = status !== 'disconnected'

  useEffect(() => {
    const video = videoRef.current
    if (!video || showImage || videoError || !videoUri) return

    if (isActiveSession) {
      video.pause()
      return
    }

    video.load()

    const hasActivation = !!(navigator as any).userActivation?.isActive
    if (hasActivation) {
      video.play().catch(() => { })
      return
    }

    const playPromise = video.play()
    if (playPromise !== undefined) {
      playPromise.catch(() => {
        attachInteractionListener(video)
      })
    }
  }, [theme, showImage, videoError, videoUri])

  useEffect(() => {
    const video = videoRef.current
    if (!video || showImage || videoError || !videoUri) return

    if (isActiveSession) {
      video.pause()
    } else if (video.paused) {
      video.play().catch(() => { })
    }
  }, [isActiveSession, showImage, videoError, videoUri])

  return (
    <View style={styles.container}>
      {imageUri ? (
        <Image
          source={{ uri: imageUri }}
          style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%' } as any}
          resizeMode="cover"
        />
      ) : null}
      <video
        ref={videoRef}
        key={theme}
        autoPlay
        muted
        loop
        playsInline
        onError={() => setVideoError(true)}
        style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%', objectFit: 'cover', opacity: (showImage || videoError || !videoUri) ? 0 : 1 } as any}
      >
        {videoUri ? <source src={videoUri} type="video/mp4" /> : null}
      </video>
      {/* No scrim — see VideoBackground.tsx. */}
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
})
