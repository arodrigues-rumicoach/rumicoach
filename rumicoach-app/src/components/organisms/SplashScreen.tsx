import { useEffect, useState } from 'react'
import { View, StyleSheet, ImageBackground, Dimensions } from 'react-native'
import { StatusBar } from 'expo-status-bar'
import Animated, {
  FadeInDown,
  useSharedValue,
  useAnimatedStyle,
  withTiming,
  runOnJS,
} from 'react-native-reanimated'
import { useSettings } from '@/hooks/useSettings'
import { getThemeImageSource } from '@/utils/theme'
import i18n from '@/i18n'

interface SplashScreenProps {
  onHidden?: () => void
  delay?: number
}

const { height: SCREEN_HEIGHT } = Dimensions.get('window')

export function SplashScreen({ onHidden, delay = 3000 }: SplashScreenProps) {
  const { theme } = useSettings()
  const splashOpacity = useSharedValue(1)
  const imageSource = getThemeImageSource(theme)

  useEffect(() => {
    const hideTimer = setTimeout(() => {
      splashOpacity.value = withTiming(0, { duration: 600 }, (finished) => {
        if (finished && onHidden) {
          runOnJS(onHidden)()
        }
      })
    }, delay)
    return () => clearTimeout(hideTimer)
  }, [delay])

  const animatedStyle = useAnimatedStyle(() => ({
    opacity: splashOpacity.value,
  }))

  return (
    <Animated.View style={[styles.container, animatedStyle]}>
      <StatusBar style="light" />
      <ImageBackground
        source={imageSource}
        style={styles.image}
        resizeMode="cover"
      >
        <View style={styles.overlay} />
        <View style={styles.quoteContainer}>
          <Animated.Text
            entering={FadeInDown.duration(1000).springify()}
            style={styles.quoteText}
          >
            {i18n.t('splash_quote') || '"Yesterday I was clever, so I wanted to change the world. Today I am wise, so I am changing myself."'}
          </Animated.Text>
          <Animated.Text
            entering={FadeInDown.delay(500).duration(1000).springify()}
            style={styles.quoteAuthor}
          >
            {i18n.t('splash_quote_author') || '— Rumi'}
          </Animated.Text>
        </View>
      </ImageBackground>
    </Animated.View>
  )
}

const styles = StyleSheet.create({
  container: {
    ...StyleSheet.absoluteFill,
    zIndex: 999,
  },
  image: {
    flex: 1,
    width: '100%',
    height: SCREEN_HEIGHT,
  },
  overlay: {
    ...StyleSheet.absoluteFill,
    backgroundColor: 'rgba(0,0,0,0.55)',
  },
  quoteContainer: {
    flex: 1,
    justifyContent: 'center',
    alignItems: 'center',
    paddingHorizontal: 32,
  },
  quoteText: {
    fontSize: 22,
    fontWeight: '600',
    color: '#fff',
    textAlign: 'center',
    lineHeight: 34,
    fontStyle: 'italic',
  },
  quoteAuthor: {
    marginTop: 20,
    fontSize: 16,
    fontWeight: '300',
    color: 'rgba(255,255,255,0.7)',
    letterSpacing: 1,
  },
})