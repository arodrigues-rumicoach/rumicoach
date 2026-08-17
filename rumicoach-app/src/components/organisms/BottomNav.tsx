import { useState, useRef, useEffect, useLayoutEffect, useCallback, memo } from 'react'
import { TouchableOpacity, View, StyleSheet } from 'react-native'
import Svg, { Line, Path } from 'react-native-svg'
import { BlurView } from 'expo-blur'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import type { BottomTabBarProps } from 'expo-router/js-tabs'
import { useSettings } from '@/hooks/useSettings'
import { useScrollNav } from '@/context/ScrollNavContext'
import { pushTab } from '@/utils/tabHistory'
import { GLASS, INK } from '@/styles/glass'
import i18n, { useI18n } from '@/i18n'
import { WEB_MAX_WIDTH } from '@/utils/constants'
import { DisplayText } from '../atoms'
import Reanimated, {
  useSharedValue,
  useAnimatedProps,
  useAnimatedStyle,
  withTiming,
  withSpring,
  withSequence,
  withDelay,
  cancelAnimation,
  Easing,
  interpolate,
  Extrapolation,
} from 'react-native-reanimated'
import type { SharedValue } from 'react-native-reanimated'
import { isAndroid } from '@tamagui/core'

const AnimatedPath = Reanimated.createAnimatedComponent(Path)
const AnimatedLine = Reanimated.createAnimatedComponent(Line)

// ─── SessionIcon: bar-chart lines that bounce when active ───

interface SessionIconProps {
  color: string
  size?: number
  active: boolean
  trigger: number
}

const SESSION_LINES = [
  { x: 2, y1: 10, y2: 14 },
  { x: 6, y1: 6, y2: 18 },
  { x: 10, y1: 3, y2: 21 },
  { x: 14, y1: 8, y2: 16 },
  { x: 18, y1: 5, y2: 19 },
  { x: 22, y1: 10, y2: 14 },
]

function AnimatedSessionLine({ line, offset }: {
  line: { x: number; y1: number; y2: number }
  offset: SharedValue<number>
}) {
  const animatedProps = useAnimatedProps(() => ({
    y1: line.y1 + offset.value,
    y2: line.y2 + offset.value,
  }))
  return <AnimatedLine x1={line.x} x2={line.x} animatedProps={animatedProps} />
}

