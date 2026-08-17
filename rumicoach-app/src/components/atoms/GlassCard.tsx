import { type ReactNode, type RefObject } from 'react'
import { View, StyleSheet, type ViewStyle } from 'react-native'
import { BlurView } from 'expo-blur'
import { GLASS } from '@/styles/glass'

export type GlassVariant = 'light' | 'dark'

interface GlassCardProps {
  children: ReactNode
  padding?: number
  borderRadius?: number
  gap?: number
  style?: ViewStyle
  blurTarget?: RefObject<View | null>
  intensity?: number
  containerView?: boolean
  /** 'light' is the app's frosted material (dark ink text, $onGlass*).
   *  'dark' is the legacy material (white text) kept for screens that have
   *  not been migrated yet — auth, streak, session, memory cards. */
  variant?: GlassVariant
  /** Translucent colour layered over the fill (e.g. streak amber).
   *  Keep alpha ≤ ~0.2 so the material keeps carrying the contrast. */
  tintColor?: string
  borderColor?: string
  /** Override the fill. Pass 'transparent' to opt out — only safe for
   *  surfaces that never carry small text. */
  fill?: string
}

const DEFAULT_PADDING = 20
const DEFAULT_GAP = 12

export function GlassCard({
  children,
  padding = DEFAULT_PADDING,
  borderRadius = GLASS.radius.card,
  gap = DEFAULT_GAP,
  style,
  blurTarget,
  intensity = GLASS.intensity,
  containerView = true,
  variant = 'dark',
  tintColor,
  borderColor,
  fill,
}: GlassCardProps) {
  const isLight = variant === 'light'
  const resolvedFill = fill ?? (isLight ? GLASS.baseFill : GLASS.dark.baseFill)
  const resolvedBorder = borderColor ?? (isLight ? GLASS.borderColor : GLASS.dark.borderColor)

  return (
    <BlurView
      blurTarget={blurTarget}
      tint={isLight ? GLASS.tint : GLASS.dark.tint}
      intensity={intensity}
      style={[{
        borderRadius,
        overflow: 'hidden',
        borderWidth: GLASS.borderWidth,
        borderColor: resolvedBorder,
      }, style]}
    >
      {/* The blur's own tint plus this fill is what guarantees the AA floor
          with no scrim over the video — see src/styles/glass.ts. */}
      <View style={[StyleSheet.absoluteFill, { pointerEvents: 'none', backgroundColor: resolvedFill }]} />
      {tintColor ? (
        <View style={[StyleSheet.absoluteFill, { pointerEvents: 'none', backgroundColor: tintColor }]} />
      ) : null}
      {/* Content always gets its own stacking layer: on web, Tamagui stacks
          render position:static, which CSS paints BELOW the absolutely-
          positioned fill overlays regardless of DOM order — without this the
          fill would wash the text out. */}
      <View style={containerView ? { padding, gap, position: 'relative', zIndex: 1 } : { position: 'relative', zIndex: 1 }}>
        {children}
      </View>
    </BlurView>
  )
}
