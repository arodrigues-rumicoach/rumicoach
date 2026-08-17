import { useCallback, useEffect, useState } from 'react'
import { View, ScrollView, TouchableOpacity, StyleSheet, ActivityIndicator, Linking, Platform } from 'react-native'
import { Text, XStack, YStack } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { useRouter } from 'expo-router'
import { MessageCircle, Mic, Plus, Undo2 } from 'lucide-react-native'
import { Toast } from '@/components/molecules'
import i18n from '../../src/i18n'
import { useBlurTarget } from '@/context/BlurContext'
import { GlassCard, ThemedButton } from '@/components/atoms'
import { useRevenueCat } from '../../src/hooks/useRevenueCat'
import { useSettings } from '../../src/hooks/useSettings'
import { hasActiveMembership, membershipDetails, minutesRemaining } from '../../src/subscriptions/gate'
import { billingPeriodFromProductId } from '../../src/subscriptions/catalog'
import { useAuth } from '../../src/hooks/useAuth'
import { api } from '@/api'
import { GLASS, INK } from '@/styles/glass'
import type { BalanceTransaction, BalanceTransactionPaginatedResponse } from '@/api'

/** How many statement rows the screen shows. The list is the full balance
 *  history, so the cap is generous — it exists only to bound the payload. */
const HISTORY_LIMIT = 30

/** Below this many minutes the balance renders as a warning, not just at zero —
 *  a session runs a few minutes, so single digits mean "one short session left".
 *  Display-only; whether a session may START stays the server's decision. */
const LOW_BALANCE_MINUTES = 10

/** Where "Manage subscription" goes when RevenueCat's per-customer
 *  managementURL is missing (sandbox receipts and some web-billing states
 *  return null). iOS/Android fall back to the store's own subscriptions page —
 *  always right for a purchase made on that platform. Web falls back to
 *  support mail, which is the cancellation path the website's FAQ and terms
 *  already promise ("or email support@rumi.coach") — a member must never be
 *  left without a way to cancel. */
const FALLBACK_MANAGEMENT_URL = Platform.select<string>({
  ios: 'https://apps.apple.com/account/subscriptions',
  android: 'https://play.google.com/store/account/subscriptions',
  default: 'mailto:support@rumi.coach?subject=Manage%20my%20subscription',
})!

function SectionLabel({ children }: { children: string }) {
  return (
    <Text fontSize={12} fontWeight="700" letterSpacing={0.5} color="$onGlassSecondary" textTransform="uppercase">
      {children}
    </Text>
  )
}

/** "Checkin Daily" from "checkin_daily" — same rendering the /sessions cards use. */
function formatSessionType(type: string): string {
  return type.split('_').map(word => word.charAt(0).toUpperCase() + word.slice(1)).join(' ')
}

function txLabel(tx: BalanceTransaction): string {
  switch (tx.type) {
    // The server names the cadence (tx.plan); the slug-sniffing fallback only
    // covers servers that predate the field. tx.product itself is never shown —
    // it is the store's package slug ("membership_monthly"), not copy.
    case 'subscription': {
      const period = tx.plan ?? billingPeriodFromProductId(tx.product ?? '')
      if (period === 'monthly') return i18n.t('tx_subscription_monthly') || 'Monthly subscription'
      if (period === 'annual') return i18n.t('tx_subscription_annual') || 'Annual subscription'
      return i18n.t('tx_subscription') || 'Subscription'
    }
    case 'top_up':
      return i18n.t('tx_top_up') || 'Minutes top-up'
    case 'refund':
      return i18n.t('tx_refund') || 'Refund'
    case 'message_usage':
      return i18n.t('usage_messages_count', { count: tx.messageCount ?? 0 }) || `${tx.messageCount ?? 0} messages`
    default:
      return tx.sessionType ? formatSessionType(tx.sessionType) : (i18n.t('session_history') || 'Session')
  }
}

function txIcon(tx: BalanceTransaction) {
  switch (tx.type) {
    case 'subscription':
    case 'top_up':
      return Plus
    case 'refund':
      return Undo2
    case 'message_usage':
      return MessageCircle
    default:
      return Mic
  }
}

