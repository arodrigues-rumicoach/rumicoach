import { TouchableOpacity, StyleSheet, ScrollView, View } from 'react-native'
import { Text } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { router } from 'expo-router'
import i18n from '../../src/i18n'
import { useSettings } from '../../src/hooks/useSettings'
import { useAuth } from '../../src/hooks/useAuth'
import { useBlurTarget } from '@/context/BlurContext'
import { LANGUAGES } from '../../src/utils/languages'
import { GlassCard } from '@/components/atoms'

export default function SettingsLanguageScreen() {
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const { colorScheme } = useSettings()
  const { appLanguage, setLanguage } = useAuth()

  const handleSelect = async (code: string) => {
    await setLanguage(code)
    router.back()
  }

  return (
    <ScrollView style={styles.scrollArea} contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 32 }]}>
      <GlassCard variant="light" borderRadius={18} padding={16} gap={10} blurTarget={blurTargetRef}>
        {LANGUAGES.map((lang, index) => (
          <View key={lang.code}>
            {index > 0 && <View style={styles.separator} />}
            <TouchableOpacity
              style={[styles.langOption, appLanguage === lang.code && styles.langOptionSelected]}
              onPress={() => handleSelect(lang.code)}
              activeOpacity={0.7}
            >
              <Text style={{ fontSize: 20 }}>{lang.flag}</Text>
              <Text fontSize={14} fontWeight="500" color="$onGlass" flex={1}>{lang.name}</Text>
              {appLanguage === lang.code && (
                <View style={{ width: 8, height: 8, borderRadius: 4, backgroundColor: colorScheme.secondary }} />
              )}
            </TouchableOpacity>
          </View>
        ))}
      </GlassCard>
    </ScrollView>
  )
}

const styles = StyleSheet.create({
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 100,
  },
  separator: {
    height: 1,
    backgroundColor: 'rgba(0,0,0,0.10)',
  },
  langOption: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
    paddingVertical: 12,
    paddingHorizontal: 4,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: 'transparent',
  },
  langOptionSelected: {
    borderColor: 'rgba(16,185,129,0.40)',
    backgroundColor: 'rgba(16,185,129,0.08)',
    borderWidth: 2,
  },
})
