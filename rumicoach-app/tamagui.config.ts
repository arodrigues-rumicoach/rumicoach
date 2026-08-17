import { createTamagui, createTokens, createFont } from '@tamagui/core'
import { animations } from '@tamagui/config/v5-rn'
import { tokens as v5Tokens } from '@tamagui/config/v5'

const isWeb = process.env.TAMAGUI_TARGET === 'web'

// Typography stack:
// - Manrope (sans-serif) for body text, UI elements, navigation, buttons,
//   headings, blockquotes, and decorative/display text.
// On web the fonts are loaded from Google Fonts in src/styles/global.css and
// app/+html.tsx. On native the static TTF files are linked via expo-font and
// loaded in app/_layout.tsx.
const bodyFontFamily = isWeb
  ? 'Manrope, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif'
  : 'Manrope'

const headingFont = createFont({
  family: bodyFontFamily,
  size: {
    1: 12, 2: 14, 3: 16, 4: 18, 5: 20, 6: 24, 7: 28, 8: 32, 9: 40, 10: 48,
  },
  lineHeight: { 1: 16, 2: 20, 3: 22, 4: 24, 5: 28, 6: 32, 7: 36, 8: 40, 9: 48, 10: 56 },
  weight: { 1: '100', 2: '200', 3: '300', 4: '400', 5: '500', 6: '600', 7: '700', 8: '800', 9: '900' },
  face: isWeb ? undefined : {
    400: { normal: 'Manrope-Regular' },
    600: { normal: 'Manrope-SemiBold' },
    700: { normal: 'Manrope-Bold' },
  },
})

const bodyFont = createFont({
  family: bodyFontFamily,
  size: {
    1: 11, 2: 12, 3: 13, 4: 14, 5: 15, 6: 16, 7: 18, 8: 20, 9: 24, 10: 32,
  },
  lineHeight: { 1: 16, 2: 18, 3: 20, 4: 22, 5: 24, 6: 26, 7: 28, 8: 30, 9: 34, 10: 40 },
  weight: { 1: '100', 2: '200', 3: '300', 4: '400', 5: '500', 6: '600', 7: '700', 8: '800', 9: '900' },
  face: isWeb ? undefined : {
    400: { normal: 'Manrope-Regular' },
    600: { normal: 'Manrope-SemiBold' },
    700: { normal: 'Manrope-Bold' },
  },
})

const appConfig = createTamagui({
  animations,
  tokens: v5Tokens,
  themes: {
    dark: {
      background: '#000000',
      backgroundSecondary: '#0a0a0a',
      backgroundTertiary: '#1a1a1a',
      color: '#ffffff',
      // 0.78 / 0.68 (was 0.7 / 0.5) keep small text ≥ 4.5:1 over the darkest
      // glass surface can get when the video behind it is fully bright —
      // verified in src/styles/__tests__/contrast.test.ts.
      colorSecondary: 'rgba(255,255,255,0.78)',
      colorTertiary: 'rgba(255,255,255,0.68)',
      accent: '#10b981',
      // Light enough for ≥ 4.5:1 small text on dark surfaces; $accent itself
      // is reserved for fills/graphics, not text.
      accentLight: '#6ee7b7',
      accentDark: '#047857',
      // ── Ink for the light liquid-glass material ──
      // Warm charcoals; ratios over the worst case (light glass on a black
      // video frame) are 7.7 / 5.6 / 4.6 / 4.9 — see src/styles/glass.ts.
      onGlass: '#262220',
      onGlassSecondary: '#3D3831',
      onGlassTertiary: '#4A4540',
      onGlassAccent: '#054C38',
      border: 'rgba(255,255,255,0.1)',
      borderLight: 'rgba(255,255,255,0.08)',
      error: '#ef4444',
      success: '#10b981',
      surfaceOverlay: 'rgba(0,0,0,0.8)',
      surfaceCard: 'rgba(255,255,255,0.1)',
    },
    light: {
      background: '#ffffff',
      backgroundSecondary: '#f5f5f5',
      backgroundTertiary: '#e5e5e5',
      color: '#000000',
      colorSecondary: 'rgba(0,0,0,0.7)',
      colorTertiary: 'rgba(0,0,0,0.5)',
      accent: '#10b981',
      accentLight: '#34d399',
      accentDark: '#047857',
      onGlass: '#262220',
      onGlassSecondary: '#3D3831',
      onGlassTertiary: '#4A4540',
      onGlassAccent: '#054C38',
      border: 'rgba(0,0,0,0.1)',
      borderLight: 'rgba(0,0,0,0.08)',
      error: '#ef4444',
      success: '#10b981',
      surfaceOverlay: 'rgba(0,0,0,0.8)',
      surfaceCard: 'rgba(0,0,0,0.05)',
    },
  },
  fonts: { heading: headingFont, body: bodyFont },
  settings: { defaultFont: 'body' },
  shorthands: {
    px: 'paddingHorizontal',
    py: 'paddingVertical',
    m: 'margin',
    p: 'padding',
  },
})

export type AppConfig = typeof appConfig

declare module '@tamagui/core' {
  interface TamaguiCustomConfig extends AppConfig { }
}

export default appConfig
