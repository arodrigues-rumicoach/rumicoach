import { useRef, useState, useCallback, useEffect, type RefObject } from 'react'
import { View, Pressable } from 'react-native'
import { YStack } from 'tamagui'
import { ChevronLeft, ChevronRight } from 'lucide-react-native'
import { ThemedIconButton } from '@/components/atoms'
import { SessionCarouselCard, type CarouselItem } from './SessionCarouselCard'
import type { SessionType } from '@/api'
import { useSettings } from '@/hooks/useSettings'

const CONTAINER_MAX_WIDTH = 650
const CARD_WIDTH_RATIO = 0.88
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
  const [activeIndex, setActiveIndex] = useState(0)
  const containerRef = useRef<HTMLDivElement>(null)

  const screenWidth = typeof window !== 'undefined' ? window.innerWidth : 800
  const effectiveWidth = Math.min(screenWidth, CONTAINER_MAX_WIDTH)
  const cardWidth = Math.floor(effectiveWidth * CARD_WIDTH_RATIO)
  const snapInterval = cardWidth + GAP

  const canGoPrev = activeIndex > 0
  const canGoNext = activeIndex < items.length - 1

  const handleScroll = useCallback(() => {
    const container = containerRef.current
    if (!container) return
    const index = Math.round(container.scrollLeft / snapInterval)
    setActiveIndex(Math.max(0, Math.min(index, items.length - 1)))
  }, [snapInterval, items.length])

  const scrollToIndex = useCallback((index: number) => {
    const container = containerRef.current
    if (!container) return
    container.scrollTo({ left: index * snapInterval, behavior: 'smooth' })
  }, [snapInterval])

  const goPrev = useCallback(() => {
    if (canGoPrev) scrollToIndex(activeIndex - 1)
  }, [canGoPrev, activeIndex, scrollToIndex])

  const goNext = useCallback(() => {
    if (canGoNext) scrollToIndex(activeIndex + 1)
  }, [canGoNext, activeIndex, scrollToIndex])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    container.addEventListener('scroll', handleScroll, { passive: true })
    return () => container.removeEventListener('scroll', handleScroll)
  }, [handleScroll])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft') goPrev()
      if (e.key === 'ArrowRight') goNext()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [goPrev, goNext])

  if (items.length === 0) return null

  const showArrows = items.length > 1

  return (
    <YStack gap="$3" paddingHorizontal="$4" alignItems="center">
      <style>{`
        .session-carousel::-webkit-scrollbar{display:none}
      `}</style>
      <div style={{ position: 'relative', width: '100%', maxWidth: CONTAINER_MAX_WIDTH }}>
        <div
          ref={containerRef}
          className="session-carousel"
          style={{
            overflowX: 'auto',
            overflowY: 'hidden',
            scrollSnapType: 'x mandatory',
            WebkitOverflowScrolling: 'touch',
            display: 'flex',
            flexDirection: 'row',
            gap: GAP,
            scrollbarWidth: 'none',
            msOverflowStyle: 'none',
          }}
        >
          {items.map((item, index) => (
            <CarouselCardSlide
              key={item.session}
              item={item}
              index={index}
              isActive={activeIndex === index}
              cardWidth={cardWidth}
              onPress={onStartSession}
              blurTargetRef={blurTargetRef}
            />
          ))}
        </div>

        {showArrows && (
          <>
            <ArrowButton
              direction="left"
              visible={canGoPrev}
              onClick={goPrev}
            />
            <ArrowButton
              direction="right"
              visible={canGoNext}
              onClick={goNext}
            />
          </>
        )}
      </div>

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

// --- Arrow Buttons ---

interface ArrowButtonProps {
  direction: 'left' | 'right'
  visible: boolean
  onClick: () => void
}

function ArrowButton({ direction, visible, onClick }: ArrowButtonProps) {
  const isLeft = direction === 'left'
  const Icon = isLeft ? ChevronLeft : ChevronRight

  return (
    <View
      style={{
        position: 'absolute',
        top: '50%',
        [isLeft ? 'left' : 'right']: -16,
        transform: 'translateY(-50%)',
        zIndex: 10,
        opacity: visible ? 1 : 0,
        pointerEvents: visible ? 'auto' : 'none',
      }}
    >
      <ThemedIconButton
        variant="glass"
        size="md"
        onPress={onClick}
        accessibilityLabel={isLeft ? 'Previous card' : 'Next card'}
      >
        <Icon size={20} color="#262220" strokeWidth={2.5} />
      </ThemedIconButton>
    </View>
  )
}

// --- Card Slide ---

interface CarouselCardSlideProps {
  item: CarouselItem
  index: number
  isActive: boolean
  cardWidth: number
  onPress: (session: SessionType) => void
  blurTargetRef?: RefObject<View | null>
}

function CarouselCardSlide({
  item,
  isActive,
  cardWidth,
  onPress,
  blurTargetRef,
}: CarouselCardSlideProps) {
  return (
    <div
      style={{
        flexShrink: 0,
        width: cardWidth,
        scrollSnapAlign: 'center',
        opacity: isActive ? 1 : 0.6,
        transform: `scale(${isActive ? 1 : 0.95})`,
        transition: 'opacity 0.3s ease, transform 0.3s ease',
      }}
    >
      <SessionCarouselCard
        item={item}
        isActive={isActive}
        onPress={onPress}
        blurTargetRef={blurTargetRef}
      />
    </div>
  )
}

// --- Pagination Dots ---

interface PaginationDotsProps {
  count: number
  activeIndex: number
  onDotPress: (index: number) => void
}

function PaginationDots({ count, activeIndex, onDotPress }: PaginationDotsProps) {
  const { colorScheme } = useSettings()
  return (
    <YStack flexDirection="row" justifyContent="center" alignItems="center" gap={DOT_SPACING}>
      {Array.from({ length: count }).map((_, i) => (
        <Pressable
          key={i}
          onPress={() => onDotPress(i)}
          hitSlop={6}
          accessibilityRole="button"
          accessibilityLabel={`Go to card ${i + 1}`}
        >
          <div
            style={{
              height: DOT_HEIGHT,
              borderRadius: 99,
              width: activeIndex === i ? DOT_ACTIVE_WIDTH : DOT_INACTIVE_WIDTH,
              backgroundColor: activeIndex === i ? colorScheme.primary : 'rgba(255,255,255,0.3)',
              transition: 'width 0.3s ease, background-color 0.3s ease',
            }}
          />
        </Pressable>
      ))}
    </YStack>
  )
}
