import { useState, useEffect, useCallback, useMemo, useContext } from 'react'
import { useFocusEffect, useLocalSearchParams } from 'expo-router'
import { View, FlatList } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { useAuth } from '@/hooks/useAuth'
import { useSettings } from '@/hooks/useSettings'
import { api, type Memory, type MemoryPaginatedResponse, type WheelOfLifeExercise, type Action } from '@/api'
import i18n from '@/i18n'
import { ContentLayout, TabScreenWrapper } from '@/components/templates'
import { GlassCard, ThemedSpinner, ThemedButton } from '@/components/atoms'
import WheelOfLifeChart from '@/components/organisms/WheelOfLifeChart'
import { ActionsCard } from '@/components/organisms/journey'
import { Toast } from '@/components/molecules'
import { AlertContext } from '@/context/AlertContext'
import { BookOpen, RefreshCw } from 'lucide-react-native'
import { useSession } from '@/hooks/useSession'
import { MemoryCard, CategoryFilter } from '@/components/organisms/memories'
import { INK } from '@/styles/glass'
import { trackMemoryDeleted, trackCommitmentToggled } from '@/analytics'

// The user-facing chips are coarser than the backend taxonomy: Filipa's
// back-office categories stay on the server, but in the app 'context'
// memories live under About Me and 'needs' under What Matters.
const CATEGORY_GROUPS: Record<string, string[]> = {
  identity: ['identity', 'context'],
  values: ['values', 'needs'],
}

// Chips that render an exercise/commitments view instead of a memory list.
const NON_MEMORY_CATEGORIES = ['wheel_of_life', 'habits_rituals']

