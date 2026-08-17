import { View, ScrollView, TouchableOpacity, StyleSheet } from 'react-native'
import { Text, XStack } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { router, useFocusEffect } from 'expo-router'
import { Check, Clock, ChevronRight, Smartphone, Send } from 'lucide-react-native'
import { useCallback, useState } from 'react'
import i18n from '../../../src/i18n'
import { api, type Integration, type IntegrationStatus } from '../../../src/api'
import { useAuth } from '../../../src/hooks/useAuth'
import { useBlurTarget } from '@/context/BlurContext'
import { GlassCard } from '@/components/atoms'
import { VerificationModal } from '@/components/molecules'
import Reanimated, { FadeInDown } from 'react-native-reanimated'

function SectionLabel({ children }: { children: string }) {
  return (
    <Text fontSize={12} fontWeight="700" letterSpacing={0.5} color="$onGlassSecondary" textTransform="uppercase">
      {children}
    </Text>
  )
}

export default function IntegrationsIndexScreen() {
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const { user } = useAuth()
  const [showPhoneVerification, setShowPhoneVerification] = useState(false)
  const [integrations, setIntegrations] = useState<Integration[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(() => {
    api.get<Integration[]>('/me/integrations')
      .then(({ data }) => setIntegrations(data))
      .catch(() => { })
      .finally(() => setLoading(false))
  }, [])

  // Re-read on focus: linking happens on another screen (and completes in
  // WhatsApp), so the status here is stale whenever the user comes back.
  useFocusEffect(refresh)

  const getIntegrationStatus = useCallback(
    (provider: string): IntegrationStatus | undefined =>
      integrations.find((i) => i.provider === provider)?.status,
    [integrations],
  )

  const getIntegration = useCallback(
    (provider: string): Integration | undefined =>
      integrations.find((i) => i.provider === provider),
    [integrations],
  )

  // Navigating by provider name STARTS A NEW LINK, which on the backend replaces
  // whatever binding exists. Until the list has loaded we cannot tell whether the user
  // is already connected, so acting now risks tearing down a working connection — QA
  // hit exactly that by tapping before the fetch returned. Wait instead.
  const handleWhatsApp = useCallback(() => {
    if (loading) return
    const integration = getIntegration('whatsapp')
    if (integration?.status === 'active' || integration?.status === 'pending') {
      router.push(`(settings)/integrations/${integration.id}`)
      return
    }
    if (!user?.phoneNumber) {
      setShowPhoneVerification(true)
      return
    }
    router.push('(settings)/integrations/whatsapp')
  }, [loading, user?.phoneNumber, getIntegration])

  // Telegram links through a bot deep link (t.me/<bot>?start=<code>), so unlike
  // WhatsApp there's no phone number to verify first.
  const handleTelegram = useCallback(() => {
    if (loading) return
    const integration = getIntegration('telegram')
    if (integration?.status === 'active' || integration?.status === 'pending') {
      router.push(`(settings)/integrations/${integration.id}`)
      return
    }
    router.push('(settings)/integrations/telegram')
  }, [loading, getIntegration])

  const handlePhoneVerified = useCallback(() => {
    setShowPhoneVerification(false)
    router.push('/(settings)/integrations/whatsapp')
  }, [])

  const handleCancelVerification = useCallback(() => {
    setShowPhoneVerification(false)
  }, [])

  return (
    <View style={{ flex: 1 }}>
      <ScrollView style={styles.scrollArea} contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 32 }]}>
        <Reanimated.View entering={FadeInDown.duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={0} blurTarget={blurTargetRef}>
            <SectionLabel>{i18n.t('available_integrations') || 'Available Integrations'}</SectionLabel>

            <TouchableOpacity
              style={styles.menuItem}
              onPress={handleWhatsApp}
              activeOpacity={0.7}
            >
              <View style={[styles.menuIcon, { backgroundColor: 'rgba(37, 211, 102, 0.1)' }]}>
                <Smartphone size={20} color="#25D366" />
              </View>
              <View style={styles.textContainer}>
                <XStack alignItems="center" gap={6}>
                  <Text fontSize={15} fontWeight="500" color="$onGlass">
                    {i18n.t('whatsapp') || 'WhatsApp'}
                  </Text>
                  {(() => {
                    const status = getIntegrationStatus('whatsapp')
                    if (status === 'active') {
                      return (
                        <View style={styles.connectedBadge}>
                          <Check size={10} color="#fff" />
                          <Text fontSize={10} fontWeight="700" color="#fff">
                            {i18n.t('connected') || 'Connected'}
                          </Text>
                        </View>
                      )
                    }
                    if (status === 'pending') {
                      return (
                        <View style={styles.pendingBadge}>
                          <Clock size={10} color="#fff" />
                          <Text fontSize={10} fontWeight="700" color="#fff">
                            {i18n.t('pending') || 'Pending'}
                          </Text>
                        </View>
                      )
                    }
                    return null
                  })()}
                </XStack>
                <Text fontSize={12} color="$onGlassSecondary">
                  {getIntegrationStatus('whatsapp') === 'active'
                    ? (i18n.t('whatsapp_connected_subtitle') || 'You\'re connected')
                    : (i18n.t('connect_whatsapp_subtitle') || 'Connect to chat via WhatsApp')}
                </Text>
              </View>
              <ChevronRight size={16} color="#4A4540" />
            </TouchableOpacity>

            <View style={styles.separator} />

            <TouchableOpacity
              style={styles.menuItem}
              onPress={handleTelegram}
              activeOpacity={0.7}
            >
              <View style={[styles.menuIcon, { backgroundColor: 'rgba(0, 136, 204, 0.1)' }]}>
                <Send size={20} color="#0088cc" />
              </View>
              <View style={styles.textContainer}>
                <XStack alignItems="center" gap={6}>
                  <Text fontSize={15} fontWeight="500" color="$onGlass">
                    {i18n.t('telegram') || 'Telegram'}
                  </Text>
                  {(() => {
                    const status = getIntegrationStatus('telegram')
                    if (status === 'active') {
                      return (
                        <View style={[styles.connectedBadge, { backgroundColor: '#0088cc' }]}>
                          <Check size={10} color="#fff" />
                          <Text fontSize={10} fontWeight="700" color="#fff">
                            {i18n.t('connected') || 'Connected'}
                          </Text>
                        </View>
                      )
                    }
                    if (status === 'pending') {
                      return (
                        <View style={styles.pendingBadge}>
                          <Clock size={10} color="#fff" />
                          <Text fontSize={10} fontWeight="700" color="#fff">
                            {i18n.t('pending') || 'Pending'}
                          </Text>
                        </View>
                      )
                    }
                    return null
                  })()}
                </XStack>
                <Text fontSize={12} color="$onGlassSecondary">
                  {getIntegrationStatus('telegram') === 'active'
                    ? (i18n.t('telegram_connected_subtitle') || 'You\'re connected')
                    : (i18n.t('connect_telegram_subtitle') || 'Connect to chat via Telegram')}
                </Text>
              </View>
              <ChevronRight size={16} color="#4A4540" />
            </TouchableOpacity>

          </GlassCard>
        </Reanimated.View>
      </ScrollView>

      <VerificationModal
        visible={showPhoneVerification}
        type="phone"
        title={i18n.t('add_phone_number') || 'Add Phone Number'}
        subtitle={i18n.t('enter_phone_number') || 'Enter your phone number'}
        onVerified={handlePhoneVerified}
        onCancel={handleCancelVerification}
      />
    </View>
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
    marginVertical: 4,
  },
  menuItem: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: 12,
    gap: 12,
  },
  menuIcon: {
    width: 36,
    height: 36,
    borderRadius: 18,
    justifyContent: 'center',
    alignItems: 'center',
  },
  textContainer: {
    flex: 1,
    gap: 2,
  },
  connectedBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 3,
    backgroundColor: '#25D366',
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  pendingBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 3,
    backgroundColor: '#F59E0B',
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
})
