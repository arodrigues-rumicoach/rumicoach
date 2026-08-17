import { useContext, useState } from 'react'
import { View, ScrollView, TouchableOpacity, StyleSheet } from 'react-native'
import { Text } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { router } from 'expo-router'
import { ChevronDown, ChevronRight, Play, Timer, Trash2 } from 'lucide-react-native'
import i18n from '../../src/i18n'
import { useSettings } from '../../src/hooks/useSettings'
import { useAuth } from '../../src/hooks/useAuth'
import { api } from '../../src/api/client'
import { AlertContext } from '../../src/context/AlertContext'
import { useBlurTarget } from '@/context/BlurContext'
import { GlassCard } from '@/components/atoms'
import { Toast } from '@/components/molecules'

interface SessionType {
  id: string
  titleKey: string
  fallback: string
  setups?: { id: string; label: string }[]
  // The test-setup persona used when this entry has no submenu and is tapped directly.
  // Deep-session entries default to 'coaching_session_ready' (a fully-furnished,
  // ready-to-go profile) further down — but the onboarding intro needs the OPPOSITE: a
  // freshly-reset account. Without this override it fell through to the shared default
  // and started the intro on top of a full check-in profile (state=CHECKIN), which the
  // onboarding session doesn't recognize and silently served the daily check-in prompt
  // instead of the intro.
  defaultSetup?: string
}

const SESSION_TYPES: SessionType[] = [
  {
    id: 'onboarding',
    titleKey: 'session_onboarding_title',
    fallback: 'Onboarding intro',
    defaultSetup: 'new_user',
  },
  {
    id: 'session_vision',
    titleKey: 'session_vision_title',
    fallback: 'Ideal Life Vision',
    setups: [
      { id: 'onboarding_ideal_vision', label: 'Ideal Life Vision Step' },
      { id: 'onboarding_wheel', label: 'Wheel of Life Step' },
      { id: 'onboarding_metaphor', label: 'Metaphor Step' },
      { id: 'onboarding_ending', label: 'Ending' },
    ],
  },
  {
    id: 'checkin',
    titleKey: 'session_checkin_title',
    fallback: 'Check-in',
    setups: [
      { id: 'checkin_daily_first', label: 'First Check-in' },
      { id: 'checkin_daily_same_day', label: 'Same Day Check-in' },
      { id: 'checkin_daily_consecutive', label: 'Consecutive Check-in' },
      { id: 'checkin_daily_after_days', label: 'After a few Days' },
      { id: 'checkin_daily_after_weeks', label: 'After a few Weeks' },
      { id: 'checkin_daily_after_long', label: 'After a long time' },
      { id: 'stressed', label: 'Stressed' },
    ],
  },
  { id: 'session_movement', titleKey: 'session_movement_title', fallback: 'Movement Coaching' },
  { id: 'session_values', titleKey: 'session_values_title', fallback: 'Core Values Exploration' },
  { id: 'session_energy', titleKey: 'session_energy_title', fallback: 'Energy Coaching' },
  { id: 'session_decisions', titleKey: 'session_decisions_title', fallback: 'Decisions Coaching' },
  { id: 'session_beliefs', titleKey: 'session_beliefs_title', fallback: 'Limiting Beliefs Coaching' },
  { id: 'session_identity', titleKey: 'session_identity_title', fallback: 'Identity Coaching' },
  { id: 'session_acceptance', titleKey: 'session_acceptance_title', fallback: 'Acceptance Coaching' },
]

