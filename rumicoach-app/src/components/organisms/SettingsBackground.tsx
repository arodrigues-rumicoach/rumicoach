import { type ReactNode } from 'react'
import { View, StyleSheet, Image } from 'react-native'
import { useSettings } from '@/hooks/useSettings'
import { getThemeImageSource } from '@/utils/theme'

export function SettingsBackground({ children }: { children: ReactNode }) {
  const { theme } = useSettings()
  const imageSource = getThemeImageSource(theme)

  return (
    <View style={styles.container}>
      <Image
        source={imageSource}
        style={styles.image}
        resizeMode="cover"
        blurRadius={8}
      />
      <View style={styles.overlay} />
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
  image: {
    position: 'absolute',
    top: 0,
    left: 0,
    width: '100%',
    height: '100%',
  },
  overlay: {
    ...StyleSheet.absoluteFill,
    backgroundColor: 'rgba(0,0,0,0.15)',
  },
})