const SessionIcon = memo(function SessionIcon({ color, size = 26, active, trigger }: SessionIconProps) {
  const offset0 = useSharedValue(0)
  const offset1 = useSharedValue(0)
  const offset2 = useSharedValue(0)
  const offset3 = useSharedValue(0)
  const offset4 = useSharedValue(0)
  const offset5 = useSharedValue(0)
  const offsets = [offset0, offset1, offset2, offset3, offset4, offset5]

  useEffect(() => {
    if (active && trigger > 0) {
      offsets.forEach((offset, i) => {
        cancelAnimation(offset)
        offset.value = withDelay(
          i * 30,
          withSequence(
            withTiming(-3, { duration: 200, easing: Easing.out(Easing.cubic) }),
            withTiming(2, { duration: 200, easing: Easing.inOut(Easing.cubic) }),
            withTiming(0, { duration: 200, easing: Easing.inOut(Easing.cubic) }),
          ),
        )
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- offsets are stable shared values
  }, [active, trigger])

  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
      {SESSION_LINES.map((line, i) => (
        <AnimatedSessionLine key={i} line={line} offset={offsets[i]} />
      ))}
    </Svg>
  )
})

// ─── JourneyIcon: a compass — the dial draws in, then the needle lands.
//     The tab used to be "Growth" and carried a trend arrow, which read as
//     metrics going up rather than a path being walked. A compass says
//     orientation, which is what the screen offers: where you are and what
//     comes next. The route, the screen id and the backend all say 'journey' now.
//
//     Two closed shapes rather than a drawn scene: at 26px the neighbouring
//     tabs are two-stroke silhouettes (book, person, waveform), and anything
//     busier reads as denser than them. A winding path or a road, tried first,
//     either collapsed into a blob or sat heavier than the rest of the bar.

interface JourneyIconProps {
  color: string
  size?: number
  active: boolean
  trigger: number
}

// The dial: a circle of radius 9 centred in the box.
const JOURNEY_PATH_1 = 'M12 3 a9 9 0 1 0 0.01 0'
// The needle: a slim diamond pointing north-east.
const JOURNEY_PATH_2 = 'M15.6 8.4 L10.4 10.4 L8.4 15.6 L13.6 13.6 Z'
// A damped swing, in degrees: the needle overshoots, hunts, and settles on
// north. Each swing is shorter and quicker than the last, which is what makes
// it read as a needle finding its bearing rather than a spinner.
const JOURNEY_SWING = [-30, 22, -13, 7, 0]
const JOURNEY_SWING_MS = [170, 200, 170, 140, 180]

const JourneyIcon = memo(function JourneyIcon({ color, size = 26, active, trigger }: JourneyIconProps) {
  const spin = useSharedValue(0)

  useEffect(() => {
    if (active && trigger > 0) {
      cancelAnimation(spin)
      spin.value = withSequence(
        ...JOURNEY_SWING.map((deg, i) =>
          withTiming(deg, {
            duration: JOURNEY_SWING_MS[i],
            // Ease out of the first throw, then ease both ways so the reversals
            // slow at the extremes the way a real needle does.
            easing: i === 0 ? Easing.out(Easing.quad) : Easing.inOut(Easing.quad),
          }),
        ),
      )
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- spin is a stable shared value
  }, [active, trigger])

  // Rotating the wrapping view rather than the SVG node: a view rotates about
  // its own centre, which is the dial's centre, and it behaves identically on
  // web and native without relying on animated transform props in react-native-svg.
  const needleStyle = useAnimatedStyle(() => ({
    transform: [{ rotate: `${spin.value}deg` }],
  }))

  return (
    <View style={{ width: size, height: size }}>
      {/* The dial is the housing — it stays put while the needle hunts. */}
      <Svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
        <Path d={JOURNEY_PATH_1} />
      </Svg>
      <Reanimated.View style={[{ position: 'absolute', top: 0, left: 0 }, needleStyle]}>
        <Svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
          <Path d={JOURNEY_PATH_2} />
        </Svg>
      </Reanimated.View>
    </View>
  )
})

// ─── MemoriesIcon: book with page flip + writing scribbles ───

interface MemoriesIconProps {
  color: string
  size?: number
  active: boolean
  trigger: number
}

const LEFT_SCRIBBLES = [
  'M4 7c2 0 1-1 3-1s1 1 3 1',
  'M4 11c2 0 1-1 3-1s1 1 5 1',
  'M4 15c2 0 1-1 3-1s1 1 4 1',
]
const RIGHT_SCRIBBLES = [
  'M14 7c2 0 1-1 3-1s1 1 3 1',
  'M14 11c2 0 1-1 3-1s1 1 5 1',
  'M14 15c2 0 1-1 3-1s1 1 4 1',
]

function AnimatedScribble({ d, opacity, length }: {
  d: string
  opacity: SharedValue<number>
  length: SharedValue<number>
}) {
  const props = useAnimatedProps(() => ({
    opacity: opacity.value,
    strokeDasharray: '12',
    strokeDashoffset: interpolate(length.value, [0, 1], [12, 0], Extrapolation.CLAMP),
  }))
  return <AnimatedPath d={d} strokeWidth={1} animatedProps={props} />
}

const MemoriesIcon = memo(function MemoriesIcon({ color, size = 26, active, trigger }: MemoriesIconProps) {
  const sLo0 = useSharedValue(0)
  const sLo1 = useSharedValue(0)
  const sLo2 = useSharedValue(0)
  const sLl0 = useSharedValue(0)
  const sLl1 = useSharedValue(0)
  const sLl2 = useSharedValue(0)
  const sRo0 = useSharedValue(0)
  const sRo1 = useSharedValue(0)
  const sRo2 = useSharedValue(0)
  const sRl0 = useSharedValue(0)
  const sRl1 = useSharedValue(0)
  const sRl2 = useSharedValue(0)
  const scribbleLeftOpacities = [sLo0, sLo1, sLo2]
  const scribbleLeftLengths = [sLl0, sLl1, sLl2]
  const scribbleRightOpacities = [sRo0, sRo1, sRo2]
  const scribbleRightLengths = [sRl0, sRl1, sRl2]

  useEffect(() => {
    if (active && trigger > 0) {
      scribbleLeftOpacities.forEach((op, i) => {
        cancelAnimation(op)
        op.value = withDelay(300 + i * 80, withSequence(
          withTiming(0, { duration: 1 }),
          withTiming(1, { duration: 1 }),
        ))
      })
      scribbleLeftLengths.forEach((len, i) => {
        cancelAnimation(len)
        len.value = withDelay(300 + i * 80, withSequence(
          withTiming(0, { duration: 1 }),
          withTiming(1, { duration: 250, easing: Easing.out(Easing.cubic) }),
        ))
      })
      scribbleRightOpacities.forEach((op, i) => {
        cancelAnimation(op)
        op.value = withDelay(700 + i * 80, withSequence(
          withTiming(0, { duration: 1 }),
          withTiming(1, { duration: 1 }),
        ))
      })
      scribbleRightLengths.forEach((len, i) => {
        cancelAnimation(len)
        len.value = withDelay(700 + i * 80, withSequence(
          withTiming(0, { duration: 1 }),
          withTiming(1, { duration: 250, easing: Easing.out(Easing.cubic) }),
        ))
      })
    } else if (!active) {
      scribbleLeftOpacities.forEach(op => { op.value = 0 })
      scribbleLeftLengths.forEach(len => { len.value = 0 })
      scribbleRightOpacities.forEach(op => { op.value = 0 })
      scribbleRightLengths.forEach(len => { len.value = 0 })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- scribble arrays are stable shared values
  }, [active, trigger])

  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <Path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
      <Path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
      <Line x1={12} y1={7} x2={12} y2={21} strokeWidth={1} strokeOpacity={0.5} />

      {LEFT_SCRIBBLES.map((d, i) => (
        <AnimatedScribble key={`l${i}`} d={d} opacity={scribbleLeftOpacities[i]} length={scribbleLeftLengths[i]} />
      ))}
      {RIGHT_SCRIBBLES.map((d, i) => (
        <AnimatedScribble key={`r${i}`} d={d} opacity={scribbleRightOpacities[i]} length={scribbleRightLengths[i]} />
      ))}
    </Svg>
  )
})

// ─── ProfileIcon: person outline, shoulders draw in when active ───

interface ProfileIconProps {
  color: string
  size?: number
  active: boolean
  trigger: number
}

const PROFILE_SHOULDERS = 'M4 21v-1a7 7 0 0 1 7-7h2a7 7 0 0 1 7 7v1'
const PROFILE_SHOULDERS_LEN = 30

const ProfileIcon = memo(function ProfileIcon({ color, size = 26, active, trigger }: ProfileIconProps) {
  const dash = useSharedValue(0)

  useEffect(() => {
    if (active && trigger > 0) {
      cancelAnimation(dash)
      dash.value = withSequence(
        withTiming(PROFILE_SHOULDERS_LEN, { duration: 1 }),
        withTiming(0, { duration: 600, easing: Easing.out(Easing.cubic) }),
      )
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- dash is a stable shared value
  }, [active, trigger])

  const shouldersProps = useAnimatedProps(() => ({
    strokeDasharray: `${PROFILE_SHOULDERS_LEN}`,
    strokeDashoffset: dash.value,
  }))

  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <Path d="M12 3a4.5 4.5 0 1 1 0 9 4.5 4.5 0 0 1 0-9z" />
      <AnimatedPath d={PROFILE_SHOULDERS} animatedProps={shouldersProps} />
    </Svg>
  )
})

const NAV_ITEMS = [
  { id: 'journey', Icon: JourneyIcon },
  { id: 'session', Icon: SessionIcon },
  { id: 'memories', Icon: MemoriesIcon },
  { id: 'profile', Icon: ProfileIcon },
]

export const BottomNav = memo(function BottomNav({ state, navigation }: BottomTabBarProps) {
  useI18n()
  const { colorScheme } = useSettings()
  const { isShrunk, reset } = useScrollNav()
  const safeInsets = useSafeAreaInsets()
  const [clickTriggers, setClickTriggers] = useState<Record<string, number>>({})
  const itemLayouts = useRef<Record<string, { x: number; y: number; width: number; height: number }>>({})

  const innerRef = useRef<View>(null)
  const itemRefs = useRef<Record<string, View>>({})

  // Reanimated shared values for bubble position and scale
  const bubbleX = useSharedValue(0)
  const bubbleY = useSharedValue(0)
  const bubbleWidth = useSharedValue(0)
  const bubbleHeight = useSharedValue(0)
  const scaleAnim = useSharedValue(1)
  const [positionsReady, setPositionsReady] = useState(false)
  const [layoutVersion, setLayoutVersion] = useState(0)

  const BUBBLE_H_PADDING = 6

  // Light frosted pill: inactive items are dark ink (≥5:1 on the material);
  // the active item turns white inside the theme-coloured bubble (≥7:1).
  const activeColor = '#fff'
  const inactiveColor = INK.secondary

  // ── Web: measure positions via getBoundingClientRect. The nav may be
  //    scaled down while shrunk, so we normalise by the current scale to
  //    keep bubble coords in the *unscaled* inner coordinate system.
  const measurePositions = useCallback(() => {
    const innerNode = innerRef.current as unknown as HTMLElement | null
    if (!innerNode) return false

    const innerRect = innerNode.getBoundingClientRect()
    const scale = scaleAnim.value || 1
    let allMeasured = true

    NAV_ITEMS.forEach(item => {
      const itemNode = itemRefs.current[item.id] as unknown as HTMLElement | null
      if (!itemNode) { allMeasured = false; return }
      const r = itemNode.getBoundingClientRect()
      itemLayouts.current[item.id] = {
        x: (r.left - innerRect.left) / scale,
        y: (r.top - innerRect.top) / scale,
        width: r.width / scale,
        height: r.height / scale,
      }
    })

    return allMeasured
  }, [scaleAnim])

  // Initial synchronous measurement after first paint
  useLayoutEffect(() => {
    if (measurePositions()) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- intentional: need positions before paint
      setPositionsReady(true)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once on mount
  }, [])

  // Re-measure on every route change so the bubble always targets fresh coords
  useEffect(() => {
    if (measurePositions()) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- intentional: re-measure after route change
      setLayoutVersion(v => v + 1)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run on every route change
  }, [state.index])

  useEffect(() => {
    if (!positionsReady) return
    const activeId = state.routes[state.index].name
    const layout = itemLayouts.current[activeId]
    if (layout) {
      bubbleX.value = withSpring(layout.x - BUBBLE_H_PADDING / 2, { damping: 35, stiffness: 300, mass: 1 })
      bubbleY.value = withSpring(layout.y, { damping: 35, stiffness: 300, mass: 1 })
      bubbleWidth.value = withSpring(layout.width + BUBBLE_H_PADDING, { damping: 35, stiffness: 300, mass: 1 })
      bubbleHeight.value = withSpring(layout.height, { damping: 35, stiffness: 300, mass: 1 })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- bubble*/state.routes are stable refs
  }, [state.index, positionsReady, layoutVersion])

  useEffect(() => {
    reset()
  }, [state.index, reset])

  useEffect(() => {
    const activeTab = state.routes[state.index].name
    pushTab(activeTab)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only needs to run on route change
  }, [state.index])

  useEffect(() => {
    scaleAnim.value = withSpring(isShrunk ? 0.75 : 1, { damping: 35, stiffness: 250 })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- scaleAnim is a stable shared value
  }, [isShrunk])

  // onLayout kept as fallback for resize detection
  const handleItemLayout = useCallback((id: string, x: number, y: number, width: number, height: number) => {
    const prev = itemLayouts.current[id]
    itemLayouts.current[id] = { x, y, width, height }

    if (Object.keys(itemLayouts.current).length === NAV_ITEMS.length) {
      if (!positionsReady) {
        setPositionsReady(true)
      } else if (prev && (prev.width !== width || prev.x !== x || prev.y !== y || prev.height !== height)) {
        setLayoutVersion(v => v + 1)
      }
    }
  }, [positionsReady])

  const handlePress = useCallback((routeName: string) => {
    setClickTriggers(prev => ({ ...prev, [routeName]: (prev[routeName] || 0) + 1 }))
    navigation.navigate(routeName)
  }, [navigation])

  const bubbleStyle = useAnimatedStyle(() => ({
    transform: [{ translateX: bubbleX.value }, { translateY: bubbleY.value }],
    width: bubbleWidth.value,
    height: bubbleHeight.value,
  }))

  const navStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scaleAnim.value }],
  }))

  const labelOpacityStyle = useAnimatedStyle(() => ({
    opacity: interpolate(scaleAnim.value, [0.75, 1], [0, 1], Extrapolation.CLAMP),
  }))

  const iconStyle = useAnimatedStyle(() => ({
    transform: [
      { scale: interpolate(scaleAnim.value, [0.75, 1], [28 / 26, 1], Extrapolation.CLAMP) },
      { translateY: interpolate(scaleAnim.value, [0.75, 1], [5, 0], Extrapolation.CLAMP) },
    ],
  }))

  return (
    <View style={[styles.container, { paddingBottom: isAndroid ? 34 : (safeInsets.bottom || 8) }]}>
      <Reanimated.View
        style={[styles.nav, navStyle]}
        collapsable={false}
      >
        {/* Light liquid glass: blur + frosted fill. The theme colour lives in
            the active bubble instead of tinting the whole bar. */}
        <BlurView
          intensity={GLASS.intensity}
          tint={GLASS.tint}
          style={[{ pointerEvents: 'none', borderRadius: 40, position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, overflow: 'hidden' }]}
        />
        <View style={[{ pointerEvents: 'none', backgroundColor: GLASS.baseFill, position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }]} />
        <View ref={innerRef} style={styles.inner}>
          <Reanimated.View
            style={[
              styles.bubble,
              { backgroundColor: colorScheme.primary },
              bubbleStyle,
            ]}
          />
          {NAV_ITEMS.map(item => {
            const isActive = state.routes[state.index].name === item.id
            const trigger = clickTriggers[item.id] || 0
            const itemColor = isActive ? activeColor : inactiveColor

            return (
              <TouchableOpacity
                key={item.id}
                ref={(v) => { itemRefs.current[item.id] = v as unknown as View }}
                onLayout={(e) => handleItemLayout(item.id, e.nativeEvent.layout.x, e.nativeEvent.layout.y, e.nativeEvent.layout.width, e.nativeEvent.layout.height)}
                onPress={() => handlePress(item.id)}
                style={styles.item}
                activeOpacity={0.7}
                accessibilityRole="tab"
                accessibilityLabel={i18n.t(`nav_${item.id}`)}
                accessibilityState={{ selected: isActive }}
              >
                <Reanimated.View style={[styles.iconWrapper, iconStyle]}>
                  <item.Icon color={itemColor} active={isActive} trigger={trigger} />
                </Reanimated.View>
                <Reanimated.View style={labelOpacityStyle}>
                  <DisplayText
                    fontSize={8}
                    fontWeight="700"
                    letterSpacing={0.3}
                    textTransform="uppercase"
                    color={itemColor}
                  >
                    {i18n.t(`nav_${item.id}`)}
                  </DisplayText>
                </Reanimated.View>
              </TouchableOpacity>
            )
          })}
        </View>
      </Reanimated.View>
    </View>
  )
})

const styles = StyleSheet.create({
  container: {
    position: 'absolute',
    bottom: 0,
    width: '100%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  nav: {
    alignSelf: 'center',
    maxWidth: WEB_MAX_WIDTH,
    borderRadius: 40,
    borderWidth: GLASS.borderWidth,
    borderColor: GLASS.borderColor,
    overflow: 'hidden',
    boxShadow: '0 20px 45px rgba(0,0,0,0.35)',
    elevation: 30,
    paddingVertical: 8,
    paddingHorizontal: 8
  },
  inner: {
    flexDirection: 'row',
    gap: 16,
    position: 'relative',
  },
  item: {
    minWidth: 64,
    height: 48,
    paddingHorizontal: 12,
    alignItems: 'center',
    justifyContent: 'center',
    position: 'relative',
    zIndex: 1,
    gap: 4
  },
  iconWrapper: {
    alignItems: 'center',
    justifyContent: 'center',
  },
  bubble: {
    position: 'absolute',
    top: 0,
    left: 0,
    // backgroundColor comes from colorScheme.primary at render time — the
    // theme's colour identity now lives here instead of tinting the bar.
    borderRadius: 9999,
  },
})