export default function AdminScreen() {
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const { colorScheme } = useSettings()
  const { user, ensureValidToken, refreshUser } = useAuth()
  const { showConfirm } = useContext(AlertContext)!

  const [expandedSession, setExpandedSession] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [toast, setToast] = useState<string | null>(null)

  const handleStartSession = async (sessionType: string, testSetupType: string) => {
    setLoading(true)
    setError(null)
    try {
      await ensureValidToken()
      await api.post('/admin/test-setup', { sessionType: testSetupType })
      router.replace({ pathname: '/(tabs)/session', params: { autoStart: 'true', sessionType } })
    } catch (e) {
      setError((e as Error).message || 'Failed to set up test session')
    } finally {
      setLoading(false)
    }
  }

  // Credits the caller's own account through the same manual-credit endpoint the
  // backoffice uses, so the grant goes through the balance ledger like a real top-up.
  const handleAddMinutes = async () => {
    if (!user) return
    setLoading(true)
    setError(null)
    try {
      await ensureValidToken()
      await api.post(`/admin/users/${user.id}/credits`, {
        amountMinutes: 60,
        type: 'top_up',
        product: 'admin_test_60',
        description: 'Admin menu test credit',
      })
      await refreshUser()
      setToast('Added 60 minutes to your balance.')
    } catch (e) {
      setError((e as Error).message || 'Failed to add minutes')
    } finally {
      setLoading(false)
    }
  }

  // Not a persona: the personas above reset state and seed a starting point, but they
  // deliberately leave real session history, the balance and the chat binding alone. This
  // is the sign-up-again case — /admin/reset-account puts the account back to the row it
  // had the moment it was created, so the journey, the history list, the usage calendar
  // and the free-session allowance all start from nothing.
  //
  // It stays on this screen rather than auto-starting a session, so the next persona is
  // one tap away; refreshUser() is what carries the fresh state (state, balance, cleared
  // profile details) into the rest of the app.
  const handleResetAccount = () => {
    showConfirm({
      title: 'Reset account',
      message: 'Puts this account back to a brand-new one: all session history, progress, memories, commitments and minutes are deleted, and any WhatsApp/Telegram link is removed. Test accounts only.',
      confirmLabel: 'Reset',
      destructive: true,
      onConfirm: async () => {
        setLoading(true)
        setError(null)
        try {
          await ensureValidToken()
          await api.post('/admin/reset-account')
          await refreshUser()
          setToast('Account reset. It is now a brand-new account.')
        } catch (e) {
          setError((e as Error).message || 'Failed to reset account')
        } finally {
          setLoading(false)
        }
      },
    })
  }

  return (
    <ScrollView style={styles.scrollArea} contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 32 }]}>
      {error && (
        <View style={styles.errorBanner}>
          <Text style={styles.errorText}>{error}</Text>
        </View>
      )}

      <GlassCard variant="light" borderRadius={18} padding={16} gap={10} blurTarget={blurTargetRef} style={styles.resetCard}>
        <TouchableOpacity
          style={styles.menuItem}
          onPress={handleAddMinutes}
          activeOpacity={0.7}
          disabled={loading}
        >
          <View style={[styles.menuIcon, { backgroundColor: 'rgba(16,185,129,0.15)' }]}>
            <Timer size={20} color="#10b981" />
          </View>
          <View style={styles.rowText}>
            <Text fontSize={15} fontWeight="500" color="$onGlass">Add 60 minutes</Text>
            <Text fontSize={12} color="$onGlassSecondary" lineHeight={16}>
              Credits one hour to this account's balance for testing
            </Text>
          </View>
        </TouchableOpacity>
        <View style={styles.separator} />
        <TouchableOpacity
          style={styles.menuItem}
          onPress={handleResetAccount}
          activeOpacity={0.7}
          disabled={loading}
        >
          <View style={[styles.menuIcon, { backgroundColor: 'rgba(239,68,68,0.15)' }]}>
            <Trash2 size={20} color="#ef4444" />
          </View>
          <View style={styles.rowText}>
            <Text fontSize={15} fontWeight="500" color="$error">Reset account</Text>
            <Text fontSize={12} color="$onGlassSecondary" lineHeight={16}>
              Deletes history, progress, minutes and links — like a brand-new signup
            </Text>
          </View>
        </TouchableOpacity>
      </GlassCard>

      <GlassCard variant="light" borderRadius={18} padding={16} gap={10} blurTarget={blurTargetRef}>
        {SESSION_TYPES.map((session, index) => {
          const isExpanded = expandedSession === session.id
          const hasSetups = !!session.setups?.length

          return (
            <View key={session.id}>
              {index > 0 && <View style={styles.separator} />}
              <TouchableOpacity
                style={[styles.menuItem, isExpanded && styles.menuItemExpanded]}
                onPress={() => {
                  if (hasSetups) {
                    setExpandedSession(isExpanded ? null : session.id)
                  } else {
                    handleStartSession(session.id, session.defaultSetup ?? 'coaching_session_ready')
                  }
                }}
                activeOpacity={0.7}
                disabled={loading}
              >
                <View style={[styles.menuIcon, { backgroundColor: 'rgba(0,0,0,0.06)' }]}>
                  {hasSetups ? (
                    isExpanded ? (
                      <ChevronDown size={20} color="#262220" />
                    ) : (
                      <ChevronRight size={20} color="#262220" />
                    )
                  ) : (
                    <Play size={20} color="#262220" />
                  )}
                </View>
                <Text fontSize={15} fontWeight="500" color="$onGlass" flex={1}>
                  {i18n.t(session.titleKey) || session.fallback}
                </Text>
              </TouchableOpacity>

              {isExpanded && session.setups && (
                <View style={styles.subList}>
                  {session.setups.map((setup) => (
                    <TouchableOpacity
                      key={setup.id}
                      style={styles.subItem}
                      onPress={() => handleStartSession(session.id, setup.id)}
                      activeOpacity={0.7}
                      disabled={loading}
                    >
                      <Play size={14} color="#262220" />
                      <Text fontSize={14} color="$onGlassSecondary">{setup.label}</Text>
                    </TouchableOpacity>
                  ))}
                </View>
              )}
            </View>
          )
        })}
      </GlassCard>

      {toast && <Toast message={toast} type="success" onClose={() => setToast(null)} />}
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
  menuItem: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: 12,
    gap: 12,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: 'transparent',
  },
  menuItemExpanded: {
    borderColor: 'rgba(16,185,129,0.40)',
    backgroundColor: 'rgba(16,185,129,0.08)',
    borderWidth: 2,
  },
  resetCard: {
    marginBottom: 12,
  },
  rowText: {
    flex: 1,
    gap: 2,
  },
  menuIcon: {
    width: 36,
    height: 36,
    borderRadius: 18,
    justifyContent: 'center',
    alignItems: 'center',
  },
  subList: {
    paddingLeft: 48,
    paddingTop: 4,
    paddingBottom: 8,
    paddingRight: 14,
    gap: 4,
  },
  subItem: {
    flexDirection: 'row',
    alignItems: 'center',
    padding: 10,
    borderRadius: 8,
    backgroundColor: 'rgba(0,0,0,0.03)',
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.08)',
    gap: 10,
  },
  errorBanner: {
    backgroundColor: 'rgba(239,68,68,0.15)',
    borderWidth: 1,
    borderColor: 'rgba(239,68,68,0.3)',
    borderRadius: 8,
    padding: 12,
    marginBottom: 12,
  },
  errorText: {
    color: '#ef4444',
    fontSize: 13,
  },
})
