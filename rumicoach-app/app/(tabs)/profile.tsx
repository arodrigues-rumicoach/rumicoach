import { useState, useCallback } from 'react'
import { Pressable, View, StyleSheet, Modal } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { router, useFocusEffect } from 'expo-router'
import {
  Sparkles,
  Eye,
  Footprints,
  Flame,
  Compass,
  Smartphone,
  Lightbulb,
  Map as MapIcon,
  CalendarDays,
  Medal,
  CheckCheck,
  PieChart,
  TrendingUp,
  Trophy,
  Lock,
  type LucideIcon,
} from 'lucide-react-native'
import { api, type BadgeType, type UserProfileResponse, type WheelCategory } from '@/api'
import { useAuth } from '@/hooks/useAuth'
import { useSettings } from '@/hooks/useSettings'
import { useBlurTarget } from '@/context/BlurContext'
import { glowShadow } from '@/adapters/platform'
import { INK } from '@/styles/glass'
import { AnimatedCard, GlassCard, ThemedSpinner } from '@/components/atoms'
import { CurrentStateCard, ProgressCard } from '@/components/organisms'
import { ContentLayout, TabScreenWrapper } from '@/components/templates'
import i18n from '@/i18n'

interface Badge {
  id: BadgeType
  Icon: LucideIcon
  /** snake_case stem for i18n: badge_<key>_label / badge_<key>_desc. Separate
   *  from `id` so the camelCase BadgeType enum can match the backend while the
   *  translation keys keep the codebase's snake_case convention. */
  key: string
  label: string
  description: string
  /** Earned badges light up in their own colour so the grid reads as a set of
   *  distinct achievements rather than one repeated accent. */
  color: string
}

