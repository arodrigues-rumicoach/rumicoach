import { useState, useCallback, type ReactNode } from 'react'
import { type ImageSourcePropType, type ImageStyle, type StyleProp, View } from 'react-native'
import Animated, { FadeIn } from 'react-native-reanimated'

interface LazyImageProps {
  source: ImageSourcePropType
  style?: StyleProp<ImageStyle>
  resizeMode?: 'cover' | 'contain' | 'stretch' | 'repeat' | 'center'
  placeholderColor?: string
  fallback?: ReactNode
}

const OBJECT_FIT_MAP: Record<string, string> = {
  cover: 'cover',
  contain: 'contain',
  stretch: 'fill',
  repeat: 'repeat',
  center: 'none',
}

const DEFAULT_ENTERING = FadeIn.duration(500).springify().damping(20).stiffness(200)

export function LazyImage({
  source,
  style,
  resizeMode = 'cover',
  placeholderColor = 'rgba(255,255,255,0.08)',
  fallback,
}: LazyImageProps) {
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState(false)

  const onLoad = useCallback(() => setLoaded(true), [])
  const onError = useCallback(() => setError(true), [])

  const uri = typeof source === 'object' && source !== null && 'uri' in source
    ? (source as { uri?: string }).uri
    : undefined

  if (error) {
    return (
      <View style={[style, { backgroundColor: placeholderColor, justifyContent: 'center', alignItems: 'center' }]}>
        {fallback}
      </View>
    )
  }

  return (
    <View style={style}>
      {!loaded && (
        <View style={[{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: placeholderColor }]} />
      )}
      <Animated.View style={{ flex: 1 }} entering={DEFAULT_ENTERING}>
        <img
          src={uri}
          alt=""
          onLoad={onLoad}
          onError={onError}
          style={{
            width: '100%',
            height: '100%',
            objectFit: (OBJECT_FIT_MAP[resizeMode] || 'cover') as any,
            opacity: loaded ? 1 : 0,
            transition: 'opacity 0.4s ease',
          }}
        />
      </Animated.View>
    </View>
  )
}
