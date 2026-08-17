import { type ReactNode } from 'react'
import { View, StyleSheet } from 'react-native'
import { YStack } from 'tamagui'
import { BlurView } from 'expo-blur'
import type { BlurTint } from 'expo-blur'
import { GLASS } from '@/styles/glass'
import type { GlassVariant } from '@/components/atoms/GlassCard'

interface GlassPanelProps {
  children: ReactNode
  margin?: number
  tint?: BlurTint;
  intensity?: number;
  backgroundColor?: string;
  /** 'light' is the app material (dark ink text). 'dark' remains for panels
   *  built around charts (Wheel of Life, Eisenhower). */
  variant?: GlassVariant
}

const CONTENT_STYLE = {
  paddingHorizontal: 12,
  paddingVertical: 36,
  gap: 24,
  alignItems: 'center' as const,
  // Own stacking layer: static children would otherwise paint below the
  // absolutely-positioned fill on web.
  position: 'relative' as const,
  zIndex: 1,
}

export function GlassPanel({ children, margin, tint, intensity = GLASS.intensity, variant = 'dark' }: GlassPanelProps) {
  const isLight = variant === 'light'
  const blurStyle = {
    width: '100%' as const,
    borderRadius: GLASS.radius.panel,
    overflow: 'hidden' as const,
    borderWidth: GLASS.borderWidth,
    borderColor: isLight ? GLASS.borderColor : GLASS.dark.borderColor,
    ...(margin != null ? { margin } : null),
  }

  return (
    <YStack alignItems="center" width="100%" paddingHorizontal="$4">
      <BlurView
        style={blurStyle}
        tint={tint ?? (isLight ? GLASS.tint : GLASS.dark.tint)}
        intensity={intensity}
      >
        <View style={[StyleSheet.absoluteFill, { pointerEvents: 'none', backgroundColor: isLight ? GLASS.baseFill : GLASS.dark.baseFill }]} />
        <View style={CONTENT_STYLE}>
          {children}
        </View>
      </BlurView>
    </YStack>
  )
}