// Tints a badge colour for fills/borders without needing a second constant per
// badge. Inputs are the fixed 6-digit hexes below, so parsing stays trivial.
function tint(hex: string, alpha: number): string {
  const n = parseInt(hex.slice(1), 16)
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${alpha})`
}

// Ordered by when a user realistically earns each one, so the grid fills
// top-down and reads as a progress map: rows are stages (first days, first
// week, first month, the long game) and the third column is the streak ladder.
// Earned state comes from the /me/profile badges array (BadgeType keys).
// Labels/descriptions fall back to these English strings if a locale misses a key.
const BADGES: Badge[] = [
  // Day 1–3
  { id: 'firstSession', key: 'first_session', Icon: Sparkles, label: 'First session', description: 'You had your first real conversation with Rumi.', color: '#f59e0b' },
  { id: 'visionSet', key: 'vision_set', Icon: Eye, label: 'Vision set', description: 'You mapped your ideal life — and where you stand today.', color: '#8b5cf6' },
  { id: 'firstCommitment', key: 'first_commitment', Icon: Footprints, label: 'First step', description: 'You kept your first commitment to yourself.', color: '#14b8a6' },
  // First week — third column starts the streak ladder (3 → 7 → 30)
  { id: 'firstDeepSession', key: 'first_deep_session', Icon: Compass, label: 'Deep dive', description: 'You went past check-ins into your first deep session.', color: '#06b6d4' },
  { id: 'alwaysWithYou', key: 'always_with_you', Icon: Smartphone, label: 'Always with you', description: 'Rumi now reaches you outside the app.', color: '#3b82f6' },
  { id: 'threeDayStreak', key: 'three_day_streak', Icon: Flame, label: '3-day streak', description: 'Three days running — the habit is forming.', color: '#fb923c' },
  // Weeks 2–5
  { id: 'tenInsights', key: 'ten_insights', Icon: Lightbulb, label: 'Self-aware', description: 'Ten insights uncovered about yourself.', color: '#eab308' },
  { id: 'allThemesExplored', key: 'all_themes_explored', Icon: MapIcon, label: 'Explorer', description: "You've explored all seven coaching themes.", color: '#a855f7' },
  { id: 'sevenDayStreak', key: 'seven_day_streak', Icon: Flame, label: '7-day streak', description: 'A full week of showing up.', color: '#f97316' },
  // Month 1–2
  { id: 'twentySessions', key: 'twenty_sessions', Icon: Medal, label: '20 sessions', description: 'Twenty sessions in — this is a practice now.', color: '#ec4899' },
  { id: 'twentyFiveCommitments', key: 'twenty_five_commitments', Icon: CheckCheck, label: 'Committed', description: 'Twenty-five commitments kept.', color: '#10b981' },
  { id: 'thirtyDayStreak', key: 'thirty_day_streak', Icon: CalendarDays, label: '30-day streak', description: 'A month of consistency.', color: '#ef4444' },
  // The long game
  { id: 'wheelRemapped', key: 'wheel_remapped', Icon: PieChart, label: 'Progress mapped', description: 'You remapped your Wheel of Life to see how far you\'ve come.', color: '#0ea5e9' },
  { id: 'areaImproved', key: 'area_improved', Icon: TrendingUp, label: 'Rising', description: 'An area of your life climbed 2 points or more.', color: '#fbbf24' },
  { id: 'hundredSessions', key: 'hundred_sessions', Icon: Trophy, label: '100 sessions', description: 'One hundred sessions with Rumi.', color: '#7c3aed' },
]

const badgeLabel = (b: Badge) => i18n.t(`badge_${b.key}_label`) || b.label
const badgeDescription = (b: Badge) => i18n.t(`badge_${b.key}_desc`) || b.description

function SectionLabel({ children, right }: { children: string; right?: string }) {
  // Rendered INSIDE each section's glass card — nothing floats bare on the
  // unscrimmed video (see src/styles/glass.ts).
  return (
    <XStack justifyContent="space-between" alignItems="baseline">
      <Text fontSize={13} fontWeight="700" letterSpacing={0.5} color="$onGlassSecondary" textTransform="uppercase">
        {children}
      </Text>
      {right ? (
        <Text fontSize={11.5} color="$onGlassTertiary">
          {right}
        </Text>
      ) : null}
    </XStack>
  )
}

function ProfileScreen() {
  const blurTargetRef = useBlurTarget()
  const { user, appLanguage, ensureValidToken } = useAuth()
  const { colorScheme } = useSettings()
  const [profile, setProfile] = useState<UserProfileResponse | null>(null)
  const [loading, setLoading] = useState(true)
  // Which badge's detail sheet is open. Descriptions live here rather than
  // under every tile: at 30% grid width there is no room for a legible line.
  const [openBadge, setOpenBadge] = useState<Badge | null>(null)

  const fetchData = useCallback(async () => {
    try {
      await ensureValidToken()
      const { data } = await api.get<UserProfileResponse>('/me/profile')
      setProfile(data)
    } catch (err) {
      if (__DEV__) console.error('Failed to fetch profile data', err)
    } finally {
      setLoading(false)
    }
  }, [ensureValidToken])

  // No key-based remount here: tabs stay mounted on native (see the tabs
  // layout), and forcing a remount on focus restarts every entering
  // animation a frame after the content has painted — visible as a flash
  // on Android. The refetch alone keeps the screen fresh.
  useFocusEffect(
    useCallback(() => {
      fetchData()
    }, [fetchData]),
  )

  const locale = appLanguage || 'en-US'
  const initials = (user?.name || '?')
    .split(/\s+/)
    .map((w) => w.charAt(0))
    .slice(0, 2)
    .join('')
    .toUpperCase()

  // lifeBalance.data is a JSON-encoded WheelOfLifeItem array; the profile
  // snapshot on the user record is the fallback.
  let wheelEntries: [string, number][] = []
  try {
    const parsed: WheelCategory[] = profile?.lifeBalance?.data ? JSON.parse(profile.lifeBalance.data) : []
    wheelEntries = parsed.map((c) => [c.name, c.currentScore])
  } catch {
    wheelEntries = []
  }
  if (wheelEntries.length === 0) {
    wheelEntries = Object.entries(user?.wheelOfLife ?? {})
  }
  const focusArea = profile?.focusArea ?? user?.focusArea ?? null
  const progress = profile?.progress
  const hoursWithRumiRaw = progress?.hoursWithRumi ?? 0
  const rumiHours = Math.floor(hoursWithRumiRaw)
  const rumiMinutes = Math.round((hoursWithRumiRaw - rumiHours) * 60)
  const rumiTimeFormatted = rumiMinutes > 0 ? `${rumiHours}h ${rumiMinutes}m` : `${rumiHours}h`
  // Map rather than Set so the detail sheet can show when a badge was earned.
  const earnedBadges = new Map((profile?.badges ?? []).map((b) => [b.type, b.earnedAt]))
  const vision = profile?.vision?.text ?? null

  const formatDate = useCallback(
    (dateString?: string) => {
      if (!dateString) return ''
      try {
        return new Date(dateString).toLocaleDateString(locale, { month: 'short', day: 'numeric', year: 'numeric' })
      } catch {
        return ''
      }
    },
    [locale],
  )

  const formatMonthYear = useCallback(
    (dateString?: string) => {
      if (!dateString) return ''
      try {
        const d = new Date(dateString)
        const formatted = d.toLocaleDateString(locale, { month: 'long', year: 'numeric' })
        return formatted.charAt(0).toUpperCase() + formatted.slice(1)
      } catch {
        return ''
      }
    },
    [locale],
  )

  const memberSinceDate = user?.createdAt ? formatMonthYear(user.createdAt) : ''

  const handleOpenStreak = useCallback(() => {
    router.push('/streak')
  }, [])

  if (loading) {
    return (
      <YStack flex={1} backgroundColor="transparent" justifyContent="center" alignItems="center">
        <ThemedSpinner size="large" />
      </YStack>
    )
  }

  return (
    <ContentLayout scrollable showsVerticalScrollIndicator={false} showsHorizontalScrollIndicator={false}>
      <YStack gap="$6" padding="$4">
        {/* Identity */}
        <AnimatedCard viewportAware staggerIndex={0}>
          <GlassCard variant="light" borderRadius={18} padding={14} blurTarget={blurTargetRef} containerView={false}>
            <XStack gap={14} alignItems="center" padding={14}>
              <View style={[styles.avatar, { backgroundColor: colorScheme.primary }]}>
                <Text fontSize={20} fontWeight="700" color="#fff">
                  {initials}
                </Text>
              </View>
              <YStack flex={1} gap={2}>
                <Text fontSize={17} fontWeight="700" color="$onGlass">
                  {user?.name || i18n.t('profile_guest') || 'Guest'}
                </Text>
                <Text fontSize={12.5} color="$onGlassSecondary">
                  {memberSinceDate
                    ? `${i18n.t('profile_growing_since') || 'Growing with Rumi since'} ${memberSinceDate}`
                    : (i18n.t('profile_growing_with_rumi') || 'Growing with Rumi')}
                </Text>
              </YStack>
            </XStack>
          </GlassCard>
        </AnimatedCard>

        {/* Ideal life vision */}
        <AnimatedCard viewportAware staggerIndex={1}>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={10} blurTarget={blurTargetRef}>
            <SectionLabel>{i18n.t('profile_vision_label') || 'Ideal life vision'}</SectionLabel>
            {vision ? (
              <YStack gap={4}>
                <Text fontSize={14} lineHeight={22} color="$onGlass">
                  "{vision}"
                </Text>
                <Text fontSize={11.5} color="$onGlassTertiary">
                  {(i18n.t('profile_vision_footer') || 'Crafted with Rumi') +
                    (profile?.vision?.craftedAt ? ' · ' + formatDate(profile.vision.craftedAt) : '')}
                </Text>
              </YStack>
            ) : (
              <Text fontSize={13} lineHeight={20} color="$onGlassSecondary" textAlign="center" paddingVertical={8}>
                {i18n.t('profile_vision_empty') ||
                  "You haven't crafted your ideal life vision yet — explore it in your next session with Rumi."}
              </Text>
            )}
          </GlassCard>
        </AnimatedCard>

        {/* Current State */}
        <AnimatedCard viewportAware staggerIndex={2}>
          <CurrentStateCard
            wheelEntries={wheelEntries}
            focusArea={focusArea}
            accentColor={colorScheme.accent}
            blurTarget={blurTargetRef}
          />
        </AnimatedCard>

        {/* Progress */}
        <AnimatedCard viewportAware staggerIndex={3}>
          <ProgressCard
            currentStreak={progress?.currentStreak ?? 0}
            bestStreak={progress?.bestStreak ?? 0}
            totalSessions={progress?.totalSessions ?? 0}
            rumiTimeFormatted={rumiTimeFormatted}
            insightsDiscovered={progress?.insightsDiscovered ?? 0}
            commitmentsKept={progress?.commitmentsKept ?? 0}
            onPressStreak={handleOpenStreak}
            onPressSessions={() => router.push('/sessions')}
            onPressInsights={() => router.push({ pathname: '/(tabs)/memories', params: { category: 'insight' } })}
            onPressCommitments={() => router.push('/commitments')}
          />
        </AnimatedCard>

        {/* Badges */}
        <AnimatedCard>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={12} blurTarget={blurTargetRef}>
            <SectionLabel
              right={`${earnedBadges.size} ${i18n.t('profile_badges_of') || 'of'} ${BADGES.length} ${i18n.t('profile_badges_earned') || 'earned'}`}
            >
              {i18n.t('profile_badges_label') || 'Badges'}
            </SectionLabel>
            {/* The chips are translucent tints reading through the card's
                shared glass, rather than nine separate BlurViews. */}
            <View style={styles.badgesGrid}>
              {BADGES.map((badge) => {
                const earned = earnedBadges.has(badge.id)
                return (
                  <Pressable
                    key={badge.id}
                    onPress={() => setOpenBadge(badge)}
                    accessibilityRole="button"
                    accessibilityLabel={`${badgeLabel(badge)}${earned ? '' : ` · ${i18n.t('profile_badge_locked') || 'locked'}`}`}
                    accessibilityHint={i18n.t('profile_badge_hint') || 'Shows how to earn this badge'}
                    style={({ pressed }) => [
                      styles.badgeCard,
                      earned
                        ? [
                          {
                            backgroundColor: tint(badge.color, 0.18),
                            borderColor: tint(badge.color, 0.5),
                            borderWidth: 1,
                          },
                          // Soft halo in the badge's own colour — the thing that
                          // makes an earned badge feel lit up rather than just filled.
                          glowShadow(tint(badge.color, 0.55), 12, 0.5),
                        ]
                        : styles.badgeLocked,
                      pressed && styles.badgePressed,
                    ]}
                  >
                    {/* Earned icons sit in a solid disc of their colour so the
                        glyph reads at 20px instead of washing into the tint. */}
                    <View style={earned ? [styles.badgeIconWrap, { backgroundColor: badge.color }] : [styles.badgeIconWrap, { opacity: 0.35, backgroundColor: '#aeaeaeff' }]}>
                      <badge.Icon size={20} color={earned ? '#ffffff' : INK.primary} />
                    </View>
                    <Text
                      fontSize={10}
                      fontWeight={earned ? '700' : '600'}
                      color="$onGlass"
                      opacity={earned ? 1 : 0.4}
                      textAlign="center"
                    >
                      {badgeLabel(badge)}
                    </Text>
                  </Pressable>
                )
              })}
            </View>
          </GlassCard>
        </AnimatedCard>
      </YStack>

      <BadgeDetailModal
        badge={openBadge}
        earnedAt={openBadge ? earnedBadges.get(openBadge.id) : undefined}
        formatDate={formatDate}
        onClose={() => setOpenBadge(null)}
      />
    </ContentLayout>
  )
}

// Tapping a badge opens this rather than expanding the tile in place: the grid
// is three columns wide, so an inline description would reflow every row below it.
function BadgeDetailModal({
  badge, earnedAt, formatDate, onClose,
}: {
  badge: Badge | null
  earnedAt?: string
  formatDate: (d?: string) => string
  onClose: () => void
}) {
  if (!badge) return null
  const earned = !!earnedAt
  const Icon = badge.Icon

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onClose}>
      {/* Backdrop is itself the dismiss target; the card stops propagation so
          taps inside it don't close the sheet. */}
      <Pressable style={styles.modalBackdrop} onPress={onClose} accessibilityLabel={i18n.t('close') || 'Close'}>
        <Pressable style={styles.modalCard} onPress={(e) => e.stopPropagation()}>
          <YStack alignItems="center" gap="$3">
            <View
              style={[
                styles.modalIconWrap,
                earned
                  ? [{ backgroundColor: badge.color }, glowShadow(tint(badge.color, 0.55), 16, 0.5)]
                  : styles.modalIconLocked,
              ]}
            >
              <Icon size={30} color={earned ? '#ffffff' : INK.primary} />
            </View>

            <Text fontSize={19} fontWeight="700" color={INK.primary} textAlign="center">
              {badgeLabel(badge)}
            </Text>

            <Text fontSize={14} lineHeight={20} color={INK.secondary} textAlign="center">
              {badgeDescription(badge)}
            </Text>

            {earned ? (
              <Text fontSize={12} fontWeight="600" color={badge.color} textAlign="center">
                {(i18n.t('profile_badge_earned_on') || 'Earned') + ' ' + formatDate(earnedAt)}
              </Text>
            ) : (
              <XStack gap={6} alignItems="center">
                <Lock size={13} color={INK.tertiary} />
                <Text fontSize={12} fontWeight="600" color={INK.tertiary}>
                  {i18n.t('profile_badge_not_earned') || 'Not earned yet'}
                </Text>
              </XStack>
            )}
          </YStack>
        </Pressable>
      </Pressable>
    </Modal>
  )
}

export default function ProfileScreenWrapper() {
  return (
    <TabScreenWrapper>
      <ProfileScreen />
    </TabScreenWrapper>
  )
}

const styles = StyleSheet.create({
  avatar: {
    width: 56,
    height: 56,
    borderRadius: 28,
    alignItems: 'center',
    justifyContent: 'center',
  },
  badgesGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 12,
  },
  badgeCard: {
    flexBasis: '30%',
    flexGrow: 1,
    borderRadius: 14,
    paddingVertical: 12,
    paddingHorizontal: 6,
    alignItems: 'center',
    gap: 8,
  },
  badgeIconWrap: {
    width: 38,
    height: 38,
    borderRadius: 19,
    alignItems: 'center',
    justifyContent: 'center',
  },
  badgeLocked: {
    backgroundColor: 'rgba(0,0,0,0.04)',
    borderWidth: 1,
    borderStyle: 'dashed',
    borderColor: 'rgba(0,0,0,0.25)',
  },
  badgePressed: {
    opacity: 0.7,
  },
  modalBackdrop: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.55)',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 32,
  },
  modalCard: {
    width: '100%',
    maxWidth: 340,
    borderRadius: 20,
    padding: 24,
    backgroundColor: '#f4f4f5',
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.08)',
  },
  modalIconWrap: {
    width: 60,
    height: 60,
    borderRadius: 30,
    alignItems: 'center',
    justifyContent: 'center',
  },
  modalIconLocked: {
    backgroundColor: 'rgba(0,0,0,0.06)',
    borderWidth: 1,
    borderStyle: 'dashed',
    borderColor: 'rgba(0,0,0,0.25)',
    opacity: 0.55,
  },
})