/** The hero balance: "55 min" under an hour, "2h 35m" from there up. No
 *  seconds on purpose — the hero is for sizing up the balance at a glance,
 *  and exact amounts live in the statement below. */
function heroBalance(minutes: number): string {
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  if (h > 0) return m > 0 ? `${h}h ${m}m` : `${h}h`
  return `${m} min`
}

/** "2m 30s", "45s", "1h 5m" — durations come from the server in seconds. */
function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.round(seconds))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h > 0) return m > 0 ? `${h}h ${m}m` : `${h}h`
  if (m > 0) return s > 0 ? `${m}m ${s}s` : `${m}m`
  return `${s}s`
}

/** The IANA timezone name, so the server groups message days on the user's
 *  calendar rather than UTC. Same convention as /journey. */
function deviceTimezone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined
  } catch {
    return undefined
  }
}

export default function SettingsUsageScreen() {
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const router = useRouter()
  const { colorScheme } = useSettings()
  const { customerInfo, isLoading: rcLoading, restorePurchases } = useRevenueCat()
  const { user, refreshUser } = useAuth()

  const [isRestoring, setIsRestoring] = useState(false)
  const [message, setMessage] = useState<{ text: string; type: 'success' | 'error' } | null>(null)
  const [statement, setStatement] = useState<BalanceTransaction[] | null>(null)

  const isMember = hasActiveMembership(customerInfo)
  const balance = minutesRemaining(user)

  // A session just ended, or a top-up just landed, and either way the cached balance is
  // stale — this screen is where someone comes to check it, so refetch on open.
  useEffect(() => { refreshUser().catch(() => { }) }, [refreshUser])

  // The statement is decoration around the balance: a failure (including a
  // backend that has not shipped grouped /me/transactions yet) hides the
  // section rather than blocking the screen — the purchase and restore paths
  // must stay usable regardless.
  const loadStatement = useCallback(async () => {
    try {
      const timezone = deviceTimezone()
      const { data } = await api.get<BalanceTransactionPaginatedResponse>(
        `/me/transactions?grouped=true&limit=${HISTORY_LIMIT}`,
        { headers: timezone ? { 'X-Timezone': timezone } : undefined },
      )
      setStatement(data.items)
    } catch (e) {
      if (__DEV__) console.error('[Usage] /me/transactions fetch failed:', e)
    }
  }, [])
  useEffect(() => { loadStatement() }, [loadStatement])

  // No progress bar on purpose: unused minutes roll over and top-ups stack, so the
  // balance has no denominator to be measured against — a bar against the monthly
  // allowance lied whenever the balance exceeded it. The wallet is the number plus,
  // for members, what the store receipt says: plan, renewal, where to manage it.
  const membership = isMember ? membershipDetails(customerInfo) : null
  const isLow = balance !== null && balance <= LOW_BALANCE_MINUTES

  const planLabel = membership?.plan === 'monthly'
    ? (i18n.t('membership_monthly') || 'Monthly plan')
    : membership?.plan === 'annual'
      ? (i18n.t('membership_annual') || 'Annual plan')
      : (i18n.t('membership_member') || 'Member')

  // Purchasing happens on the RevenueCat paywalls, not here. They carry the price, the
  // billing period, the terms and the Terms/Privacy links App Review requires under
  // guideline 3.1.2 — none of which this settings screen showed when it sold directly.
  //
  // No platform check: web reaches the same paywalls through Paddle. The screen used to
  // refuse here, from back when the web bundle had no paywall to send anyone to.
  const openPaywall = () => {
    router.push(isMember ? '/paywall?mode=topup' : '/paywall')
  }

  const handleRestore = async () => {
    try {
      setIsRestoring(true)
      await restorePurchases()
      setMessage({ text: i18n.t('restore_success') || 'Purchases restored!', type: 'success' })
    } catch (e: any) {
      if (__DEV__) console.error('[RevenueCat] Restore failed:', e)
      setMessage({
        text: i18n.t('restore_error') || 'Could not restore purchases.',
        type: 'error',
      })
    } finally {
      setIsRestoring(false)
    }
  }

  const formatDate = (dateStr: string) => {
    try {
      return new Date(dateStr).toLocaleDateString(user?.preferredLanguage || undefined, {
        month: 'short', day: 'numeric', year: 'numeric',
      })
    } catch {
      return dateStr
    }
  }

  return (
    <View style={{ flex: 1 }}>
      {message && (
        <Toast
          message={message.text}
          type={message.type}
          onClose={() => setMessage(null)}
        />
      )}
      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 32 }]}
      >
        {/* The wallet: subscription status, balance and the one action that matters,
            together — "can I talk to Rumi, and if not, what do I do about it" is
            answered without leaving this card. */}
        <GlassCard variant="light" borderRadius={GLASS.radius.card} padding={20} gap={14} blurTarget={blurTargetRef}>
          {/* Two lines, no chrome: the balance as one readable phrase, then one
              status sentence beneath it. The old header row and status pill are
              gone — "remaining" says what the number is, and the status line is
              where the eye lands next anyway. */}
          {balance !== null && (
            <YStack gap={4}>
              <XStack alignItems="baseline" gap={8} flexWrap="wrap">
                <Text fontSize={42} fontWeight="700" lineHeight={48} color={isLow ? INK.amber : '$onGlass'}>
                  {heroBalance(balance)}
                </Text>
                <Text fontSize={15} color="$onGlassSecondary">
                  {i18n.t('remaining') || 'remaining'}
                </Text>
              </XStack>
              {isMember ? (
                <Text fontSize={13} lineHeight={19}>
                  {/* A member running low still sees plan and renewal — that is the
                      exact moment "when do I get more?" matters most. */}
                  {isLow && (
                    <Text fontSize={13} color={INK.amber}>
                      {(balance === 0
                        ? (i18n.t('balance_empty_hint') || "You're out of minutes. Get more to keep talking with Rumi.")
                        : (i18n.t('balance_low_hint') || "You're running low on minutes.")) + ' '}
                    </Text>
                  )}
                  <Text fontSize={13} fontWeight="600" color={INK.accent}>{planLabel}</Text>
                  {membership?.renewal && (
                    <Text fontSize={13} color="$onGlassSecondary">
                      {' · '}
                      {membership.renewal.willRenew
                        ? (i18n.t('renews_on', { date: formatDate(membership.renewal.date.toISOString()) }) || `Renews ${formatDate(membership.renewal.date.toISOString())}`)
                        : (i18n.t('expires_on', { date: formatDate(membership.renewal.date.toISOString()) }) || `Expires ${formatDate(membership.renewal.date.toISOString())}`)}
                    </Text>
                  )}
                </Text>
              ) : (
                <Text fontSize={13} lineHeight={19} color="$onGlassSecondary">
                  {i18n.t('membership_none') || 'No active subscription'}
                </Text>
              )}
            </YStack>
          )}

          <YStack gap={8} marginTop={2}>
            {/* Solid only when there is something to sell: a member already pays, so
                the top-up stays available without shouting at them. No caption either
                way — the button says what it does, and the plans' contents belong to
                the paywall it opens. */}
            <ThemedButton variant={isMember ? 'outline' : 'solid'} fullWidth onPress={openPaywall}>
              {isMember
                ? (i18n.t('top_up_minutes') || 'Top up minutes')
                : (i18n.t('subscribe') || 'Subscribe')}
            </ThemedButton>
            {/* Cancel, switch cadence, update payment — all of it belongs to whichever
                store owns the subscription, so this opens RevenueCat's managementURL
                (App Store, Play, or the web billing portal) and falls back per
                platform when the receipt did not carry one. Always present for a
                member: the terms promise a way to cancel from here. */}
            {isMember && (
              <ThemedButton
                variant="outline"
                fullWidth
                onPress={() => {
                  Linking.openURL(membership?.managementURL || FALLBACK_MANAGEMENT_URL).catch(() => { })
                }}
              >
                {i18n.t('manage_subscription') || 'Manage subscription'}
              </ThemedButton>
            )}
            {/* Restore lives here as a quiet link rather than its own card — it matters
                to two audiences only (a new device, and App Review, which just needs a
                findable control), and a full card gave it the wallet's visual weight.
                Not shown to members: it recovers a subscription this device doesn't
                know about, and a recognized member has nothing to restore. */}
            {!isMember && (
              <TouchableOpacity onPress={handleRestore} activeOpacity={0.7} disabled={isRestoring}>
                <XStack alignItems="center" justifyContent="center" gap={6} paddingVertical={4}>
                  {isRestoring && <ActivityIndicator size="small" color={INK.secondary} />}
                  <Text fontSize={12} color="$onGlassSecondary" textDecorationLine="underline">
                    {i18n.t('restore_purchases_detail') || 'Already subscribed on another device? Restore it here.'}
                  </Text>
                </XStack>
              </TouchableOpacity>
            )}
          </YStack>
        </GlassCard>

        {/* The statement: the balance ledger rendered like a bank statement —
            signed credits and debits with the balance after each row, all
            server-computed. A free intro session is a row that moved nothing,
            which is what makes it self-explanatory. Hidden until
            /me/transactions answers. */}
        {statement !== null && statement.length > 0 && (
          <GlassCard variant="light" borderRadius={GLASS.radius.card} padding={16} gap={4} blurTarget={blurTargetRef} style={{ marginTop: 16 }}>
            <SectionLabel>{i18n.t('statement_title') || 'Balance Activity'}</SectionLabel>
            {statement.map((tx, index) => {
              const Icon = txIcon(tx)
              const isCredit = tx.amountSeconds > 0
              const isFree = tx.type === 'session_free'
              const isMessages = tx.type === 'message_usage'
              return (
                <View key={tx.id}>
                  {index > 0 && <View style={styles.separator} />}
                  <XStack alignItems="center" paddingVertical={10} gap={12}>
                    <View style={[styles.iconChip, isMessages && { backgroundColor: colorScheme.secondary }]}>
                      <Icon size={15} color={isMessages ? colorScheme.tertiary : isCredit ? INK.accent : INK.secondary} />
                    </View>
                    <YStack flex={1} gap={2}>
                      <Text fontSize={14} fontWeight="500" color="$onGlass" flexShrink={1}>{txLabel(tx)}</Text>
                      <Text fontSize={12} color="$onGlassTertiary">{formatDate(tx.createdAt)}</Text>
                    </YStack>
                    <YStack alignItems="flex-end" gap={2}>
                      {isFree ? (
                        <Text fontSize={14} fontWeight="600" color={INK.accent}>
                          {i18n.t('usage_free') || 'Free'}
                        </Text>
                      ) : (
                        <Text fontSize={14} fontWeight="600" color={isCredit ? INK.accent : '$onGlassSecondary'}>
                          {isCredit ? '+' : '−'}{formatDuration(Math.abs(tx.amountSeconds))}
                        </Text>
                      )}
                      <Text fontSize={11} color="$onGlassTertiary">
                        {/* A refund can overdraw the balance; formatDuration clamps at zero,
                            so the sign has to survive on its own. */}
                        {(i18n.t('statement_balance') || 'Balance')} {tx.balanceAfter < 0 ? '−' : ''}{formatDuration(Math.abs(tx.balanceAfter))}
                      </Text>
                    </YStack>
                  </XStack>
                </View>
              )
            })}
          </GlassCard>
        )}

        {rcLoading && (
          <View style={styles.loadingRow}>
            <ActivityIndicator size="small" color={INK.secondary} />
          </View>
        )}
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
    gap: 0,
  },
  iconChip: {
    width: 32,
    height: 32,
    borderRadius: GLASS.radius.row - 4,
    backgroundColor: 'rgba(0,0,0,0.06)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  separator: {
    height: 1,
    backgroundColor: GLASS.separator,
  },
  loadingRow: {
    paddingTop: 16,
    alignItems: 'center',
  },
})
