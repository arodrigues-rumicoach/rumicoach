import { memo, useCallback, useState, type RefObject } from 'react'
import { StyleSheet, View, type LayoutChangeEvent } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
import { Mic } from 'lucide-react-native'
import { LinearGradient } from 'expo-linear-gradient'
import { Heading, ThemedButton, GlassCard, LazyImage } from '@/components/atoms'
import { haptic } from '@/utils/haptics'

// Past this width there's room to sit the CTA beside the copy instead of
// under it. Keyed off the card's own width rather than Platform.OS so a
// narrow browser window or a split-screen tablet gets the stacked layout too.
const SIDE_BY_SIDE_WIDTH = 520

interface DailyFocusCardProps {
  title: string
  subtitle: string
  actionText: string
  imageUrl: string
  onPress: () => void
  blurTargetRef?: RefObject<View | null>
}

export const DailyFocusCard = memo(function DailyFocusCard({
  title, subtitle, actionText, imageUrl, onPress, blurTargetRef
}: DailyFocusCardProps) {
  const [sideBySide, setSideBySide] = useState(false)

  const handleLayout = useCallback((e: LayoutChangeEvent) => {
    const next = e.nativeEvent.layout.width >= SIDE_BY_SIDE_WIDTH
    setSideBySide((prev) => (prev === next ? prev : next))
  }, [])

  const handlePress = useCallback(() => {
    haptic.medium()
    onPress()
  }, [onPress])

  const copy = (
    <>
      <Heading color="#fff" fontWeight="bold" fontSize={20} lineHeight={26}>
        {title}
      </Heading>
      {/* Pure white, no dimming: text on media follows the same white-only
          rule as everywhere else — hierarchy comes from size/weight. */}
      <Text color="#fff" fontSize={14} lineHeight={19}>
        {subtitle}
      </Text>
    </>
  )

  // Glass rather than solid, and sized to its label: a full-width accent block
  // covered a quarter of the hero and fought the photo. Shape and typography
  // are left to ThemedButton's defaults so this matches every other button in
  // the app — only the fill differs.
  const button = (
    <ThemedButton
      variant="glass"
      icon={<Mic size={20} />}
      // Narrow: a full-width bar, the standard mobile CTA shape. Wide: sized to
      // its label so it sits beside the copy instead of stretching across.
      fullWidth={!sideBySide}
      alignSelf={sideBySide ? 'flex-end' : 'stretch'}
      onPress={handlePress}
    >
      {actionText}
    </ThemedButton>
  )

  return (
    <GlassCard
      blurTarget={blurTargetRef}
      padding={0}
      borderRadius={18}
      style={styles.card}
    >
      {/* Full-bleed hero: the photo runs the whole card so there's no seam
          between "image" and "controls" — content sits on top of it. */}
      <View style={styles.hero} onLayout={handleLayout}>
        <LazyImage source={{ uri: imageUrl }} style={styles.image} resizeMode="cover" />

        {/* Text-protection gradient, not a wash: the top half of the photo is
            completely untouched and only the bottom band (where the copy sits)
            darkens enough for white text. The session photos are arbitrary
            (bright bokeh, pale skies), so the floor must come from this band:
            ≥0.45 under the title (20px bold → 3:1) and ≥0.55 under the
            subtitle (14px → 4.5:1) — asserted in contrast.test.ts. */}
        <LinearGradient
          colors={['rgba(0,0,0,0)', 'rgba(0,0,0,0.30)', 'rgba(0,0,0,0.72)']}
          locations={[0, 0.45, 1]}
          style={styles.scrim}
        />

        {/* zIndex is required: the image and scrim are absolutely positioned,
            so without it they paint over this statically-positioned content.
            Bottom-anchored so a long subtitle grows upward into the photo
            instead of clipping against a fixed-height band. */}
        <YStack flex={1} zIndex={1} justifyContent="flex-end" padding="$4" gap="$3.5">
          {sideBySide ? (
            // Wide (web): copy and CTA share a line, both sitting on the baseline.
            <XStack alignItems="flex-end" justifyContent="space-between" gap="$4">
              <YStack flex={1} gap={4}>
                {copy}
              </YStack>
              {button}
            </XStack>
          ) : (
            // Narrow (mobile): stacked, with the CTA centred under the copy.
            <>
              <YStack gap={4}>
                {copy}
              </YStack>
              {button}
            </>
          )}
        </YStack>
      </View>
    </GlassCard>
  )
})

const styles = StyleSheet.create({
  card: {
    overflow: 'hidden',
    position: 'relative',
    borderColor: 'rgba(255,255,255,0.10)',
  },
  hero: {
    minHeight: 200,
    justifyContent: 'flex-end',
  },
  // Explicit inset rules rather than spreading StyleSheet.absoluteFill: that
  // constant is a registered-style reference, so spreading it can yield an
  // empty object and silently drop the positioning.
  image: {
    position: 'absolute',
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
    width: '100%',
  },
  scrim: {
    position: 'absolute',
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
  },
})
