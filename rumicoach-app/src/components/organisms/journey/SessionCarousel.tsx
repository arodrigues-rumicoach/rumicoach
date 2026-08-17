import { useCallback, useRef, type RefObject } from 'react'
import { FlatList, View, useWindowDimensions, Pressable } from 'react-native'
import { YStack } from 'tamagui'
import Animated, {
  useAnimatedScrollHandler,
  useSharedValue,
  useAnimatedStyle,
  useDerivedValue,
  interpolate,
  withSpring,
  type SharedValue,
} from 'react-native-reanimated'
import { SessionCarouselCard, type CarouselItem } from './SessionCarouselCard'
import type { SessionType } from '@/api'
import { useSettings } from '@/hooks/useSettings'

const AnimatedFlatList = Animated.createAnimatedComponent(FlatList<CarouselItem>)

const CARD_WIDTH_RATIO = 0.80
const GAP = 12
const DOT_ACTIVE_WIDTH = 24
const DOT_INACTIVE_WIDTH = 8
const DOT_HEIGHT = 8
const DOT_SPACING = 6

interface SessionCarouselProps {
  items: CarouselItem[]
  onStartSession: (session: SessionType) => void
  blurTargetRef?: RefObject<View | null>
}

export function SessionCarousel({ items, onStartSession, blurTargetRef }: SessionCarouselProps) {
  const { width: screenWidth } = useWindowDimensions()
  const cardWidth = Math.floor(screenWidth * CARD_WIDTH_RATIO)
  const snapInterval = cardWidth + GAP

  const flatListRef = useRef<FlatList<CarouselItem>>(null)
  const scrollX = useSharedValue(0)

  const scrollHandler = useAnimatedScrollHandler({
    onScroll: (e) => {
      scrollX.value = e.contentOffset.x
    },
  })

  const activeIndex = useDerivedValue(() => {
    if (snapInterval === 0) return 0
    return Math.round(scrollX.value / snapInterval)
  })

  const scrollToIndex = useCallback((index: number) => {
    flatListRef.current?.scrollToOffset({
      offset: index * snapInterval,
      animated: true,
    })
  }, [snapInterval])

  const renderItem = useCallback(({ item, index }: { item: CarouselItem; index: number }) => {
    return (
      <CarouselCardWrapper
        item={item}
        index={index}
        scrollX={scrollX}
        cardWidth={cardWidth}
        snapInterval={snapInterval}
        onPress={onStartSession}
        blurTargetRef={blurTargetRef}
        isFirst={index === 0}
        isLast={index === items.length - 1}
      />
    )
  }, [scrollX, cardWidth, snapInterval, onStartSession, blurTargetRef])

  const keyExtractor = useCallback((item: CarouselItem) => item.session, [])

  const ItemSeparator = useCallback(() => (<View style={{ width: GAP }} />), [])

  if (items.length === 0) return null

  return (
    <YStack gap="$3">
      <AnimatedFlatList
        ref={flatListRef}
        data={items}
        renderItem={renderItem}
        keyExtractor={keyExtractor}
        horizontal
        showsHorizontalScrollIndicator={false}
        snapToInterval={snapInterval}
        snapToAlignment="center"
        decelerationRate="fast"
        onScroll={scrollHandler}
        scrollEventThrottle={16}
        ItemSeparatorComponent={ItemSeparator}
      />
      {items.length > 1 && (
        <PaginationDots
          count={items.length}
          activeIndex={activeIndex}
          onDotPress={scrollToIndex}
        />
      )}
    </YStack>
  )
}

// --- Card Wrapper (scroll-driven animation) ---

interface CarouselCardWrapperProps {
  item: CarouselItem
  index: number
  scrollX: SharedValue<number>
  cardWidth: number
  snapInterval: number
  onPress: (session: SessionType) => void
  blurTargetRef?: RefObject<View | null>
  isFirst?: boolean
  isLast?: boolean
}

function CarouselCardWrapper({
  item,
  index,
  scrollX,
  cardWidth,
  snapInterval,
  onPress,
  blurTargetRef,
  isFirst,
  isLast,
}: CarouselCardWrapperProps) {
  const wrapperStyle = useAnimatedStyle(() => {
    const cardCenter = index * snapInterval
    const distance = Math.abs(scrollX.value - cardCenter)
    const normalized = Math.min(distance / snapInterval, 1)
    return {
      opacity: interpolate(normalized, [0, 1], [1, 0.6]),
      transform: [{ scale: interpolate(normalized, [0, 1], [1, 0.95]) }],
    }
  })

  const isActive = useDerivedValue(() => {
    const cardCenter = index * snapInterval
    const distance = Math.abs(scrollX.value - cardCenter)
    return distance < snapInterval / 2
  })

  return (
    <Animated.View style={[{ width: cardWidth, overflow: 'hidden', borderRadius: 18 }, wrapperStyle]}>
      <SessionCarouselCard
        item={item}
        isActive={isActive}
        onPress={onPress}
        blurTargetRef={blurTargetRef}
        isFirst={isFirst}
        isLast={isLast}
      />
    </Animated.View>
  )
}

// --- Pagination Dots ---

interface PaginationDotsProps {
  count: number
  activeIndex: SharedValue<number>
  onDotPress: (index: number) => void
}

function PaginationDots({ count, activeIndex, onDotPress }: PaginationDotsProps) {
  return (
    <YStack flexDirection="row" justifyContent="center" alignItems="center" gap={DOT_SPACING}>
      {Array.from({ length: count }).map((_, i) => (
        <PaginationDot key={i} index={i} activeIndex={activeIndex} onPress={onDotPress} />
      ))}
    </YStack>
  )
}

interface PaginationDotProps {
  index: number
  activeIndex: SharedValue<number>
  onPress: (index: number) => void
}

function PaginationDot({ index, activeIndex, onPress }: PaginationDotProps) {
  const { colorScheme } = useSettings()
  const animatedStyle = useAnimatedStyle(() => {
    const isActive = activeIndex.value === index
    return {
      width: withSpring(isActive ? DOT_ACTIVE_WIDTH : DOT_INACTIVE_WIDTH, {
        damping: 15,
        stiffness: 400,
      }),
      backgroundColor: isActive ? colorScheme.primary : 'rgba(255,255,255,0.3)',
    }
  })

  const handlePress = useCallback(() => {
    onPress(index)
  }, [onPress, index])

  return (
    <Pressable onPress={handlePress} hitSlop={6}>
      <Animated.View
        style={[
          {
            height: DOT_HEIGHT,
            borderRadius: 99,
          },
          animatedStyle,
        ]}
      />
    </Pressable>
  )
}
