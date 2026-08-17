import { View, ScrollView, StyleSheet, Linking, ActivityIndicator, TouchableOpacity } from 'react-native'
import { Text, XStack, YStack } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { useLocalSearchParams, router } from 'expo-router'
import { Smartphone, ExternalLink, AlertCircle, MessageCircle, Send, Trash2, Type, Mic, Sparkles, RefreshCw } from 'lucide-react-native'
import { useState, useEffect, useCallback, useRef } from 'react'
import i18n from '../../../src/i18n'
import { useAuth } from '../../../src/hooks/useAuth'
import { api, type Integration, type ChannelLinkResponse, type ReplyMode } from '../../../src/api'
import { useBlurTarget } from '@/context/BlurContext'
import { GlassCard, ThemedButton } from '@/components/atoms'
import Reanimated, { FadeInDown } from 'react-native-reanimated'
import {
  trackIntegrationLinkStarted,
  trackIntegrationUnlinked,
  trackReplyModeChanged,
} from '@/analytics'

const KNOWN_PROVIDERS = ['whatsapp', 'telegram'] as const

/** Per-provider chrome. Copy is looked up by key (`connect_<p>`,
 *  `<p>_integration_desc`, `open_in_<p>`, `send_code_to_<p>`,
 *  `<p>_not_available`), so adding a channel means adding those five strings. */
const PROVIDER_UI: Record<string, { Icon: typeof Smartphone; color: string; tint: string }> = {
  whatsapp: { Icon: Smartphone, color: '#25D366', tint: 'rgba(37, 211, 102, 0.1)' },
  telegram: { Icon: Send, color: '#0088cc', tint: 'rgba(0, 136, 204, 0.1)' },
}
const FALLBACK_UI = { Icon: MessageCircle, color: '#4A4540', tint: 'rgba(0,0,0,0.05)' }

/** How often we re-check whether the user's first message has landed. The
 *  webhook flips the binding to `active` server-side; there's no push. */
const POLL_INTERVAL_MS = 5000

const REPLY_MODES: { mode: ReplyMode; Icon: typeof Type }[] = [
  { mode: 'text', Icon: Type },
  { mode: 'audio', Icon: Mic },
  { mode: 'auto', Icon: Sparkles },
]