function WheelOfLifeView({ data, selectedId, onSelect }: { data: WheelOfLifeExercise[]; selectedId: string | null; onSelect: (id: string) => void }) {
  const { colorScheme } = useSettings()
  const { user } = useAuth()

  const formatDate = useCallback((dateString: string) => {
    try {
      return new Date(dateString).toLocaleDateString(user?.preferredLanguage || 'en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      })
    } catch {
      return dateString
    }
  }, [user?.preferredLanguage])

  if (data.length === 0) {
    return (
      <YStack flex={1} justifyContent="center" alignItems="center" gap="$4" padding="$4">
        <BookOpen size={48} color="$colorTertiary" />
        <GlassCard variant="light" borderRadius={16} padding={16}>
          <Text color="$onGlass" textAlign="center" fontSize={14}>
            {i18n.t('no_wheel_of_life') || 'No Wheel of Life exercises found.'}
          </Text>
        </GlassCard>
      </YStack>
    )
  }

  const selected = data.find(e => e.id === selectedId) || data[0]

  return (
    <YStack gap="$3" padding="$4">
      {data.length > 1 && (
        <XStack gap="$2" flexWrap="wrap">
          {data.map(e => (
            <ThemedButton
              key={e.id}
              variant={selectedId === e.id ? 'solid' : 'glass'}
              onPress={() => onSelect(e.id)}
            >
              {formatDate(e.createdAt)}
            </ThemedButton>
          ))}
        </XStack>
      )}
      <GlassCard padding={16} borderRadius={16}>
        <WheelOfLifeChart data={selected.data} />
      </GlassCard>
    </YStack>
  )
}

function HabitsRitualsView({ data, onToggle }: { data: Action[]; onToggle: (actionId: string, status: string) => void }) {
  const { appLanguage } = useAuth()

  const getWeekdayNames = useCallback((days?: number[]) => {
    if (!days || days.length === 0) return ''
    const locale = appLanguage || 'en-US'
    return days.map((d) => {
      try {
        const base = new Date(2024, 0, d)
        return base.toLocaleDateString(locale, { weekday: 'short' })
      } catch {
        return ''
      }
    }).filter(Boolean).join(', ')
  }, [appLanguage])

  const formatGoalDate = useCallback((dateString?: string) => {
    if (!dateString) return ''
    try {
      return new Date(dateString).toLocaleDateString(appLanguage || 'en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      })
    } catch {
      return dateString || ''
    }
  }, [appLanguage])

  if (data.length === 0) {
    return (
      <YStack flex={1} justifyContent="center" alignItems="center" gap="$4" padding="$4">
        <BookOpen size={48} color="$colorTertiary" />
        <GlassCard variant="light" borderRadius={16} padding={16}>
          <Text color="$onGlass" textAlign="center" fontSize={14}>
            {i18n.t('no_commitments') || 'No habits or rituals yet. Commit to something in a session with Rumi!'}
          </Text>
        </GlassCard>
      </YStack>
    )
  }

  return (
    <YStack gap="$3" padding="$4">
      <ActionsCard
        actions={data}
        onToggleAction={onToggle}
        getWeekdayNames={getWeekdayNames}
        formatDate={formatGoalDate}
      />
    </YStack>
  )
}

function MemoriesScreen() {
  const { user } = useAuth()
  const { isConnected } = useSession()
  const { showConfirm } = useContext(AlertContext)!
  const [memories, setMemories] = useState<Memory[]>([])
  const [wheelData, setWheelData] = useState<WheelOfLifeExercise[]>([])
  const [commitments, setCommitments] = useState<Action[]>([])
  const [selectedWheelId, setSelectedWheelId] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [activeCategory, setActiveCategory] = useState<string>('all')
  const [message, setMessage] = useState<{ text: string; type: 'success' | 'error' } | null>(null)
  const { category } = useLocalSearchParams<{ category?: string }>()

  useEffect(() => {
    if (category) {
      setActiveCategory(category)
    }
  }, [category])

  const fetchData = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      if (activeCategory === 'wheel_of_life') {
        const { data } = await api.get<{ items: WheelOfLifeExercise[] }>('/wheel-of-life')
        const items = data.items || []
        setWheelData(items)
        if (items.length > 0 && !selectedWheelId) {
          setSelectedWheelId(items[0].id)
        }
      } else if (activeCategory === 'habits_rituals') {
        const { data } = await api.get<Action[]>('/commitments')
        setCommitments(data ?? [])
      } else {
        const { data } = await api.get<MemoryPaginatedResponse>('/memories')
        setMemories(data.items)
      }
    } catch (err) {
      if (__DEV__) console.error('Failed to fetch data', err)
      setError(i18n.t('failed_update') || 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [activeCategory])

  useFocusEffect(
    useCallback(() => {
      fetchData()
    }, [fetchData])
  )

  const filteredMemories = useMemo(() => {
    if (activeCategory === 'all') return memories
    const group = CATEGORY_GROUPS[activeCategory]
    return memories.filter(m => (group ? group.includes(m.category) : m.category === activeCategory))
  }, [memories, activeCategory])

  const toggleCommitment = useCallback(async (actionId: string, currentStatus: string) => {
    const isCompleted = currentStatus === 'completed'
    const newDone = !isCompleted
    const optimistic = (done: boolean) => setCommitments(prev => prev.map(a =>
      a.id === actionId ? { ...a, status: done ? 'completed' as const : 'pending' as const } : a
    ))

    optimistic(newDone)

    try {
      // PATCH /commitments/{id} returns the updated commitment (same flow as the journey tab).
      const { data } = await api.patch<Action>(`/commitments/${actionId}`, { done: newDone })
      setCommitments(prev => prev.map(a => (a.id === actionId ? data : a)))
      trackCommitmentToggled(newDone, data.type)
    } catch (err) {
      if (__DEV__) console.error('Failed to update commitment status', err)
      optimistic(isCompleted)
    }
  }, [])

  const handleDeleteMemory = useCallback((id: string) => {
    showConfirm({
      title: i18n.t('delete_confirm_title') || 'Are you sure?',
      message: i18n.t('delete_memory_confirm') || 'Are you sure you want to delete this memory?',
      confirmLabel: i18n.t('delete') || 'Delete',
      destructive: true,
      onConfirm: async () => {
        const prev = memories
        // Read before the optimistic removal below drops it from state.
        const category = memories.find(m => m.id === id)?.category
        setMemories(prev => prev.filter(m => m.id !== id))
        try {
          await api.delete(`/memories/${id}`)
          trackMemoryDeleted(category ?? 'unknown')
          setMessage({ text: i18n.t('memory_deleted') || 'Memory deleted.', type: 'success' })
        } catch {
          setMemories(prev)
          setMessage({ text: i18n.t('failed_update') || 'Failed to delete', type: 'error' })
        }
      },
    })
  }, [memories, showConfirm])

  const formatDate = useCallback((dateString: string) => {
    try {
      return new Date(dateString).toLocaleDateString(user?.preferredLanguage || 'en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      })
    } catch {
      return dateString
    }
  }, [user?.preferredLanguage])

  const handleCategoryChange = useCallback((category: string) => {
    setActiveCategory(category)
  }, [])

  const isMemoriesCategory = !NON_MEMORY_CATEGORIES.includes(activeCategory)

  return (
    <ContentLayout scrollable={!isMemoriesCategory}>
      <YStack paddingHorizontal={16} flex={1}>
        {message && (
          <YStack position="absolute" top={60} left="$4" right="$4" zIndex={100}>
            <Toast
              message={message.text}
              type={message.type}
              onClose={() => setMessage(null)}
            />
          </YStack>
        )}

        <View style={{ height: 56, marginBottom: 12, marginTop: isConnected ? 32 : 12 }}>
          <CategoryFilter
            activeCategory={activeCategory}
            onCategoryChange={handleCategoryChange}
          />
        </View>

        {loading ? (
          <YStack flex={1} justifyContent="center" alignItems="center">
            <ThemedSpinner size="large" />
          </YStack>
        ) : error ? (
          <YStack flex={1} justifyContent="center" alignItems="center" gap="$4" padding="$4">
            <GlassCard variant="light" borderRadius={16} padding={16}>
              <Text color="$onGlass" textAlign="center" fontSize={14}>
                {error}
              </Text>
            </GlassCard>
            <ThemedButton
              variant="glass"
              onPress={fetchData}
              icon={<RefreshCw size={16} />}
            >
              {i18n.t('retry') || 'Retry'}
            </ThemedButton>
          </YStack>
        ) : (
          <>
            <View style={{ display: isMemoriesCategory && filteredMemories.length > 0 ? 'flex' : 'none', flex: 1 }}>
              <FlatList
                data={filteredMemories}
                keyExtractor={(item) => item.id}
                renderItem={({ item }) => (
                  <MemoryCard
                    memory={item}
                    showCategory={activeCategory === 'all'}
                    formatDate={formatDate}
                    onDelete={handleDeleteMemory}
                  />
                )}
                ItemSeparatorComponent={() => <View style={{ height: 12 }} />}
                removeClippedSubviews
                maxToRenderPerBatch={10}
                windowSize={5}
                showsVerticalScrollIndicator={false}
              />
            </View>

            {isMemoriesCategory && filteredMemories.length === 0 && (
              <GlassCard variant="light" borderRadius={16} padding={16}>
                <YStack alignItems="center" gap="$4" padding="$4">
                  <BookOpen size={48} color={INK.primary} />
                  <YStack alignItems="center" gap="$1">
                    <Text color="$onGlass" textAlign="center" fontSize={14}>
                      {i18n.t('no_memories_found') || 'No memories found. Start a session to capture new insights!'}
                    </Text>
                    <Text color="$onGlass" textAlign="center" fontSize={14}>
                      {i18n.t('no_memories_found_description') || 'No memories found. Start a session to capture new insights!'}
                    </Text>
                    <Text color="$onGlass" textAlign="center" fontSize={14}>
                      {i18n.t('session_talk_to_rumi') || 'No memories found. Start a session to capture new insights!'}
                    </Text>
                  </YStack>
                </YStack>
              </GlassCard>
            )}

            {activeCategory === 'wheel_of_life' && (
              <WheelOfLifeView
                data={wheelData}
                selectedId={selectedWheelId}
                onSelect={setSelectedWheelId}
              />
            )}

            {activeCategory === 'habits_rituals' && (
              <HabitsRitualsView
                data={commitments}
                onToggle={toggleCommitment}
              />
            )}
          </>
        )}
      </YStack>
    </ContentLayout>
  )
}

export default function MemoriesScreenWrapper() {
  return (
    <TabScreenWrapper>
      <MemoriesScreen />
    </TabScreenWrapper>
  )
}
