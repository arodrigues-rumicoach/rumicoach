import { useState, useEffect } from 'react'
import { View, TouchableOpacity, StyleSheet, ScrollView, Keyboard, KeyboardAvoidingView, Platform } from 'react-native'
import { Text } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { router } from 'expo-router'
import { ChevronRight, Mars, Play, Square, Venus } from 'lucide-react-native'
import i18n from '../../../src/i18n'
import { useSettings } from '../../../src/hooks/useSettings'
import { useAuth } from '../../../src/hooks/useAuth'
import { getThemeImageSource, type ThemeType } from '../../../src/utils/theme'
import { ThemedButton, LazyImage, GlassCard, ThemedSpinner } from '@/components/atoms'
import { Toast } from '@/components/molecules'
import type { User } from '../../../src/api'
import { useVoicePreview } from '@/hooks/useVoicePreview'
import { useBlurTarget } from '@/context/BlurContext'
import Reanimated, { FadeInDown } from 'react-native-reanimated'
import { INK } from '@/styles/glass'

type VisualizerType = 'organic' | 'waveform'

interface AppFormData {
  visualizerType: VisualizerType
  coachGender: string
  coachVoice: string
  theme: ThemeType
}

function SectionLabel({ children }: { children: string }) {
  return (
    <Text fontSize={12} fontWeight="700" letterSpacing={0.5} color="$onGlassSecondary" textTransform="uppercase">
      {children}
    </Text>
  )
}

