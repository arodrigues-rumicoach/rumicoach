'use worklet'

export const MAX_SCORE = 10

export const PETAL_COLORS = [
  '#F87171',
  '#FB923C',
  '#FBBF24',
  '#A3E635',
  '#34D399',
  '#22D3EE',
  '#60A5FA',
  '#818CF8',
  '#C084FC',
  '#F472B6',
]

export const withAlpha = (hex: string, alpha: number): string => {
  'worklet'
  const a = Math.round(Math.min(Math.max(alpha, 0), 1) * 255)
    .toString(16)
    .padStart(2, '0')
  return hex + a
}

export const polarToCartesian = (
  cx: number,
  cy: number,
  r: number,
  angleDeg: number,
): { x: number; y: number } => {
  'worklet'
  const rad = (angleDeg * Math.PI) / 180
  return {
    x: cx + r * Math.sin(rad),
    y: cy - r * Math.cos(rad),
  }
}

const computeCornerRadius = (
  r: number,
  spanDeg: number,
): number => {
  'worklet'
  if (r <= 0 || spanDeg <= 0) return 0
  const arcWidth = r * (spanDeg * Math.PI) / 180
  return Math.min(arcWidth * 0.25, r * 0.15)
}

export const buildPetalPath = (
  cx: number,
  cy: number,
  r: number,
  angleStart: number,
  angleSweep: number,
  gapDeg: number = 1,
): string => {
  'worklet'
  if (r <= 0) return 'M 0 0'

  const a0 = angleStart + gapDeg
  const a1 = angleStart + angleSweep - gapDeg
  const span = a1 - a0
  if (span <= 0) return 'M 0 0'

  const cr = computeCornerRadius(r, span)
  const cornerInsetDeg = cr > 0 ? (cr / r) * (180 / Math.PI) : 0

  const arcA0 = a0 + cornerInsetDeg
  const arcA1 = a1 - cornerInsetDeg
  if (arcA1 <= arcA0) return 'M 0 0'

  const rInner = Math.max(r - cr, 0)
  const p0 = polarToCartesian(cx, cy, rInner, a0)
  const pCorner0 = polarToCartesian(cx, cy, r, a0)
  const pArc0 = polarToCartesian(cx, cy, r, arcA0)
  const pArc1 = polarToCartesian(cx, cy, r, arcA1)
  const pCorner1 = polarToCartesian(cx, cy, r, a1)
  const p1 = polarToCartesian(cx, cy, rInner, a1)

  const f = (v: number) => v.toFixed(2)
  return (
    `M ${f(cx)} ${f(cy)} ` +
    `L ${f(p0.x)} ${f(p0.y)} ` +
    `Q ${f(pCorner0.x)} ${f(pCorner0.y)} ${f(pArc0.x)} ${f(pArc0.y)} ` +
    `A ${f(r)} ${f(r)} 0 0 1 ${f(pArc1.x)} ${f(pArc1.y)} ` +
    `Q ${f(pCorner1.x)} ${f(pCorner1.y)} ${f(p1.x)} ${f(p1.y)} ` +
    'Z'
  )
}

export const buildTrackPath = (
  cx: number,
  cy: number,
  maxR: number,
  angleStart: number,
  angleSweep: number,
): string => {
  'worklet'
  return buildPetalPath(cx, cy, maxR, angleStart, angleSweep, 0)
}

export const buildFullCircle = (cx: number, cy: number, r: number): string => {
  'worklet'
  if (r <= 0) return 'M 0 0'
  const f = (v: number) => v.toFixed(2)
  const top = polarToCartesian(cx, cy, r, 0)
  const bottom = polarToCartesian(cx, cy, r, 180)
  return (
    `M ${f(top.x)} ${f(top.y)} ` +
    `A ${f(r)} ${f(r)} 0 1 1 ${f(bottom.x)} ${f(bottom.y)} ` +
    `A ${f(r)} ${f(r)} 0 1 1 ${f(top.x)} ${f(top.y)} Z`
  )
}

export const buildRingPath = (cx: number, cy: number, r: number): string => {
  'worklet'
  return buildFullCircle(cx, cy, r)
}

export const buildLabelArcPath = (
  cx: number,
  cy: number,
  r: number,
  angleStart: number,
  angleEnd: number,
  clockwise: boolean,
): string => {
  'worklet'
  const f = (v: number) => v.toFixed(2)
  const start = polarToCartesian(cx, cy, r, angleStart)
  const end = polarToCartesian(cx, cy, r, angleEnd)
  // Sweep flag selects traversal direction on the same circle. Swapping
  // endpoints instead would mirror the arc onto the wrong (inner) circle.
  const sweep = clockwise ? 1 : 0
  return `M ${f(start.x)} ${f(start.y)} A ${f(r)} ${f(r)} 0 0 ${sweep} ${f(end.x)} ${f(end.y)}`
}

export const easeOutCubic = (t: number): number => {
  'worklet'
  return 1 - Math.pow(1 - t, 3)
}

export const computeWedgeIntro = (
  introProgress: number,
  index: number,
  totalWedges: number,
): number => {
  'worklet'
  const stagger = totalWedges > 1 ? index / (totalWedges - 1) : 0
  const t = (introProgress - stagger * 0.5) / 0.5
  return easeOutCubic(Math.min(Math.max(t, 0), 1))
}

export const computeLabelRadius = (
  cx: number,
  cy: number,
  canvasSize: number,
  angleDeg: number,
  textWidth: number,
  fontSize: number,
): number => {
  'worklet'
  const rad = (angleDeg * Math.PI) / 180
  const xDir = Math.abs(Math.sin(rad))
  const yDir = Math.abs(Math.cos(rad))

  let maxDist = canvasSize / 2 - fontSize
  if (xDir > 0.01) {
    maxDist = Math.min(maxDist, (canvasSize / 2 - textWidth - 4) / xDir)
  }
  if (yDir > 0.01) {
    maxDist = Math.min(maxDist, (canvasSize / 2 - 8) / yDir)
  }

  return Math.max(Math.min(maxDist, 200), 0)
}

export const truncateLabel = (name: string, maxLen: number = 14): string => {
  'worklet'
  if (name.length <= maxLen) return name
  return name.slice(0, maxLen - 1) + '…'
}
