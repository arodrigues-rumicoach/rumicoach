import { memo, useEffect, useMemo, useCallback, useState } from 'react'
import { View, type LayoutChangeEvent } from 'react-native'
import {
  Canvas,
  Path,
  Circle,
  Text as SkiaText,
  TextPath,
  useFont,
  vec,
  RadialGradient,
  DashPathEffect,
} from '@shopify/react-native-skia'
import {
  useSharedValue,
  withSpring,
  useDerivedValue,
  useReducedMotion,
  type SharedValue,
} from 'react-native-reanimated'
import type { WheelCategory } from '@/api'
import { useSettings } from '@/hooks/useSettings'
import {
  MAX_SCORE,
  PETAL_COLORS,
  polarToCartesian,
  buildPetalPath,
  buildTrackPath,
  buildFullCircle,
  buildRingPath,
  buildLabelArcPath,
  withAlpha,
  computeWedgeIntro,
} from './wheelMath'

interface WheelOfLifeChartProps {
  data: WheelCategory[]
  size?: number
}

const SPRING_CONFIG = {
  damping: 14,
  stiffness: 90,
  mass: 1,
}

const PETAL_GAP_DEG = 1
const LABEL_PAD = 56
const LABEL_FONT_SIZE = 14
const SCORE_FONT_SIZE = 12
const SCORE_INSET = 16

interface PetalWedgeProps {
  index: number
  totalWedges: number
  scores: SharedValue<number[]>
  introProgress: SharedValue<number>
  cx: number
  cy: number
  radiusStep: number
  sliceAngle: number
  color: string
  maxRadius: number
}

const PetalWedge = memo(function PetalWedge({
  index,
  totalWedges,
  scores,
  introProgress,
  cx,
  cy,
  radiusStep,
  sliceAngle,
  color,
  maxRadius,
}: PetalWedgeProps) {
  const dPath = useDerivedValue(() => {
    const intro = computeWedgeIntro(introProgress.value, index, totalWedges)
    const r = Math.min(
      Math.max(scores.value[index] * radiusStep * intro, 0),
      maxRadius,
    )
    if (r < 0.5) return 'M 0 0'
    return buildPetalPath(
      cx,
      cy,
      r,
      index * sliceAngle,
      sliceAngle,
      PETAL_GAP_DEG,
    )
  }, [scores, introProgress, index, totalWedges, cx, cy, radiusStep, sliceAngle, maxRadius])

  return (
    <Path path={dPath} opacity={0.93}>
      <RadialGradient
        c={vec(cx, cy)}
        r={maxRadius}
        colors={[
          withAlpha(color, 0.18),
          withAlpha(color, 0.55),
          withAlpha(color, 0.95),
        ]}
        positions={[0, 0.5, 1]}
      />
    </Path>
  )
})

interface TipLabelProps {
  index: number
  totalWedges: number
  scores: SharedValue<number[]>
  introProgress: SharedValue<number>
  cx: number
  cy: number
  radiusStep: number
  sliceAngle: number
  font: ReturnType<typeof useFont>
  scoreText: string
  rawScore: number
}

const TipLabel = memo(function TipLabel({
  index,
  totalWedges,
  scores,
  introProgress,
  cx,
  cy,
  radiusStep,
  sliceAngle,
  font,
  scoreText,
  rawScore,
}: TipLabelProps) {
  const angle = index * sliceAngle + sliceAngle / 2
  const maxRadius = MAX_SCORE * radiusStep

  const dX = useDerivedValue(() => {
    const intro = computeWedgeIntro(introProgress.value, index, totalWedges)
    const r = Math.min(scores.value[index] * radiusStep * intro, maxRadius)
    if (r < 2) return -100
    return polarToCartesian(cx, cy, Math.max(r - SCORE_INSET, 0), angle).x
  }, [scores, introProgress, index, totalWedges, cx, cy, radiusStep, angle, maxRadius])

  const dY = useDerivedValue(() => {
    const intro = computeWedgeIntro(introProgress.value, index, totalWedges)
    const r = Math.min(scores.value[index] * radiusStep * intro, maxRadius)
    if (r < 2) return -100
    return polarToCartesian(cx, cy, Math.max(r - SCORE_INSET, 0), angle).y + SCORE_FONT_SIZE * 0.35
  }, [scores, introProgress, index, totalWedges, cx, cy, radiusStep, angle, maxRadius])

  const dOpacity = useDerivedValue(() => {
    const intro = computeWedgeIntro(introProgress.value, index, totalWedges)
    if (intro < 0.2) return 0
    const r = scores.value[index] * radiusStep * intro
    if (r < 2) return 0
    return Math.min((intro - 0.2) / 0.4, 1)
  }, [scores, introProgress, index, totalWedges, radiusStep])

  if (!font || rawScore <= 0) return null

  return (
    <SkiaText
      font={font}
      text={scoreText}
      x={dX}
      y={dY}
      color="white"
      opacity={dOpacity}
    />
  )
})

