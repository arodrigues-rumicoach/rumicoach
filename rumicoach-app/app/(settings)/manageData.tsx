import { useState, useContext } from 'react'
import { View, ScrollView, TouchableOpacity, StyleSheet, Platform } from 'react-native'
import { Text, XStack, YStack } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { router } from 'expo-router'
import { Brain, Download, Eraser, Flame, ListChecks, Mail, MessageCircle } from 'lucide-react-native'
// expo-file-system 56 moved the classic API behind /legacy: `documentDirectory`
// is gone from the main entrypoint, so importing it from there left the URI as
// "undefined/rumi_data.json" and every native export failed into the catch.
import * as FileSystem from 'expo-file-system/legacy'
import * as Sharing from 'expo-sharing'
import i18n from '../../src/i18n'
import { useAuth } from '../../src/hooks/useAuth'
import { AlertContext } from '../../src/context/AlertContext'
import { useBlurTarget } from '@/context/BlurContext'
import { GlassCard } from '@/components/atoms'
import { Toast } from '@/components/molecules'
import { api, CHAT_RETENTION_OPTIONS, type DataScope } from '../../src/api'
import Reanimated, { FadeInDown } from 'react-native-reanimated'
import { trackChatRetentionChanged, trackDataExportRequested } from '@/analytics'

function SectionLabel({ children }: { children: string }) {
  return (
    <Text fontSize={12} fontWeight="700" letterSpacing={0.5} color="$onGlassSecondary" textTransform="uppercase">
      {children}
    </Text>
  )
}

/** One row: icon disc, label, a line saying what it actually does. These are
 *  irreversible, and a bare label is what made "Delete Memories" quietly wipe
 *  a user's whole journey — the consequence line is the point, not decoration. */
function DataRow({
  icon, label, detail, onPress, danger,
}: {
  icon: React.ReactNode
  label: string
  detail: string
  onPress: () => void
  danger?: boolean
}) {
  return (
    <TouchableOpacity style={styles.menuItem} onPress={onPress} activeOpacity={0.7}>
      <View style={[styles.menuIcon, { backgroundColor: danger ? 'rgba(239,68,68,0.15)' : 'rgba(0,0,0,0.06)' }]}>
        {icon}
      </View>
      <View style={styles.rowText}>
        <Text fontSize={15} fontWeight="500" color={danger ? '$error' : '$onGlass'}>
          {label}
        </Text>
        <Text fontSize={12} color="$onGlassSecondary" lineHeight={16}>
          {detail}
        </Text>
      </View>
    </TouchableOpacity>
  )
}