export default function SettingsAppScreen() {
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const { colorScheme, setTheme, setVisualizerType, theme: currentTheme, visualizerType: currentVisualizerType, shakeToReport, setShakeToReport } = useSettings()
  const { user, updateUser, ensureValidToken } = useAuth()
  const { playingVoice, loadingVoice, play: playVoice } = useVoicePreview()

  const [formData, setFormDataState] = useState({
    visualizerType: (user?.visualizerType || currentVisualizerType || 'organic') as VisualizerType,
    coachGender: user?.coachGender || '',
    coachVoice: user?.coachVoice || '',
    theme: (user?.theme || currentTheme || 'waterfall') as ThemeType,
  })
  const [hasChanges, setHasChanges] = useState(false)
  const [loading, setLoading] = useState(false)
  const [message, setMessage] = useState<{ text: string; type: 'success' | 'error' } | null>(null)

  const setFormData = (value: AppFormData | ((prev: AppFormData) => AppFormData)) => {
    setFormDataState(value)
  }

  useEffect(() => {
    if (!user) {
      setHasChanges(false)
      return
    }
    setFormData({
      visualizerType: (user.visualizerType || currentVisualizerType || 'organic') as VisualizerType,
      coachGender: user.coachGender || '',
      coachVoice: user.coachVoice || '',
      theme: (user.theme || currentTheme || 'waterfall') as ThemeType,
    })
  }, [user, currentTheme, currentVisualizerType])

  useEffect(() => {
    if (!user) return
    const isModified =
      formData.visualizerType !== (user.visualizerType || currentVisualizerType || 'organic') ||
      formData.coachGender !== (user.coachGender || '') ||
      formData.coachVoice !== (user.coachVoice || '') ||
      formData.theme !== (user.theme || currentTheme || 'waterfall')
    setHasChanges(isModified)
  }, [formData, user, currentTheme, currentVisualizerType])

  const handleSubmit = async () => {
    setLoading(true)
    setMessage(null)
    Keyboard.dismiss()
    try {
      await ensureValidToken()
      await updateUser(formData as Partial<User>)
      setVisualizerType(formData.visualizerType)
      setTheme(formData.theme)
      setMessage({ text: i18n.t('profile_updated') || 'Settings updated', type: 'success' })
    } catch {
      setMessage({ text: i18n.t('failed_update') || 'Update failed', type: 'error' })
    } finally {
      setLoading(false)
    }
  }

  const voices = formData.coachGender === 'female'
    ? [
      { id: 'gacrux', label: i18n.t('voice_gacrux') || 'Gacrux' },
      { id: 'aoede', label: i18n.t('voice_aoede') || 'Aoede' },
      { id: 'vindemiatrix', label: i18n.t('voice_vindemiatrix') || 'Vindemiatrix' },
    ]
    : [
      { id: 'algieba', label: i18n.t('voice_algieba') || 'Algieba' },
      { id: 'enceladus', label: i18n.t('voice_enceladus') || 'Enceladus' },
      { id: 'charon', label: i18n.t('voice_charon') || 'Charon' },
    ]

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
    >
      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 100 }]}
        keyboardShouldPersistTaps="handled"
      >
        <Reanimated.View entering={FadeInDown.duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={10} blurTarget={blurTargetRef}>
            <SectionLabel>{i18n.t('visualizer_style') || 'Visualizer Style'}</SectionLabel>
            <View style={styles.optionsRow}>
              <ThemedButton
                variant={formData.visualizerType === 'organic' ? 'solid' : 'outline'}
                onPress={() => setFormData(p => ({ ...p, visualizerType: 'organic' }))}
              >
                {i18n.t('organic_orb') || 'Organic Orb'}
              </ThemedButton>
              <ThemedButton
                variant={formData.visualizerType === 'waveform' ? 'solid' : 'outline'}
                onPress={() => setFormData(p => ({ ...p, visualizerType: 'waveform' }))}
              >
                {i18n.t('waveform_bars') || 'Waveform Bars'}
              </ThemedButton>
            </View>

            <SectionLabel>{i18n.t('coach_gender') || 'Coach Gender'}</SectionLabel>
            <View style={styles.optionsRow}>
              <ThemedButton
                variant={formData.coachGender === 'male' ? 'solid' : 'outline'}
                onPress={() => setFormData(p => ({ ...p, coachGender: 'male', coachVoice: p.coachGender === 'male' ? p.coachVoice : 'algieba' }))}
                icon={<Mars size={20} color={formData.coachGender === 'male' ? "#fff" : colorScheme.primary} />}
              >
                {i18n.t('male') || 'Male'}
              </ThemedButton>
              <ThemedButton
                variant={formData.coachGender === 'female' ? 'solid' : 'outline'}
                onPress={() => setFormData(p => ({ ...p, coachGender: 'female', coachVoice: p.coachGender === 'female' ? p.coachVoice : 'gacrux' }))}
                icon={<Venus size={20} color={formData.coachGender === 'female' ? "#fff" : colorScheme.primary} />}
              >
                {i18n.t('female') || 'MFemaleale'}
              </ThemedButton>
            </View>

            <SectionLabel>{i18n.t('coach_voice') || 'Coach Voice'}</SectionLabel>
            <View style={styles.optionsRowWrap}>
              {voices.map(v => (
                <View style={styles.optionsRow} key={v.id}>
                  <ThemedButton
                    size="$3"
                    onPress={() => setFormData(p => ({ ...p, coachVoice: v.id }))}
                    variant={formData.coachVoice === v.id ? 'solid' : 'outline'}
                  >
                    {v.label}
                  </ThemedButton>
                  <TouchableOpacity
                    onPress={() => playVoice(v.id)}
                    disabled={!!loadingVoice && loadingVoice === v.id}
                    style={{
                      width: 32,
                      height: 32,
                      borderRadius: 16,
                      backgroundColor: loadingVoice === v.id ? 'rgba(16,185,129,0.2)' : '#fff',
                      justifyContent: 'center',
                      alignItems: 'center',
                      opacity: loadingVoice && loadingVoice === v.id ? 0.4 : 1,
                    }}
                  >
                    {loadingVoice === v.id ? (
                      <ThemedSpinner size="small" color="$accent" />
                    ) : playingVoice === v.id ? (
                      <Square size={12} color={INK.primary} />
                    ) : (
                      <Play size={12} color={INK.primary} style={{ borderRadius: 99 }} />
                    )}
                  </TouchableOpacity>
                </View>
              ))}
            </View>

            {/* Mobile only: web has no dependable accelerometer, so offering the
                switch there would promise something that never fires. Applies
                immediately and stays on this device — it is not part of the
                Save flow because it never leaves the phone. */}
            {Platform.OS !== 'web' && (
              <>
                <SectionLabel>{i18n.t('shake_to_report') || 'Shake to report a problem'}</SectionLabel>
                <Text fontSize={12} color="$onGlassSecondary" lineHeight={16}>
                  {i18n.t('shake_to_report_detail') || 'Shake your phone to open the feedback form from anywhere.'}
                </Text>
                <View style={styles.optionsRow}>
                  <ThemedButton
                    variant={shakeToReport ? 'solid' : 'outline'}
                    onPress={() => setShakeToReport(true)}
                  >
                    <Text fontSize={14} color={shakeToReport ? "#fff" : "#262220"}>{i18n.t('on') || 'On'}</Text>
                  </ThemedButton>
                  <ThemedButton
                    variant={!shakeToReport ? 'solid' : 'outline'}
                    onPress={() => setShakeToReport(false)}
                  >
                    <Text fontSize={14} color={!shakeToReport ? "#fff" : "#262220"}>{i18n.t('off') || 'Off'}</Text>
                  </ThemedButton>
                </View>
              </>
            )}

            <SectionLabel>{i18n.t('choose_theme') || 'Choose Theme'}</SectionLabel>
            <TouchableOpacity
              style={styles.themeRow}
              onPress={() => router.push('/(settings)/app/theme')}
              activeOpacity={0.7}
            >
              <View style={styles.themeInfo}>
                <View style={styles.themeThumb}>
                  <LazyImage
                    source={getThemeImageSource(formData.theme)}
                    style={{ width: 32, height: 32 }}
                    resizeMode="cover"
                  />
                </View>
                <Text fontSize={14} color="$onGlass">
                  {i18n.t(`theme_${formData.theme}`) || formData.theme || 'waterfall'}
                </Text>
              </View>
              <ChevronRight size={20} color="#4A4540" />
            </TouchableOpacity>
          </GlassCard>
        </Reanimated.View>
      </ScrollView>

      {message && <Toast message={message.text} type={message.type} onClose={() => setMessage(null)} />}

      {hasChanges && (
        <View style={[styles.saveBar, { paddingBottom: insets.bottom + 16 }]}>
          <ThemedButton variant="solid" fullWidth disabled={loading} onPress={handleSubmit}>
            {i18n.t('save_changes') || 'Save Changes'}
          </ThemedButton>
        </View>
      )}
    </KeyboardAvoidingView>
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
  optionsRow: {
    flexDirection: 'row',
    gap: 10,
  },
  optionsRowWrap: {
    flexWrap: 'wrap',
    flexDirection: 'row',
    gap: 10,
  },
  optionsColumn: {
    gap: 8,
  },
  optionItem: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    padding: 12,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.10)',
    backgroundColor: 'rgba(0,0,0,0.03)',
  },
  optionItemSelected: {
    borderColor: 'rgba(0,0,0,0.30)',
    backgroundColor: 'rgba(0,0,0,0.08)',
    borderWidth: 2,
  },
  optionItemSelectedAccent: {
    borderColor: 'rgba(16,185,129,0.40)',
    backgroundColor: 'rgba(16,185,129,0.08)',
    borderWidth: 2,
  },
  voiceButton: {
    width: 28,
    height: 28,
    borderRadius: 14,
    backgroundColor: 'rgba(0,0,0,0.06)',
    justifyContent: 'center',
    alignItems: 'center',
  },
  voiceButtonActive: {
    backgroundColor: 'rgba(0,0,0,0.12)',
  },
  voiceButtonDisabled: {
    opacity: 0.4,
  },
  themeRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: 12,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.10)',
    backgroundColor: 'rgba(0,0,0,0.03)',
  },
  themeInfo: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  themeThumb: {
    width: 32,
    height: 32,
    borderRadius: 4,
    overflow: 'hidden',
  },
  saveBar: {
    position: 'absolute',
    bottom: 0,
    left: 0,
    right: 0,
    padding: 16,
    paddingBottom: 40,
  },
})
