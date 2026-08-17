import { useState, useCallback } from 'react'
import { View, ScrollView, Pressable, StyleSheet } from 'react-native'
import { YStack, Text, XStack } from 'tamagui'
import { useFocusEffect, Stack } from 'expo-router'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { Target, CheckCircle, Clock } from 'lucide-react-native'

import { api } from '@/api'
import type { Action } from '@/api'
import { useAuth } from '@/hooks/useAuth'
import i18n from '@/i18n'
import { WebMaxWidth } from '@/components/templates'
import { GlassCard, ThemedSpinner, ThemedButton, BackButton } from '@/components/atoms'
import { Toast } from '@/components/molecules'
import { haptic } from '@/utils/haptics'
import { PageHeader } from '@/components'
import { ScrollNavProvider } from '@/context/ScrollNavContext'

export default function CommitmentsHistoryScreen() {
  const { user } = useAuth()
  const insets = useSafeAreaInsets()
  const [commitments, setCommitments] = useState<Action[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<{ text: string; type: 'success' | 'error' } | null>(null)

  const fetchCommitments = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const { data } = await api.get<Action[]>('/commitments?history=true')
      setCommitments(data || [])
    } catch (err) {
      if (__DEV__) console.error('Failed to fetch commitments', err)
      setError(i18n.t('failed_update') || 'Failed to load commitments')
    } finally {
      setLoading(false)
    }
  }, [])

  useFocusEffect(
    useCallback(() => {
      fetchCommitments()
    }, [fetchCommitments])
  )

  const getWeekdayNames = useCallback((days?: number[]) => {
    if (!days || days.length === 0) return ''
    const names = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
    return days.map(d => names[d] || d).join(', ')
  }, [])

  const formatDate = useCallback(
    (dateString?: string) => {
      if (!dateString) return ''
      try {
        return new Date(dateString).toLocaleDateString(user?.preferredLanguage || 'en-US', {
          year: 'numeric',
          month: 'short',
          day: 'numeric',
        })
      } catch {
        return dateString
      }
    },
    [user?.preferredLanguage]
  )

  const handleActionToggle = useCallback(async (actionId: string, status: string) => {
    haptic.medium()
    try {
      const isCompleted = status === 'completed'
      const newDone = !isCompleted
      setCommitments(prev =>
        prev.map(a => (a.id === actionId ? { ...a, done: newDone, status: newDone ? 'completed' : 'pending' } : a))
      )
      await api.patch<Action>(`/commitments/${actionId}`, { done: newDone })
    } catch (err) {
      if (__DEV__) console.error('Failed to toggle action', err)
      fetchCommitments() // Revert on failure
      setMessage({ text: i18n.t('failed_update') || 'Failed to update', type: 'error' })
    }
  }, [fetchCommitments])

  // Calculate summary stats
  const totalCommitments = commitments.length
  const completedCount = commitments.filter(c => c.status === 'completed').length
  const pendingCount = commitments.filter(c => c.status === 'pending').length
  const overdueCount = commitments.filter(c => c.status === 'overdue').length
  const completionRate = totalCommitments > 0 ? Math.round((completedCount / totalCommitments) * 100) : 0

  return (
    <ScrollNavProvider>
      <Stack.Screen options={{ headerShown: false }} />
      <WebMaxWidth>
        <PageHeader title={i18n.t('commitments_history') || 'Commitments History'} canGoBack />
        <View style={{ flex: 1 }}>
          <YStack paddingHorizontal={16} flex={1} paddingTop={16}>
            {message && (
              <YStack position="absolute" top={insets.top + 16} left="$4" right="$4" zIndex={100}>
                <Toast
                  message={message.text}
                  type={message.type}
                  onClose={() => setMessage(null)}
                />
              </YStack>
            )}

            {loading ? (
              <YStack flex={1} justifyContent="center" alignItems="center">
                <ThemedSpinner size="large" />
              </YStack>
            ) : error ? (
              <YStack flex={1} justifyContent="center" alignItems="center" gap="$4">
                <GlassCard variant="light" borderRadius={16} padding={16}>
                  <Text color="$onGlass" textAlign="center">{error}</Text>
                </GlassCard>
                <ThemedButton variant="glass" onPress={fetchCommitments}>
                  {i18n.t('retry') || 'Retry'}
                </ThemedButton>
              </YStack>
            ) : commitments.length === 0 ? (
              <YStack flex={1} justifyContent="center" alignItems="center" gap="$4" padding="$4">
                <GlassCard variant="light" borderRadius={16} padding={20}>
                  <YStack alignItems="center" gap="$3">
                    <Target size={48} color="rgba(0,0,0,0.2)" />
                    <Text fontSize={16} fontWeight="600" color="$onGlass" textAlign="center">
                      {i18n.t('no_commitments_found') || 'No commitments yet'}
                    </Text>
                    <Text fontSize={13} color="$onGlassSecondary" textAlign="center" lineHeight={18}>
                      {i18n.t('commitments_empty_hint') || 'Set commitments during your sessions with Rumi to track your progress.'}
                    </Text>
                  </YStack>
                </GlassCard>
              </YStack>
            ) : (
              <ScrollView showsVerticalScrollIndicator={false} contentContainerStyle={{ paddingBottom: insets.bottom + 16 }}>
                <YStack gap={12}>
                  {/* Summary Stats */}
                  <GlassCard variant="light" borderRadius={14} padding={14}>
                    <XStack justifyContent="space-around">
                      <YStack alignItems="center" gap={4}>
                        <XStack alignItems="center" gap={6}>
                          <Target size={14} color="rgba(0,0,0,0.4)" />
                          <Text fontSize={20} fontWeight="700" color="$onGlass">
                            {totalCommitments}
                          </Text>
                        </XStack>
                        <Text fontSize={11} color="$onGlassSecondary">
                          {i18n.t('commitments_total') || 'total'}
                        </Text>
                      </YStack>
                      <YStack alignItems="center" gap={4}>
                        <XStack alignItems="center" gap={6}>
                          <CheckCircle size={14} color="rgba(6,95,70,0.6)" />
                          <Text fontSize={20} fontWeight="700" color="$onGlass">
                            {completedCount}
                          </Text>
                        </XStack>
                        <Text fontSize={11} color="$onGlassSecondary">
                          {i18n.t('commitments_completed') || 'completed'}
                        </Text>
                      </YStack>
                      <YStack alignItems="center" gap={4}>
                        <XStack alignItems="center" gap={6}>
                          <Clock size={14} color="rgba(0,0,0,0.4)" />
                          <Text fontSize={20} fontWeight="700" color="$onGlass">
                            {completionRate}%
                          </Text>
                        </XStack>
                        <Text fontSize={11} color="$onGlassSecondary">
                          {i18n.t('commitments_rate') || 'completion rate'}
                        </Text>
                      </YStack>
                    </XStack>
                  </GlassCard>

                  {/* Commitments List */}
                  {commitments.map((action, index) => {
                    const isCompleted = action.status === 'completed'
                    const isOverdue = action.status === 'overdue'

                    return (
                      <Pressable
                        key={action.id}
                        onPress={() => handleActionToggle(action.id, action.status)}
                        style={({ pressed }) => [{ opacity: pressed ? 0.7 : 1 }]}
                      >
                        <GlassCard variant="light" borderRadius={20} padding={16} gap={14}>
                          <XStack justifyContent="space-between" alignItems="center">
                            <XStack
                              backgroundColor={
                                isCompleted
                                  ? 'rgba(6, 95, 70, 0.2)' // subtle green
                                  : isOverdue
                                    ? 'rgba(185, 28, 28, 0.2)' // subtle red
                                    : 'rgba(255, 255, 255, 0.2)'
                              }
                              paddingHorizontal={12}
                              paddingVertical={4}
                              borderRadius={16}
                            >
                              <Text
                                fontSize={12}
                                fontWeight="800"
                                color={
                                  isCompleted
                                    ? '#065F46'
                                    : isOverdue
                                      ? '#B91C1C'
                                      : '$onGlass'
                                }
                                textTransform="uppercase"
                                letterSpacing={0.5}
                              >
                                {isCompleted
                                  ? (i18n.t('completed') || 'Completed')
                                  : isOverdue
                                    ? (i18n.t('journey_overdue') || 'Overdue')
                                    : (i18n.t('pending') || 'Pending')}
                              </Text>
                            </XStack>
                            <Text fontSize={13} fontWeight="600" color="$onGlassSecondary">
                              {action.type === 'recurring' ? (i18n.t('journey_recurring_task') || 'Recurring') : (i18n.t('journey_one_time_task') || 'One-time')}
                            </Text>
                          </XStack>

                          <YStack gap={6}>
                            <Text
                              fontSize={16}
                              fontWeight="500"
                              color="$onGlass"
                              textDecorationLine={isCompleted ? 'line-through' : 'none'}
                              opacity={isCompleted ? 0.65 : 1}
                            >
                              {action.title}
                            </Text>
                            <Text fontSize={13} color="$onGlassTertiary">
                              {action.type === 'recurring'
                                ? (action.days && action.days.length > 0 ? getWeekdayNames(action.days) : '')
                                : (action.date ? formatDate(action.date) : '')}
                            </Text>
                          </YStack>
                        </GlassCard>
                      </Pressable>
                    )
                  })}
                </YStack>
              </ScrollView>
            )}
          </YStack>
        </View>
      </WebMaxWidth>
    </ScrollNavProvider>
  )
}

const styles = StyleSheet.create({})