export default function WheelOfLifeChart({
  data,
  size = 260,
}: WheelOfLifeChartProps) {
  const { colorScheme } = useSettings()
  const n = data.length
  const reducedMotion = useReducedMotion()
  const [layoutWidth, setLayoutWidth] = useState(0)

  const onLayout = useCallback((e: LayoutChangeEvent) => {
    const w = e.nativeEvent.layout.width
    if (w > 0) setLayoutWidth(w)
  }, [])

  const canvasSize = layoutWidth > 0 ? layoutWidth : size
  const cx = canvasSize / 2
  const cy = canvasSize / 2
  const maxRadius = canvasSize / 2 - LABEL_PAD
  const sliceAngle = 360 / n
  const radiusStep = maxRadius / MAX_SCORE

  const scores = useSharedValue<number[]>(data.map(() => 0))
  const introProgress = useSharedValue(0)

  useEffect(() => {
    if (reducedMotion) {
      scores.value = data.map((d) => d.currentScore)
      introProgress.value = 1
      return
    }
    scores.value = withSpring(
      data.map((d) => d.currentScore),
      SPRING_CONFIG,
    )
    introProgress.value = withSpring(1, {
      damping: 20,
      stiffness: 100,
      mass: 0.5,
    })
  }, [data, reducedMotion])

  const font = useFont(
    require('../../../assets/fonts/Manrope-SemiBold.ttf'),
    LABEL_FONT_SIZE,
  )
  const fontBold = useFont(
    require('../../../assets/fonts/Manrope-Bold.ttf'),
    SCORE_FONT_SIZE,
  )

  const colors = useMemo(
    () => data.map((_, i) => PETAL_COLORS[i % PETAL_COLORS.length]),
    [data],
  )

  const trackPaths = useMemo(
    () =>
      Array.from({ length: n }, (_, i) => ({
        path: buildTrackPath(cx, cy, maxRadius, i * sliceAngle, sliceAngle),
        color: colors[i],
      })),
    [n, cx, cy, maxRadius, sliceAngle, colors],
  )

  const rings = useMemo(() => [2, 4, 6, 8, 10], [])

  const targetArcs = useMemo(() => {
    return data
      .map((cat, i) => {
        if (cat.targetScore == null || cat.targetScore <= 0) return null
        const r = cat.targetScore * radiusStep
        if (r <= 0) return null
        const startAngle = i * sliceAngle + 1
        const endAngle = (i + 1) * sliceAngle - 1
        if (endAngle <= startAngle) return null
        const a0 = polarToCartesian(cx, cy, r, startAngle)
        const a1 = polarToCartesian(cx, cy, r, endAngle)
        const f = (v: number) => v.toFixed(2)
        return {
          path: `M ${f(a0.x)} ${f(a0.y)} A ${f(r)} ${f(r)} 0 0 1 ${f(a1.x)} ${f(a1.y)}`,
        }
      })
      .filter(Boolean) as { path: string }[]
  }, [data, cx, cy, radiusStep, sliceAngle])

  const labelArcs = useMemo(() => {
    const labelRadius = maxRadius + 22

    return data.map((cat, i) => {
      const startAngle = i * sliceAngle
      const endAngle = (i + 1) * sliceAngle
      const midAngle = startAngle + sliceAngle / 2

      // Top half: path goes start→end (CW) — text reads L→R outside
      // Bottom half: path goes end→start (CCW) — text reads L→R outside
      const isBottomHalf = midAngle > 90 && midAngle < 270
      const pathStart = isBottomHalf ? endAngle : startAngle
      const pathEnd = isBottomHalf ? startAngle : endAngle

      let name = cat.name
      const arcLength = labelRadius * (sliceAngle * Math.PI) / 180
      if (font) {
        const textWidth = font.measureText(name).width
        if (textWidth > arcLength - 8) {
          while (name.length > 3 && font.measureText(name + '…').width > arcLength - 8) {
            name = name.slice(0, -1)
          }
          name = name + '…'
        }
      }

      const textWidth = font ? font.measureText(name).width : name.length * 6
      const initialOffset = Math.max((arcLength - textWidth) / 2, 0)

      const path = buildLabelArcPath(
        cx,
        cy,
        labelRadius,
        pathStart,
        pathEnd,
        !isBottomHalf,
      )

      return { name, path, initialOffset }
    })
  }, [data, cx, cy, maxRadius, sliceAngle, font])

  const scoreTexts = useMemo(
    () => data.map((d) => String(Math.round(d.currentScore))),
    [data],
  )

  if (n === 0) return null

  return (
    <View
      onLayout={onLayout}
      style={{ width: '100%', height: canvasSize, alignItems: 'center' }}
      accessible
      accessibilityRole="image"
      accessibilityLabel={`Wheel of Life chart with ${n} categories`}
    >
      <Canvas style={{ width: canvasSize, height: canvasSize }}>
        {/* Track wedges (gapless background wheel) */}
        {trackPaths.map((t, i) => (
          <Path key={`track-${i}`} path={t.path} opacity={0.22}>
            <RadialGradient
              c={vec(cx, cy)}
              r={maxRadius}
              colors={[
                withAlpha(t.color, 0.05),
                withAlpha(t.color, 0.18),
              ]}
              positions={[0.2, 1]}
            />
          </Path>
        ))}

        {/* Grid rings */}
        {rings.map((level) => {
          const r = level * radiusStep
          return (
            <Path
              key={`ring-${level}`}
              path={buildRingPath(cx, cy, r)}
              color="#c1c1c1ff"
              opacity={level === 10 ? 0.24 : 0.07}
              strokeWidth={level === 10 ? 1 : 0.5}
              style="stroke"
            />
          )
        })}

        {/* Outer wheel frame ring */}
        <Path
          path={buildFullCircle(cx, cy, maxRadius)}
          color="white"
          opacity={0.35}
          strokeWidth={1.5}
          style="stroke"
        />

        {/* Animated petals */}
        {data.map((_, i) => (
          <PetalWedge
            key={`petal-${i}`}
            index={i}
            totalWedges={n}
            scores={scores}
            introProgress={introProgress}
            cx={cx}
            cy={cy}
            radiusStep={radiusStep}
            sliceAngle={sliceAngle}
            color={colors[i]}
            maxRadius={maxRadius}
          />
        ))}

        {/* Target arcs */}
        {targetArcs.map((t, i) => (
          <Path
            key={`target-${i}`}
            path={t.path}
            color="white"
            opacity={0.6}
            strokeWidth={2}
            style="stroke"
            strokeCap="round"
          >
            <DashPathEffect intervals={[5, 5]} />
          </Path>
        ))}

        {/* Score tip numbers */}
        {data.map((cat, i) => (
          <TipLabel
            key={`tip-${i}`}
            index={i}
            totalWedges={n}
            scores={scores}
            introProgress={introProgress}
            cx={cx}
            cy={cy}
            radiusStep={radiusStep}
            sliceAngle={sliceAngle}
            font={fontBold}
            scoreText={scoreTexts[i]}
            rawScore={Math.round(cat.currentScore)}
          />
        ))}

        {/* Category labels along arc */}
        {font &&
          labelArcs.map((l, i) => (
            <TextPath
              key={`label-${i}`}
              font={font}
              text={l.name}
              path={l.path}
              initialOffset={l.initialOffset}
              color="white"
            />
          ))}

        {/* Center hub */}
        <Circle
          cx={cx}
          cy={cy}
          r={4}
          color="white"
          opacity={0.55}
        />
        <Circle
          cx={cx}
          cy={cy}
          r={2.5}
          color={colorScheme.primary}
          opacity={0.85}
        />
      </Canvas>
    </View>
  )
}
