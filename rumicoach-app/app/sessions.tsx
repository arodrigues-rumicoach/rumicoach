import { useState, useCallback } from 'react'
import { View, FlatList } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { useFocusEffect, Stack } from 'expo-router'
import { useSafeAreaInsets } from 'react-native-safe-area-context'

import { api } from '@/api'
import type { UserSessionPaginatedResponse, CommunicationSession } from '@/api'
import { useAuth } from '@/hooks/useAuth'
import i18n from '@/i18n'
import { WebMaxWidth } from '@/components/templates'
import { GlassCard, ThemedSpinner, ThemedButton } from '@/components/atoms'
import { CommunicationSessionCard, PageHeader } from '@/components/organisms'
import { MessageCircle, Clock, Timer } from 'lucide-react-native'

export default function SessionsHistoryScreen() {
  const { user } = useAuth()
  const insets = useSafeAreaInsets()
  const [sessions, setSessions] = useState<CommunicationSession[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchSessions = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const { data } = await api.get<UserSessionPaginatedResponse>('/sessions?limit=50')
      setSessions(data.items || [])
    } catch (err) {
      if (__DEV__) console.error('Failed to fetch sessions', err)
      setError(i18n.t('failed_update') || 'Failed to load sessions')
    } finally {
      setLoading(false)
    }
  }, [])

  useFocusEffect(
    useCallback(() => {
      fetchSessions()
    }, [fetchSessions])
  )

  // Calculate summary stats
  const totalSessions = sessions.length
  const totalMinutes = sessions.reduce((acc, session) => acc + Math.round((session.duration || 0) / 60), 0)
  const avgDuration = totalSessions > 0 ? Math.round(totalMinutes / totalSessions) : 0
  const totalHours = Math.floor(totalMinutes / 60)
  const remainingMinutes = totalMinutes % 60
  const totalTimeFormatted = totalHours > 0 ? `${totalHours}h ${remainingMinutes}m` : `${remainingMinutes}m`

  const renderItem = ({ item }: { item: CommunicationSession }) => {
    return <CommunicationSessionCard item={item} userLanguage={user?.preferredLanguage} />
  }

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <WebMaxWidth>
        <View style={{ flex: 1 }}>

          <PageHeader title={i18n.t('session_history') || 'Session History'} />
          <YStack paddingHorizontal={16} flex={1} paddingTop={16}>
            {loading ? (
              <YStack flex={1} justifyContent="center" alignItems="center">
                <ThemedSpinner size="large" />
              </YStack>
            ) : error ? (
              <YStack flex={1} justifyContent="center" alignItems="center" gap="$4">
                <GlassCard variant="light" borderRadius={16} padding={16}>
                  <Text color="$onGlass" textAlign="center">{error}</Text>
                </GlassCard>
                <ThemedButton variant="glass" onPress={fetchSessions}>
                  {i18n.t('retry') || 'Retry'}
                </ThemedButton>
              </YStack>
            ) : sessions.length === 0 ? (
              <YStack flex={1} justifyContent="center" alignItems="center" gap="$4" padding="$4">
                <GlassCard variant="light" borderRadius={16} padding={20}>
                  <YStack alignItems="center" gap="$3">
                    <MessageCircle size={48} color="rgba(0,0,0,0.2)" />
                    <Text fontSize={16} fontWeight="600" color="$onGlass" textAlign="center">
                      {i18n.t('no_sessions_found') || 'No sessions yet'}
                    </Text>
                    <Text fontSize={13} color="$onGlassSecondary" textAlign="center" lineHeight={18}>
                      {i18n.t('sessions_empty_hint') || 'Start a conversation with Rumi to begin tracking your sessions.'}
                    </Text>
                  </YStack>
                </GlassCard>
              </YStack>
            ) : (
              <YStack flex={1} gap={12}>
                {/* Summary Stats */}
                <GlassCard variant="light" borderRadius={14} padding={14}>
                  <XStack justifyContent="space-around">
                    <YStack alignItems="center" gap={4}>
                      <XStack alignItems="center" gap={6}>
                        <MessageCircle size={14} color="rgba(0,0,0,0.4)" />
                        <Text fontSize={20} fontWeight="700" color="$onGlass">
                          {totalSessions}
                        </Text>
                      </XStack>
                      <Text fontSize={11} color="$onGlassSecondary">
                        {i18n.t('sessions_total') || 'total sessions'}
                      </Text>
                    </YStack>
                    <YStack alignItems="center" gap={4}>
                      <XStack alignItems="center" gap={6}>
                        <Clock size={14} color="rgba(0,0,0,0.4)" />
                        <Text fontSize={20} fontWeight="700" color="$onGlass">
                          {avgDuration}m
                        </Text>
                      </XStack>
                      <Text fontSize={11} color="$onGlassSecondary">
                        {i18n.t('sessions_avg_duration') || 'avg duration'}
                      </Text>
                    </YStack>
                    <YStack alignItems="center" gap={4}>
                      <XStack alignItems="center" gap={6}>
                        <Timer size={14} color="rgba(0,0,0,0.4)" />
                        <Text fontSize={20} fontWeight="700" color="$onGlass">
                          {totalTimeFormatted}
                        </Text>
                      </XStack>
                      <Text fontSize={11} color="$onGlassSecondary">
                        {i18n.t('sessions_total_time') || 'total time'}
                      </Text>
                    </YStack>
                  </XStack>
                </GlassCard>

                {/* Sessions List */}
                <FlatList
                  data={sessions}
                  keyExtractor={item => item.id}
                  renderItem={renderItem}
                  contentContainerStyle={{ paddingBottom: insets.bottom + 16, gap: 12 }}
                  showsVerticalScrollIndicator={false}
                />
              </YStack>
            )}
          </YStack>
        </View>
      </WebMaxWidth>
    </>
  )
}