function formatCountdown(msLeft: number): string {
  const total = Math.max(0, Math.floor(msLeft / 1000))
  const m = Math.floor(total / 60)
  const s = total % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

export default function IntegrationProviderScreen() {
  const { provider: param } = useLocalSearchParams<{ provider: string }>()
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const { ensureValidToken } = useAuth()

  // The route param is either a provider name (start a fresh link) or the id of
  // an integration the user already has. Prefer the id for reads/writes: the
  // backend resolves a provider name with `First(...)`, which can surface a
  // stale revoked row once the user has re-linked.
  const isProviderName = KNOWN_PROVIDERS.includes(param as any)

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [linkData, setLinkData] = useState<ChannelLinkResponse | null>(null)
  const [integration, setIntegration] = useState<Integration | null>(null)
  const [cancelling, setCancelling] = useState(false)
  const [savingMode, setSavingMode] = useState(false)
  const [now, setNow] = useState(() => Date.now())

  // The provider we're operating on, whichever way we got here.
  const providerName = (isProviderName ? param : integration?.provider) as string | undefined

  const isActive = integration?.status === 'active'
  const isPending = integration?.status === 'pending'
  const codeExpired = linkData ? new Date(linkData.expiresAt).getTime() <= now : false
  // Poll while the user is mid-link — either showing a code, or holding a
  // pending binding that the webhook may activate at any moment.
  const isWaiting = !isActive && (Boolean(linkData) || isPending)

  const requestLinkCode = useCallback(async (target: string) => {
    try {
      setLoading(true)
      setError(null)
      await ensureValidToken()
      const { data } = await api.post<ChannelLinkResponse>(`/me/integrations/${target}/link`)
      // The code was issued, not yet redeemed — whether they finish is the drop-off
      // this measures, and the linked state itself only ever arrives from the server.
      trackIntegrationLinkStarted(target)
      setLinkData(data)
      setNow(Date.now())
      // Linking revokes any previous binding and creates a fresh pending one,
      // so whatever detail we were showing is now stale.
      setIntegration(null)
    } catch (err: any) {
      console.error(`Failed to link ${target}:`, err)
      setError(
        err?.response?.status === 503
          ? i18n.t(`${target}_not_available`)
          : i18n.t('failed_generate_link'),
      )
    } finally {
      setLoading(false)
    }
  }, [ensureValidToken])

  const fetchIntegrationDetail = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      await ensureValidToken()
      const { data } = await api.get<Integration>(`/me/integrations/${param}`)
      setIntegration(data)
    } catch (err: any) {
      console.error('Failed to fetch integration:', err)
      setError(i18n.t('failed_load_integration'))
    } finally {
      setLoading(false)
    }
  }, [param, ensureValidToken])

  useEffect(() => {
    if (isProviderName) {
      requestLinkCode(param)
    } else {
      fetchIntegrationDetail()
    }
  }, [isProviderName, param, requestLinkCode, fetchIntegrationDetail])

  // Tick the expiry countdown.
  useEffect(() => {
    if (!linkData || codeExpired) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [linkData, codeExpired])

  // Watch for the binding going active. The list endpoint filters out revoked
  // rows, so matching on provider there is unambiguous.
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  useEffect(() => {
    if (!isWaiting || !providerName) return
    const check = async () => {
      try {
        const { data } = await api.get<Integration[]>('/me/integrations')
        const found = data.find((i) => i.provider === providerName)
        if (found?.status === 'active') {
          setIntegration(found)
          setLinkData(null)
        } else if (found && !integration) {
          setIntegration(found)
        }
      } catch {
        // Transient — keep polling.
      }
    }
    pollRef.current = setInterval(check, POLL_INTERVAL_MS)
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [isWaiting, providerName, integration])

  // `waLink` carries the wa.me link for WhatsApp and the t.me link for Telegram.
  const handleOpenChannel = useCallback(async () => {
    if (linkData?.waLink) await Linking.openURL(linkData.waLink)
  }, [linkData])

  const handleSetReplyMode = useCallback(async (mode: ReplyMode) => {
    if (!integration || integration.replyMode === mode) return
    const previous = integration.replyMode
    setIntegration({ ...integration, replyMode: mode })
    setSavingMode(true)
    setError(null)
    try {
      await ensureValidToken()
      const { data } = await api.patch<Integration>(`/me/integrations/${integration.id}`, { replyMode: mode })
      trackReplyModeChanged(integration.provider, mode)
      setIntegration(data)
    } catch (err: any) {
      console.error('Failed to update reply mode:', err)
      setIntegration((curr) => (curr ? { ...curr, replyMode: previous } : curr))
      setError(i18n.t('failed_update_reply_mode'))
    } finally {
      setSavingMode(false)
    }
  }, [integration, ensureValidToken])

  const handleCancelIntegration = useCallback(async () => {
    // Fall back to the route param when we never loaded a detail (the backend
    // accepts a provider name here and does exclude revoked rows on delete).
    const target = integration?.id ?? param
    try {
      setCancelling(true)
      await ensureValidToken()
      await api.delete(`/me/integrations/${target}`)
      // Not `target` — that falls back to the integration id, and a UUID as an
      // event property is a new value on every row instead of a dimension.
      trackIntegrationUnlinked(providerName ?? 'unknown')
      router.back()
    } catch (err: any) {
      console.error('Failed to cancel integration:', err)
      setError(i18n.t('failed_cancel_integration'))
      setCancelling(false)
    }
  }, [integration, param, ensureValidToken])

  const ui = (providerName && PROVIDER_UI[providerName]) || FALLBACK_UI
  const ProviderIcon = ui.Icon

  const title = isActive
    ? i18n.t('integration_details')
    : providerName
      ? i18n.t(`connect_${providerName}`)
      : i18n.t('integration_details')

  const retry = () => (isProviderName ? requestLinkCode(param) : fetchIntegrationDetail())

  return (
    <View style={{ flex: 1 }}>
      <ScrollView style={styles.scrollArea} contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 32 }]}>
        <Reanimated.View entering={FadeInDown.duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={24} gap={16} blurTarget={blurTargetRef}>

            <View style={styles.header}>
              <View style={[styles.iconContainer, { backgroundColor: ui.tint }]}>
                <ProviderIcon size={32} color={ui.color} />
              </View>
              <Text fontSize={20} fontWeight="700" color="$onGlass" textAlign="center">
                {title}
              </Text>
              {!isActive && providerName && (
                <Text fontSize={14} color="$onGlassSecondary" textAlign="center">
                  {i18n.t(`${providerName}_integration_desc`)}
                </Text>
              )}
            </View>

            {loading ? (
              <YStack padding={32} alignItems="center" gap={16}>
                <ActivityIndicator size="large" color="#262220" />
                <Text color="$onGlassSecondary">
                  {isProviderName ? i18n.t('generating_link') : i18n.t('loading')}
                </Text>
              </YStack>
            ) : error && !integration ? (
              <YStack padding={16} alignItems="center" gap={16} backgroundColor="rgba(239,68,68,0.1)" borderRadius={12}>
                <AlertCircle size={24} color="#ef4444" />
                <Text color="$error" textAlign="center">{error}</Text>
                <ThemedButton variant="outline" onPress={retry}>
                  {i18n.t('retry')}
                </ThemedButton>
              </YStack>
            ) : isActive && integration ? (
              /* ---------- Connected ---------- */
              <YStack gap={24} marginTop={16}>
                <YStack gap={12} style={styles.panel}>
                  <XStack justifyContent="space-between" alignItems="center">
                    <Text fontSize={13} fontWeight="600" color="$onGlassSecondary" textTransform="uppercase" letterSpacing={1}>
                      {i18n.t('status')}
                    </Text>
                    <View style={[styles.connectedBadge, { backgroundColor: ui.color }]}>
                      <Text fontSize={12} fontWeight="700" color="#fff">{i18n.t('connected')}</Text>
                    </View>
                  </XStack>
                  {integration.maskedExternalId && (
                    <XStack justifyContent="space-between" alignItems="center">
                      <Text fontSize={13} fontWeight="600" color="$onGlassSecondary" textTransform="uppercase" letterSpacing={1}>
                        {i18n.t('account')}
                      </Text>
                      <Text fontSize={14} fontWeight="500" color="$onGlass">{integration.maskedExternalId}</Text>
                    </XStack>
                  )}
                  <XStack justifyContent="space-between" alignItems="center">
                    <Text fontSize={13} fontWeight="600" color="$onGlassSecondary" textTransform="uppercase" letterSpacing={1}>
                      {i18n.t('connected_since')}
                    </Text>
                    <Text fontSize={14} fontWeight="500" color="$onGlass">
                      {new Date(integration.createdAt).toLocaleDateString()}
                    </Text>
                  </XStack>
                </YStack>

                {/* Reply mode — PATCH /me/integrations/{id} */}
                <YStack gap={10}>
                  <YStack gap={2}>
                    <Text fontSize={13} fontWeight="600" color="$onGlassSecondary" textTransform="uppercase" letterSpacing={1}>
                      {i18n.t('reply_mode')}
                    </Text>
                    <Text fontSize={12} color="$onGlassSecondary">{i18n.t('reply_mode_desc')}</Text>
                  </YStack>
                  <XStack style={styles.segmented}>
                    {REPLY_MODES.map(({ mode, Icon }) => {
                      const selected = (integration.replyMode ?? 'text') === mode
                      return (
                        <TouchableOpacity
                          key={mode}
                          style={[styles.segment, selected && styles.segmentSelected]}
                          onPress={() => handleSetReplyMode(mode)}
                          disabled={savingMode}
                          activeOpacity={0.7}
                        >
                          <Icon size={16} color={selected ? '#054C38' : '#4A4540'} />
                          <Text fontSize={13} fontWeight={selected ? '700' : '500'} color={selected ? '$onGlassAccent' : '$onGlassSecondary'}>
                            {i18n.t(`reply_mode_${mode}`)}
                          </Text>
                        </TouchableOpacity>
                      )
                    })}
                  </XStack>
                  {error && <Text fontSize={12} color="$error">{error}</Text>}
                </YStack>

                <ThemedButton
                  variant="error"
                  fullWidth
                  onPress={handleCancelIntegration}
                  loading={cancelling}
                  icon={<Trash2 size={20} color="#ef4444" />}
                >
                  {i18n.t('disconnect')}
                </ThemedButton>
              </YStack>
            ) : linkData ? (
              /* ---------- Code issued, waiting for the user's message ---------- */
              <YStack gap={24} marginTop={16}>
                <YStack alignItems="center" gap={8} style={styles.panel}>
                  <Text fontSize={13} fontWeight="600" color="$onGlassSecondary" textTransform="uppercase" letterSpacing={1}>
                    {i18n.t('your_connection_code')}
                  </Text>
                  <Text fontSize={32} fontWeight="800" color="$onGlass" letterSpacing={2}>
                    {linkData.code}
                  </Text>
                  <Text fontSize={12} color={codeExpired ? '$error' : '$onGlassSecondary'} textAlign="center" marginTop={4}>
                    {codeExpired
                      ? i18n.t('code_expired')
                      : i18n.t('code_expires_in', { time: formatCountdown(new Date(linkData.expiresAt).getTime() - now) })}
                  </Text>
                </YStack>

                {codeExpired ? (
                  <ThemedButton
                    variant="solid"
                    fullWidth
                    onPress={() => providerName && requestLinkCode(providerName)}
                    icon={<RefreshCw size={20} color="#fff" />}
                  >
                    {i18n.t('get_new_code')}
                  </ThemedButton>
                ) : (
                  <YStack gap={12}>
                    <Text fontSize={12} color="$onGlassSecondary" textAlign="center">
                      {providerName ? i18n.t(`send_code_to_${providerName}`) : ''}
                    </Text>
                    <ThemedButton
                      variant="solid"
                      fullWidth
                      onPress={handleOpenChannel}
                      icon={<ExternalLink size={20} color="#fff" />}
                      style={{ backgroundColor: ui.color }}
                    >
                      {providerName ? i18n.t(`open_in_${providerName}`) : ''}
                    </ThemedButton>
                    <XStack alignItems="center" justifyContent="center" gap={8}>
                      <ActivityIndicator size="small" color="#4A4540" />
                      <Text fontSize={12} color="$onGlassSecondary">{i18n.t('waiting_for_message')}</Text>
                    </XStack>
                  </YStack>
                )}
              </YStack>
            ) : isPending && integration ? (
              /* ---------- Pending binding, but we don't hold the code ---------- */
              <YStack gap={24} marginTop={16}>
                <YStack alignItems="center" gap={8} style={styles.panel}>
                  <View style={styles.pendingBadge}>
                    <Text fontSize={12} fontWeight="700" color="#fff">{i18n.t('pending')}</Text>
                  </View>
                  <Text fontSize={13} color="$onGlassSecondary" textAlign="center" marginTop={4}>
                    {i18n.t('waiting_for_message_desc')}
                  </Text>
                </YStack>

                <ThemedButton
                  variant="solid"
                  fullWidth
                  onPress={() => providerName && requestLinkCode(providerName)}
                  icon={<RefreshCw size={20} color="#fff" />}
                >
                  {i18n.t('get_new_code')}
                </ThemedButton>

                <ThemedButton
                  variant="error"
                  fullWidth
                  onPress={handleCancelIntegration}
                  loading={cancelling}
                  icon={<Trash2 size={20} color="#ef4444" />}
                >
                  {i18n.t('cancel_integration')}
                </ThemedButton>
              </YStack>
            ) : null}

          </GlassCard>
        </Reanimated.View>
      </ScrollView>
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
  header: {
    alignItems: 'center',
    gap: 12,
    marginBottom: 8,
  },
  iconContainer: {
    width: 64,
    height: 64,
    borderRadius: 32,
    justifyContent: 'center',
    alignItems: 'center',
    marginBottom: 8,
  },
  panel: {
    backgroundColor: 'rgba(0,0,0,0.03)',
    padding: 20,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.05)',
  },
  segmented: {
    flexDirection: 'row',
    gap: 6,
    padding: 4,
    borderRadius: 14,
    backgroundColor: 'rgba(0,0,0,0.04)',
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.05)',
  },
  segment: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    paddingVertical: 10,
    borderRadius: 10,
  },
  segmentSelected: {
    backgroundColor: 'rgba(255,255,255,0.85)',
    borderWidth: 1,
    borderColor: 'rgba(5,76,56,0.18)',
  },
  connectedBadge: {
    backgroundColor: '#25D366',
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 4,
  },
  pendingBadge: {
    backgroundColor: '#F59E0B',
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 4,
  },
})
