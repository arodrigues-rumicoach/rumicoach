import { memo, useCallback } from 'react'
import { StyleSheet, ScrollView, Pressable, Platform, View } from 'react-native'
import { Text } from 'tamagui'
import { useSettings } from '@/hooks/useSettings'
import { INK } from '@/styles/glass'
import i18n from '@/i18n'
import { haptic } from '@/utils/haptics'

// User-facing sections, not the backend taxonomy: 'identity' also collects
// 'context' memories and 'values' collects 'needs' (see CATEGORY_GROUPS in
// memories.tsx). 'habits_rituals' and 'wheel_of_life' are exercise views
// backed by their own endpoints, not memory categories.
const CATEGORIES = [
  'all', 'identity', 'values', 'wheel_of_life',
  'obstacles', 'habits_rituals', 'insight',
] as const

const CATEGORY_LABELS: Record<string, string> = {
  all: 'category_all',
  identity: 'category_identity',
  values: 'category_values',
  wheel_of_life: 'category_wheel_of_life',
  obstacles: 'category_obstacles',
  habits_rituals: 'category_habits_rituals',
  insight: 'category_insight',
}

interface CategoryFilterProps {
  activeCategory: string
  onCategoryChange: (category: string) => void
}

export const CategoryFilter = memo(function CategoryFilter({ activeCategory, onCategoryChange }: CategoryFilterProps) {
  const { colorScheme } = useSettings()

  const handlePress = useCallback((cat: string) => {
    haptic.light()
    onCategoryChange(cat)
  }, [onCategoryChange])

  if (Platform.OS === 'web') {
    return (
      <div
        className="category-scroll"
        data-testid="category-filter"
        style={{
          height: 56,
          width: '100%',
          overflowX: 'auto',
          overflowY: 'hidden',
        }}
      >
        <div style={{
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          height: 44,
          gap: 8,
          flexWrap: 'nowrap',
          width: 'max-content',
        }}>
          {CATEGORIES.map(cat => {
            const isActive = activeCategory === cat
            return (
              <div
                key={cat}
                role="button"
                tabIndex={0}
                data-testid={`category-${cat}`}
                onClick={() => handlePress(cat)}
                onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') handlePress(cat) }}
                style={{
                  borderRadius: 100,
                  borderWidth: 1,
                  borderStyle: 'solid',
                  // Chips float on the unscrimmed video, so the inactive fill
                  // is a self-sufficient frost; active uses the theme primary.
                  borderColor: isActive ? colorScheme.primary : 'rgba(255,255,255,0.7)',
                  backgroundColor: isActive ? colorScheme.primary : 'rgba(255,255,255,0.75)',
                  paddingLeft: 16,
                  paddingRight: 16,
                  paddingTop: 8,
                  paddingBottom: 8,
                  cursor: 'pointer',
                  flexShrink: 0,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  boxSizing: 'border-box',
                  whiteSpace: 'nowrap',
                }}
              >
                <span style={{
                  color: isActive ? '#fff' : INK.primary,
                  fontSize: 13,
                  fontWeight: 600,
                  fontFamily: 'Manrope, sans-serif',
                }}>
                  {i18n.t(CATEGORY_LABELS[cat]) || cat}
                </span>
              </div>
            )
          })}
        </div>
      </div>
    )
  }

  return (
    <ScrollView
      horizontal
      showsHorizontalScrollIndicator={false}
      nestedScrollEnabled={true}
      bounces={false}
      overScrollMode="never"
      style={styles.container}
      contentContainerStyle={styles.content}
    >
      {CATEGORIES.map(cat => {
        const isActive = activeCategory === cat
        return (
          <Pressable
            key={cat}
            onPress={() => handlePress(cat)}
            style={({ pressed }) => [
              styles.chip,
              {
                backgroundColor: isActive ? colorScheme.primary : 'rgba(255,255,255,0.75)',
                borderColor: isActive ? colorScheme.primary : 'rgba(255,255,255,0.7)',
                transform: [{ scale: pressed ? 0.96 : 1 }],
              },
            ]}
          >
            <Text
              color={isActive ? '#fff' : '$onGlass'}
              fontSize={13}
              fontWeight="600"
            >
              {i18n.t(CATEGORY_LABELS[cat]) || cat}
            </Text>
          </Pressable>
        )
      })}
    </ScrollView>
  )
})

const styles = StyleSheet.create({
  container: {
    height: 44,
  },
  content: {
    paddingHorizontal: 8,
    alignItems: 'center',
    height: 44,
  },
  chip: {
    borderRadius: 100,
    borderWidth: 1,
    paddingHorizontal: 16,
    paddingVertical: 8,
    marginRight: 8,
  },
})
