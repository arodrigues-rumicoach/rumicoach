import { useState, useCallback, type ReactNode } from 'react'
import { Image, type ImageSourcePropType, type ImageStyle, type StyleProp, View } from 'react-native'
import Animated, { FadeIn } from 'react-native-reanimated'

interface LazyImageProps {
  source: ImageSourcePropType
  style?: StyleProp<ImageStyle>
  resizeMode?: 'cover' | 'contain' | 'stretch' | 'repeat' | 'center'
  placeholderColor?: string
  fallback?: ReactNode
  blurRadius?: number
}

const DEFAULT_ENTERING = FadeIn.duration(500).springify().damping(20).stiffness(200)

export function LazyImage({
  source,
  style,
  resizeMode = 'cover',
  placeholderColor = 'rgba(255,255,255,0.08)',
  fallback,
  blurRadius,
}: LazyImageProps) {
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState(false)

  const onLoad = useCallback(() => setLoaded(true), [])
  const onError = useCallback(() => setError(true), [])

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
        <Image
          source={source}
          style={{ flex: 1 }}
          resizeMode={resizeMode}
          onLoad={onLoad}
          onError={onError}
          blurRadius={blurRadius}
        />
      </Animated.View>
    </View>
  )
}
