// Unified "liquid glass" surface tokens — iOS-26-style frosted material.
//
// The theme video runs at FULL brightness: there is no scrim over it at all.
// Legibility comes entirely from the surfaces, the way the iOS home screen
// does it: bright frosted panels with dark warm ink, and no text ever floats
// bare on the video.
//
// Why light glass is the accessible way to get "no overlay"
// ─────────────────────────────────────────────────────────
// Dark ink on a white-frosted panel is at its worst when the video behind is
// pitch black — and over a black frame the panel is simply white × its fill,
// a floor that no video content can break. So the panels alone guarantee AA
// and the video needs no veil. (White text has the opposite physics: its
// worst case is a bright frame, which forces global darkening — the old
// design's death spiral.)
//
// expo-blur's tint="light" paints rgba(249,249,249, intensity/100 × 0.78)
// (same factor on web and Android) — rgba(249,249,249,0.312) at intensity 40,
// modelled as TINT_LIGHT in the tests. baseFill tops it up so that over a
// pure-black frame the panel keeps:
//   INK.primary   7.7:1   INK.secondary 5.6:1   INK.tertiary 4.6:1
// Verified against real frames from every theme video in
// src/styles/__tests__/contrast.test.ts.
export const GLASS = {
  intensity: 40,
  tint: 'light' as const,
  borderWidth: 1,
  borderColor: 'rgba(255,255,255,0.70)',
  /** Frosted-white top-up over the blur's own tint. */
  baseFill: 'rgba(255,255,255,0.58)',
  pressedFill: 'rgba(0,0,0,0.06)',
  /** Hairline between rows that share one glass panel. */
  separator: 'rgba(0,0,0,0.10)',
  /** Legacy dark material for screens not yet migrated (auth, streak,
   *  session panels, memory cards). Self-sufficient without any scrim:
   *  white/0.78/0.68 text keeps ≥4.5:1 even over a pure-white frame. */
  dark: {
    tint: 'dark' as const,
    borderColor: 'rgba(255,255,255,0.15)',
    baseFill: 'rgba(0,0,0,0.58)',
    pressedFill: 'rgba(255,255,255,0.10)',
  },
  radius: {
    row: 14, // rows, stat tiles, badge chips
    card: 20, // standard content cards
    panel: 24, // large session panels
    pill: 999, // toolbars, chips, nav bubble
  },
} as const

/** Warm dark ink for light glass. Mirrors the $onGlass* Tamagui tokens —
 *  use these in StyleSheets, the tokens in Tamagui props. */
export const INK = {
  primary: '#262220',
  secondary: '#3D3831',
  tertiary: '#4A4540',
  /** Deep green for links/labels on light glass (4.9:1). */
  accent: '#054C38',
  /** Deep amber for the streak identity on light glass (4.6:1). */
  amber: '#7C2D12',
} as const

/** Guaranteed minimum backdrops expo-blur paints at GLASS.intensity —
 *  referenced by the contrast tests, never applied manually. */
export const TINT_LIGHT = 'rgba(249,249,249,0.312)'
export const TINT_DARK = 'rgba(25,25,25,0.276)'
