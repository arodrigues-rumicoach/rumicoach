import { useState } from 'react'
import { View, TouchableOpacity, Modal, StyleSheet, Dimensions, Platform } from 'react-native'
import { Text } from '@tamagui/core'
import { ChevronDown, Check } from 'lucide-react-native'
import i18n, { useI18n } from '@/i18n'
import { useAuth } from '@/hooks/useAuth'
import { useSettings } from '@/hooks/useSettings'
import { LANGUAGES } from '@/utils/languages'
import { ThemedIconButton } from '@/components/atoms'

interface LanguageSwitcherProps {
  variant?: 'default' | 'minimal'
  color?: string
  align?: 'left' | 'right'
}

const { height: SCREEN_HEIGHT } = Dimensions.get('window')
const isWeb = Platform.OS === 'web'

export function LanguageSwitcher({ variant = 'default', color }: LanguageSwitcherProps) {
  const { locale, setLanguage: setI18n } = useI18n()
  const { setLanguage: setAuthLang } = useAuth()
  const { colorScheme } = useSettings()
  const [isOpen, setIsOpen] = useState(false)

  const currentLanguage = LANGUAGES.find(l => l.code === locale) || LANGUAGES.find(l => l.code === 'en-US')

  const handleLanguageSelect = (code: string) => {
    setI18n(code)
    setAuthLang(code)
    setIsOpen(false)
  }

  const triggerColor = color || colorScheme.secondary

  return (
    <View>
      <ThemedIconButton onPress={() => setIsOpen(true)} style={{ alignItems: 'center', display: 'flex', justifyContent: 'center' }}>
        {variant === 'default' ? (
          <View style={styles.defaultTrigger}>
            <Text style={[styles.flagText, { color: triggerColor }]}>{currentLanguage?.flag}</Text>
            <Text style={[styles.langName, { color: triggerColor }]}>{currentLanguage?.name}</Text>
            <ChevronDown size={14} color={triggerColor} />
          </View>
        ) : (
          <Text style={styles.flagLarge}>{currentLanguage?.flag}</Text>
        )}
      </ThemedIconButton>

      <Modal visible={isOpen} transparent animationType={isWeb ? 'fade' : 'slide'} onRequestClose={() => setIsOpen(false)}>
        <TouchableOpacity
          style={[styles.backdrop, !isWeb && { justifyContent: 'flex-end' }]}
          activeOpacity={1}
          onPress={() => setIsOpen(false)}
        >
          {isWeb ? (
            <View style={[styles.webDialog, { backgroundColor: colorScheme.tertiary }]}>
              <Text style={[styles.sheetTitle, { color: colorScheme.secondary }]}>{i18n.t('select_language') || 'Select Language'}</Text>
              <View style={styles.optionsList}>
                {LANGUAGES.map((lang) => {
                  const isActive = locale === lang.code
                  return (
                    <TouchableOpacity
                      key={lang.code}
                      style={[
                        styles.option,
                        isActive && { backgroundColor: colorScheme.primary + '30' },
                      ]}
                      onPress={() => handleLanguageSelect(lang.code)}
                      activeOpacity={0.7}
                    >
                      <View style={styles.optionContent}>
                        <Text style={styles.optionFlag}>{lang.flag}</Text>
                        <Text
                          style={[
                            styles.optionName,
                            { color: colorScheme.secondary },
                            isActive && { fontWeight: '700' },
                          ]}
                        >
                          {lang.name}
                        </Text>
                      </View>
                      {isActive && <Check size={18} color={colorScheme.secondary} />}
                    </TouchableOpacity>
                  )
                })}
              </View>
            </View>
          ) : (
            <View style={[styles.sheet, { backgroundColor: colorScheme.tertiary }]}>
              <View style={[styles.handle, { backgroundColor: colorScheme.secondary }]} />
              <Text style={[styles.sheetTitle, { color: colorScheme.secondary }]}>{i18n.t('select_language') || 'Select Language'}</Text>
              <View style={styles.optionsList}>
                {LANGUAGES.map((lang) => {
                  const isActive = locale === lang.code
                  return (
                    <TouchableOpacity
                      key={lang.code}
                      style={[
                        styles.option,
                        isActive && { backgroundColor: colorScheme.primary + '30' },
                      ]}
                      onPress={() => handleLanguageSelect(lang.code)}
                      activeOpacity={0.7}
                    >
                      <View style={styles.optionContent}>
                        <Text style={styles.optionFlag}>{lang.flag}</Text>
                        <Text
                          style={[
                            styles.optionName,
                            { color: colorScheme.secondary },
                            isActive && { fontWeight: '700' },
                          ]}
                        >
                          {lang.name}
                        </Text>
                      </View>
                      {isActive && <Check size={18} color={colorScheme.secondary} />}
                    </TouchableOpacity>
                  )
                })}
              </View>
            </View>
          )}
        </TouchableOpacity>
      </Modal>
    </View>
  )
}

const styles = StyleSheet.create({
  defaultTrigger: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  flagText: {
    fontSize: 18,
    lineHeight: 18,
  },
  langName: {
    fontSize: 14,
    fontWeight: '500',
  },
  flagLarge: {
    fontSize: 20,
    lineHeight: 24,
  },
  backdrop: {
    flex: 1,
    justifyContent: 'center',
    alignItems: 'center',
  },
  webDialog: {
    width: 320,
    borderRadius: 16,
    paddingTop: 20,
    paddingBottom: 20,
    paddingHorizontal: 20,
    maxHeight: SCREEN_HEIGHT * 0.6,
  },
  sheet: {
    borderTopLeftRadius: 20,
    borderTopRightRadius: 20,
    paddingTop: 12,
    paddingBottom: 40,
    paddingHorizontal: 20,
    maxHeight: SCREEN_HEIGHT * 0.5,
    width: '100%',
  },
  handle: {
    width: 36,
    height: 4,
    borderRadius: 2,
    alignSelf: 'center',
    marginBottom: 16,
    opacity: 0.3,
  },
  sheetTitle: {
    fontSize: 17,
    fontWeight: '600',
    textAlign: 'center',
    marginBottom: 16,
  },
  optionsList: {
    gap: 4,
    maxHeight: '100%',
    overflow: 'scroll',
    paddingBottom: 24
  },
  option: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingVertical: 14,
    paddingHorizontal: 16,
    borderRadius: 12,
  },
  optionContent: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  optionFlag: {
    fontSize: 22,
    lineHeight: 22,
  },
  optionName: {
    fontSize: 16,
  },
})