export default function ManageDataScreen() {
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const { user, updateUser, ensureValidToken, deleteUserData } = useAuth()
  const { showConfirm } = useContext(AlertContext)!
  const [message, setMessage] = useState<{ text: string; type: 'success' | 'error' } | null>(null)
  const [loading, setLoading] = useState(false)
  const [savingRetention, setSavingRetention] = useState(false)

  const handleDownloadData = async () => {
    try {
      const response = await api.get('/me/data')
      const dataStr = JSON.stringify(response.data, null, 2)

      if (Platform.OS === 'web') {
        const blob = new Blob([dataStr], { type: 'application/json' })
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = 'rumi_data.json'
        a.click()
        URL.revokeObjectURL(url)
      } else {
        const fileUri = FileSystem.documentDirectory + 'rumi_data.json'
        await FileSystem.writeAsStringAsync(fileUri, dataStr, { encoding: FileSystem.EncodingType.UTF8 })
        await Sharing.shareAsync(fileUri)
      }
      trackDataExportRequested('download')
      setMessage({ text: i18n.t('data_downloaded') || 'Your data is ready', type: 'success' })
    } catch {
      setMessage({ text: i18n.t('failed_download') || 'Failed to download data', type: 'error' })
    }
  }

  const retention = user?.chatHistoryRetentionDays ?? 7

  const applyRetention = async (days: number) => {
    setSavingRetention(true)
    try {
      await ensureValidToken()
      await updateUser({ chatHistoryRetentionDays: days })
      trackChatRetentionChanged(days)
      setMessage({ text: i18n.t('data_retention_saved') || 'Saved.', type: 'success' })
    } catch {
      setMessage({ text: i18n.t('failed_update') || 'Failed to update', type: 'error' })
    } finally {
      setSavingRetention(false)
    }
  }

  // Changing this rewrites the expiry of messages already stored, so shortening
  // the window deletes the older ones straight away. Raising it, or picking "no
  // limit", only ever keeps more — nothing to warn about there.
  const handleRetentionChange = (days: number) => {
    if (days === retention) return
    const shortens = days !== 0 && (retention === 0 || days < retention)
    if (!shortens) {
      applyRetention(days)
      return
    }
    showConfirm({
      title: i18n.t('data_retention_label') || 'Keep my chat with Rumi',
      message: i18n.t('data_retention_shorten_warning', { count: days })
        || `Messages older than ${days} days will be deleted now. This cannot be undone.`,
      confirmLabel: i18n.t('data_retention_confirm') || 'Change',
      destructive: true,
      onConfirm: () => applyRetention(days),
    })
  }

  // POST, not a GET flag: mailing a copy is a side effect, and `GET /me/data` has
  // to stay safe to retry — a browser prefetch or an axios retry must not put the
  // user's whole export in their inbox twice.
  const handleEmailData = async () => {
    if (!user?.email) {
      // Nothing to send to. Point at the one screen that can fix it rather than
      // failing with an error the user can do nothing about.
      setMessage({ text: i18n.t('data_email_needs_address') || 'Add an email address to use this.', type: 'error' })
      router.push('/(settings)/manageAccount')
      return
    }
    try {
      await ensureValidToken()
      await api.post('/me/data/export')
      trackDataExportRequested('email')
      setMessage({ text: i18n.t('data_email_sent') || 'On its way. Check your inbox.', type: 'success' })
    } catch {
      setMessage({ text: i18n.t('failed_download') || 'Failed to download data', type: 'error' })
    }
  }

  // One confirm-then-delete path for every scope. The server runs each as a
  // single transaction that also repairs what it invalidates — badges are never
  // revoked by the award service, the streak is derived only from sessions, and
  // habit plans point at commitments — so the app just names the scope.
  // See rumicoach-backend/docs/delete-by-category.md.
  const confirmDelete = (scope: DataScope, title: string, warning: string, confirmLabel: string) => {
    showConfirm({
      title,
      message: warning,
      confirmLabel,
      destructive: true,
      onConfirm: async () => {
        setLoading(true)
        try {
          await ensureValidToken()
          await deleteUserData(scope)
          setMessage({ text: i18n.t('data_deleted') || 'Done. The selected data has been deleted.', type: 'success' })
        } catch {
          setMessage({ text: i18n.t('failed_update') || 'Failed to delete', type: 'error' })
        } finally {
          setLoading(false)
        }
      },
    })
  }

  // Ordered by how much of the journey each one costs, so the reach for the
  // cheapest is the shortest and "everything" sits at the bottom.
  const scopes: {
    scope: DataScope
    icon: React.ReactNode
    label: string
    detail: string
    warning: string
  }[] = [
      {
        scope: 'memories',
        icon: <Brain size={20} color="#ef4444" />,
        label: i18n.t('data_delete_memories') || 'Memories',
        detail: i18n.t('data_delete_memories_detail') || 'Rumi forgets what you told it. Your sessions stay.',
        warning: i18n.t('data_delete_memories_warning') || 'Rumi will permanently forget everything it learned about you. Your sessions, commitments and progress stay. This cannot be undone.',
      },
      {
        scope: 'chat',
        icon: <MessageCircle size={20} color="#ef4444" />,
        label: i18n.t('data_delete_chat') || 'Chat with Rumi',
        detail: i18n.t('data_delete_chat_detail') || 'Your WhatsApp and Telegram messages. You stay connected.',
        warning: i18n.t('data_delete_chat_warning') || 'This permanently deletes your WhatsApp and Telegram messages. Your channels stay connected and everything else is untouched. This cannot be undone.',
      },
      {
        scope: 'commitments',
        icon: <ListChecks size={20} color="#ef4444" />,
        label: i18n.t('data_delete_commitments') || 'Commitments',
        detail: i18n.t('data_delete_commitments_detail') || 'Your commitments and habit plans, kept or not.',
        warning: i18n.t('data_delete_commitments_warning') || 'This permanently deletes your commitments, their history and any habit plans built on them. Your sessions and memories stay. This cannot be undone.',
      },
      {
        scope: 'progress',
        icon: <Flame size={20} color="#ef4444" />,
        label: i18n.t('data_delete_progress') || 'Progress',
        detail: i18n.t('data_delete_progress_detail') || 'Sessions, streak and badges — your journey starts over.',
        warning: i18n.t('data_delete_progress_warning') || 'This permanently deletes your sessions, streak and badges, and your journey starts from the beginning. Your memories and commitments stay. This cannot be undone.',
      },
      {
        scope: 'all',
        icon: <Eraser size={20} color="#ef4444" />,
        label: i18n.t('data_delete_all') || 'Delete Everything',
        detail: i18n.t('data_delete_all_detail') || 'Your sessions, commitments, vision and badges. Your account, balance and connected channels stay.',
        warning: i18n.t('delete_memories_warning') || 'This permanently deletes your sessions, memories, commitments, vision, wheel of life and badges. This cannot be undone.',
      },
    ]

  return (
    <View style={{ flex: 1 }}>
      {message && (
        <Toast message={message.text} type={message.type} onClose={() => setMessage(null)} />
      )}
      <ScrollView style={styles.scrollArea} contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 32 }]}>
        <Reanimated.View entering={FadeInDown.duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={0} blurTarget={blurTargetRef}>
            <SectionLabel>{i18n.t('data_get_copy') || 'Get a copy'}</SectionLabel>

            {/* Email first: on a phone the download ends in a share sheet, which
                is a clumsy way to keep a file. Email is where it can actually
                live. */}
            <DataRow
              icon={<Mail size={20} color="#262220" />}
              label={i18n.t('data_email_copy') || 'Email me my data'}
              detail={user?.email
                ? (i18n.t('data_email_copy_detail', { email: user.email }) || `We'll send a copy to ${user.email}.`)
                : (i18n.t('data_email_needs_address') || 'Add an email address to use this.')}
              onPress={handleEmailData}
            />

            <View style={styles.separator} />

            <DataRow
              icon={<Download size={20} color="#262220" />}
              label={i18n.t('download_data') || 'Download My Data'}
              detail={i18n.t('download_data_detail') || 'A file with everything we hold about you.'}
              onPress={handleDownloadData}
            />
          </GlassCard>
        </Reanimated.View>

        {/* A setting, not an action — kept out of the DELETE card below, where
            everything happens the moment you tap it. */}
        <Reanimated.View entering={FadeInDown.delay(50).duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={10} blurTarget={blurTargetRef} style={styles.card}>
            <SectionLabel>{i18n.t('data_retention') || 'Automatic deletion'}</SectionLabel>

            <YStack gap={2}>
              <Text fontSize={15} fontWeight="500" color="$onGlass">
                {i18n.t('data_retention_label') || 'Keep my chat with Rumi'}
              </Text>
              {/* The 0 case is spelled out rather than left to read as just
                  another duration: it is the one choice that keeps everything. */}
              <Text fontSize={12} color="$onGlassSecondary" lineHeight={16}>
                {retention === 0
                  ? (i18n.t('data_retention_detail_forever') || 'Messages are kept indefinitely.')
                  : (i18n.t('data_retention_detail_days', { count: retention }) || `Messages older than ${retention} days are deleted automatically.`)}
              </Text>
            </YStack>

            <XStack style={styles.segmented}>
              {CHAT_RETENTION_OPTIONS.map((days) => {
                const selected = retention === days
                return (
                  <TouchableOpacity
                    key={days}
                    style={[styles.segment, selected && styles.segmentSelected]}
                    onPress={() => handleRetentionChange(days)}
                    disabled={savingRetention}
                    activeOpacity={0.7}
                  >
                    <Text fontSize={13} fontWeight={selected ? '700' : '500'} color={selected ? '$onGlassAccent' : '$onGlassSecondary'}>
                      {days === 0
                        ? (i18n.t('data_retention_forever') || 'No limit')
                        : (i18n.t('data_retention_days', { count: days }) || `${days} days`)}
                    </Text>
                  </TouchableOpacity>
                )
              })}
            </XStack>
          </GlassCard>
        </Reanimated.View>

        <Reanimated.View entering={FadeInDown.delay(100).duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={0} blurTarget={blurTargetRef} style={styles.card}>
            <SectionLabel>{i18n.t('data_delete') || 'Delete'}</SectionLabel>

            {scopes.map((s, i) => (
              <View key={s.scope}>
                {i > 0 && <View style={styles.separator} />}
                <DataRow
                  icon={s.icon}
                  label={s.label}
                  detail={s.detail}
                  onPress={() => confirmDelete(s.scope, s.label, s.warning, i18n.t('data_confirm_delete') || 'Delete')}
                  danger
                />
              </View>
            ))}
          </GlassCard>
        </Reanimated.View>

        {/* A pointer, deliberately not a button: account deletion lives in Manage
            Account, so every action on this screen shares one promise — your
            account survives it. Duplicating an irreversible button would double
            the chances of hitting it by mistake. */}
        <Reanimated.View entering={FadeInDown.delay(200).duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={12} gap={0} blurTarget={blurTargetRef} style={styles.card}>

            <Text fontSize={13} color="$onGlassSecondary" textAlign="center">
              {i18n.t('data_close_account_hint') || 'Want to close your account?'}{' '}
            </Text>
            <TouchableOpacity
              style={styles.footerLink}
              onPress={() => router.push('/(settings)/manageAccount')}
              activeOpacity={0.7}
              disabled={loading}
            >
              <Text fontSize={13} fontWeight="600" color="$onGlass" style={{ textDecorationLine: 'underline' }}>
                {i18n.t('profile_settings') || 'Manage Account'}
              </Text>
            </TouchableOpacity>
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
  card: {
    marginTop: 16,
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
  rowText: {
    flex: 1,
    gap: 2,
  },
  separator: {
    height: 1,
    backgroundColor: 'rgba(0,0,0,0.10)',
  },
  // Same segmented control as the integrations reply-mode picker.
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
    alignItems: 'center',
    justifyContent: 'center',
    paddingVertical: 10,
    borderRadius: 10,
  },
  segmentSelected: {
    backgroundColor: 'rgba(255,255,255,0.85)',
    borderWidth: 1,
    borderColor: 'rgba(5,76,56,0.18)',
  },
  footerLink: {
    paddingVertical: 4,
    paddingHorizontal: 4,
    alignItems: 'center',
  },
})
