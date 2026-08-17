import { useState, useCallback, useEffect } from 'react'
import { type ViewStyle, StyleSheet } from 'react-native'
import { Input, type InputProps, XStack, type ColorTokens } from 'tamagui'
import Reanimated, {
  useSharedValue,
  useAnimatedStyle,
  withTiming,
  interpolateColor,
} from 'react-native-reanimated'
import { INK } from '@/styles/glass'

interface ThemedInputProps extends Omit<
  InputProps,
  'placeholderTextColor' | 'style' | 'backgroundColor' | 'borderRadius' | 'borderWidth' | 'borderColor' | 'width' | 'maxWidth' | 'flex'
> {
  placeholderTextColor?: string | ColorTokens
  icon?: React.ReactNode
  iconPosition?: 'left' | 'right'
  maxWidth?: number | string
  width?: number | string
  flex?: number | string
  backgroundColor?: string
  borderRadius?: number
  paddingHorizontal?: number
  paddingVertical?: number
  minHeight?: number
  style?: ViewStyle | ViewStyle[]
  inputSyle?: ViewStyle | ViewStyle[]
  /** 'dark' = legacy dark glass (white text/borders), 'light' = frosted white
   *  glass with dark ink (matches GlassCard variant='light'). */
  variant?: 'dark' | 'light'
}

const DARK_IDLE_BORDER = 'rgba(255,255,255,0.15)'
const DARK_FOCUSED_BORDER = 'rgba(255,255,255,0.7)'
const LIGHT_IDLE_BORDER = INK.primary
const LIGHT_FOCUSED_BORDER = INK.primary

interface AnimatedIconProps {
  children: React.ReactNode
  focused: boolean
}

function AnimatedIcon({ children, focused }: AnimatedIconProps) {
  const progress = useSharedValue(focused ? 1 : 0)

  useEffect(() => {
    progress.value = withTiming(focused ? 1 : 0, { duration: 180 })
  }, [focused, progress])

  const animatedStyle = useAnimatedStyle(() => ({
    opacity: 0.55 + progress.value * 0.45,
    transform: [{ scale: 0.95 + progress.value * 0.1 }],
  }))

  if (!children) return null

  return <Reanimated.View style={[styles.iconWrapper, animatedStyle]}>{children}</Reanimated.View>
}

export function ThemedInput({
  placeholderTextColor,
  icon,
  iconPosition = 'left',
  maxWidth,
  width,
  flex,
  backgroundColor,
  borderRadius = 12,
  paddingHorizontal = 16,
  paddingVertical,
  minHeight = 44,
  onFocus,
  onBlur,
  inputSyle,
  style,
  variant = 'dark',
  ...inputProps
}: ThemedInputProps) {
  const isLight = variant === 'light'
  const resolvedPlaceholderColor = placeholderTextColor ?? (isLight ? 'rgba(74,69,64,0.5)' : 'rgba(255,255,255,0.4)')
  const resolvedBg = backgroundColor ?? (isLight ? 'rgba(255,255,255,0.05)' : 'rgba(255,255,255,0.08)')
  const IDLE_BORDER = isLight ? LIGHT_IDLE_BORDER : DARK_IDLE_BORDER
  const FOCUSED_BORDER = isLight ? LIGHT_FOCUSED_BORDER : DARK_FOCUSED_BORDER
  const [focused, setFocused] = useState(false)
  const focusProgress = useSharedValue(0)

  const handleFocus = useCallback((e: any) => {
    setFocused(true)
    focusProgress.value = withTiming(1, { duration: 180 })
    onFocus?.(e)
  }, [onFocus, focusProgress])

  const handleBlur = useCallback((e: any) => {
    setFocused(false)
    focusProgress.value = withTiming(0, { duration: 180 })
    onBlur?.(e)
  }, [onBlur, focusProgress])

  const animatedBorderStyle = useAnimatedStyle(() => ({
    borderColor: interpolateColor(
      focusProgress.value,
      [0, 1],
      [IDLE_BORDER, FOCUSED_BORDER]
    ),
    borderWidth: focusProgress.value > 0.5 ? 2 : 1,
  }))

  const containerStyle: ViewStyle = {
    borderRadius,
    overflow: 'hidden',
    flex: flex as any,
    width: width as any,
    maxWidth: maxWidth as any,
    backgroundColor: resolvedBg,
    paddingVertical,
    minHeight,
    height: 48,
  }
  const containerPaddingLeft = icon && iconPosition === 'left' ? 16 : 0
  const containerPaddingRight = icon && iconPosition === 'right' ? 16 : 0

  return (
    <Reanimated.View style={[containerStyle, animatedBorderStyle, style as ViewStyle]}>
      <XStack
        alignItems="center"
        gap={8}
        style={{ flexDirection: iconPosition === 'right' ? 'row-reverse' : 'row', paddingLeft: containerPaddingLeft, paddingRight: containerPaddingRight }}
        height='100%'
      >
        {icon && iconPosition === 'left' && <AnimatedIcon focused={focused}>{icon}</AnimatedIcon>}
        <Input
          placeholderTextColor={resolvedPlaceholderColor as ColorTokens}
          onFocus={handleFocus}
          onBlur={handleBlur}
          borderWidth={0}
          borderColor="transparent"
          backgroundColor="transparent"
          outlineWidth={0}
          outlineColor="transparent"
          outlineStyle="none"
          focusStyle={{ outlineWidth: 0, outlineColor: 'transparent', borderColor: 'transparent', borderWidth: 0 }}
          flex={1}
          style={inputSyle}
          {...inputProps}
        />
        {icon && iconPosition === 'right' && <AnimatedIcon focused={focused}>{icon}</AnimatedIcon>}
      </XStack>
    </Reanimated.View>
  )
}

const styles = StyleSheet.create({
  iconWrapper: {
    alignItems: 'center',
    justifyContent: 'center',
  },
})
