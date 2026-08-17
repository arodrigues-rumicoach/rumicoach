/**
 * WCAG 2.1 AA contrast verification for the liquid-glass design system.
 *
 * The theme video runs with NO scrim (iOS-26 style), so legibility is carried
 * entirely by the surfaces. This suite:
 *
 *  1. Extracts real frames from every theme video in assets/theme (fireplace
 *     is near-black with white-hot flame cores, rain is dark green,
 *     sunset_beach has a bright sky, …) via ffmpeg, plus synthetic pure-white
 *     and pure-black worst-case frames.
 *  2. Simulates the exact layer stacks used in the app:
 *       light glass:  video → blur tint rgba(249,249,249,0.312) → white fill
 *       dark  glass:  video → blur tint rgba(25,25,25,0.276)   → black fill
 *     (the tint layers are what expo-blur itself paints at intensity 40 —
 *     the guaranteed minimum across web/Android/iOS; see src/styles/glass.ts)
 *  3. Asserts WCAG AA for every text/UI colour on the growth & profile
 *     screens, bottom nav and top controls: ≥ 4.5:1 for normal text, ≥ 3:1
 *     for large text (≥24px, or ≥18.66px bold) and UI components (1.4.11).
 *
 * Because the app's material is LIGHT glass with dark ink, its worst case is
 * the DARKEST video patch; legacy dark-glass surfaces (auth, streak, session)
 * are worst over the BRIGHTEST patch. Both extremes are checked for every
 * pairing, so no video content — flame core or black night — can break AA.
 *
 * Design rules encoded here:
 *  - No text ever floats bare on the video (there is no scrim to protect it).
 *  - Locked badges are disabled UI → exempt from 1.4.3/1.4.11.
 *  - Life-balance bar fills are redundant with the adjacent "n / 10" text →
 *    decorative under 1.4.11.
 *
 * If ffmpeg is unavailable the video-frame cases are skipped (with a console
 * warning) and the synthetic worst-case floor still runs.
 */
import { describe, expect, it, beforeAll, jest } from '@jest/globals'
import { execFileSync } from 'child_process'
import * as fs from 'fs'
import * as os from 'os'
import * as path from 'path'
import { GLASS, INK, TINT_LIGHT, TINT_DARK } from '../glass'
import { COLOR_SCHEMES } from '../../utils/theme.shared'
import appConfig from '../../../tamagui.config'
 
const { PNG } = require('pngjs')

jest.setTimeout(120000)

// ─── colour math ────────────────────────────────────────────────────────────

type RGB = [number, number, number] // 0..1 sRGB

interface RGBA { rgb: RGB; a: number }

function parseColor(c: string): RGBA {
  const hex = c.match(/^#([0-9a-f]{6})$/i)
  if (hex) {
    const n = parseInt(hex[1], 16)
    return { rgb: [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255], a: 1 }
  }
  const rgba = c.match(/^rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)\s*(?:,\s*([\d.]+)\s*)?\)$/)
  if (rgba) {
    return {
      rgb: [Number(rgba[1]) / 255, Number(rgba[2]) / 255, Number(rgba[3]) / 255],
      a: rgba[4] !== undefined ? Number(rgba[4]) : 1,
    }
  }
  throw new Error(`Unparseable colour: ${c}`)
}

/** Source-over compositing in gamma-encoded sRGB — matches how RN stacks translucent views. */
function over(bg: RGB, fg: RGBA): RGB {
  return [
    fg.a * fg.rgb[0] + (1 - fg.a) * bg[0],
    fg.a * fg.rgb[1] + (1 - fg.a) * bg[1],
    fg.a * fg.rgb[2] + (1 - fg.a) * bg[2],
  ]
}

function channelLin(c: number): number {
  return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
}

function luminance(rgb: RGB): number {
  return 0.2126 * channelLin(rgb[0]) + 0.7152 * channelLin(rgb[1]) + 0.0722 * channelLin(rgb[2])
}

function contrast(a: RGB, b: RGB): number {
  const la = luminance(a)
  const lb = luminance(b)
  const [hi, lo] = la > lb ? [la, lb] : [lb, la]
  return (hi + 0.05) / (lo + 0.05)
}

// ─── frame extraction ───────────────────────────────────────────────────────

const ASSETS = path.resolve(__dirname, '../../../assets/theme')
const VIDEOS = ['fireplace', 'rain', 'lavender', 'mountain_lake', 'sunset_beach']
const STILLS = ['waterfall'] // waterfall theme ships as a poster only

interface Frame {
  theme: string
  /** Per-block average colours — a block approximates a text-line-sized patch. */
  blocks: RGB[]
}

function hasFfmpeg(): boolean {
  try {
    execFileSync('ffmpeg', ['-version'], { stdio: 'ignore' })
    return true
  } catch {
    return false
  }
}

function decodeBlocks(pngBuffer: Buffer, blockSize = 12): RGB[] {
  const png = PNG.sync.read(pngBuffer)
  const blocks: RGB[] = []
  for (let by = 0; by < png.height; by += blockSize) {
    for (let bx = 0; bx < png.width; bx += blockSize) {
      let r = 0
      let g = 0
      let b = 0
      let n = 0
      for (let y = by; y < Math.min(by + blockSize, png.height); y++) {
        for (let x = bx; x < Math.min(bx + blockSize, png.width); x++) {
          const i = (png.width * y + x) << 2
          r += png.data[i]
          g += png.data[i + 1]
          b += png.data[i + 2]
          n++
        }
      }
      blocks.push([r / n / 255, g / n / 255, b / n / 255])
    }
  }
  return blocks
}

function extractFrames(): Frame[] {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'contrast-frames-'))
  const frames: Frame[] = []
  try {
    for (const theme of VIDEOS) {
      const video = path.join(ASSETS, `${theme}.mp4`)
      if (!fs.existsSync(video)) continue
      const pattern = path.join(tmp, `${theme}-%02d.png`)
      // One frame every 2 s, up to 10 frames, downscaled so a 12px block is
      // roughly a text-line-sized patch of the screen.
      execFileSync(
        'ffmpeg',
        ['-i', video, '-vf', 'fps=1/2,scale=192:-1', '-frames:v', '10', '-y', pattern],
        { stdio: 'ignore' },
      )
      for (const f of fs.readdirSync(tmp).filter((f) => f.startsWith(`${theme}-`))) {
        frames.push({ theme, blocks: decodeBlocks(fs.readFileSync(path.join(tmp, f))) })
      }
    }
    for (const theme of STILLS) {
      const still = path.join(ASSETS, `${theme}.jpg`)
      if (!fs.existsSync(still)) continue
      const out = path.join(tmp, `${theme}-01.png`)
      execFileSync('ffmpeg', ['-i', still, '-vf', 'scale=192:-1', '-y', out], { stdio: 'ignore' })
      frames.push({ theme, blocks: decodeBlocks(fs.readFileSync(out)) })
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true })
  }
  return frames
}

/** Brightest and darkest composited patches for a surface stack — a colour
 *  pairing must survive BOTH extremes. */
function extremeBlocks(blocks: RGB[], layers: RGBA[]): { brightest: RGB; darkest: RGB } {
  let brightest: RGB = [0, 0, 0]
  let darkest: RGB = [1, 1, 1]
  let maxLum = -1
  let minLum = 2
  for (const block of blocks) {
    let c = block
    for (const layer of layers) c = over(c, layer)
    const lum = luminance(c)
    if (lum > maxLum) {
      maxLum = lum
      brightest = c
    }
    if (lum < minLum) {
      minLum = lum
      darkest = c
    }
  }
  return { brightest, darkest }
}

/** Min contrast of a (possibly translucent) colour across both extreme patches. */
function minContrast(colour: string, blocks: RGB[], layers: RGBA[]): number {
  const { brightest, darkest } = extremeBlocks(blocks, layers)
  const c = parseColor(colour)
  return Math.min(contrast(over(brightest, c), brightest), contrast(over(darkest, c), darkest))
}

// ─── surfaces and colours under test (keep in sync with the components) ─────

const darkThemeRaw = (appConfig as any).themes.dark as Record<string, unknown>
// createTamagui wraps theme values in Variable objects — unwrap to plain strings.
const themeVal = (v: unknown): string => (typeof v === 'string' ? v : (v as { val: string }).val)
const darkTheme = Object.fromEntries(
  Object.entries(darkThemeRaw).map(([k, v]) => [k, themeVal(v)]),
) as Record<string, string>

const LIGHT_GLASS = [parseColor(TINT_LIGHT), parseColor(GLASS.baseFill)]
const DARK_GLASS = [parseColor(TINT_DARK), parseColor(GLASS.dark.baseFill)]

// Ink on the light material (mirrors $onGlass* / INK)
const ON_LIGHT = {
  primary: darkTheme.onGlass,
  secondary: darkTheme.onGlassSecondary,
  tertiary: darkTheme.onGlassTertiary,
  accent: darkTheme.onGlassAccent, // labels + links
  streakAmber: INK.amber, // streak number (24px bold) + focus label (12.5px bold)
}

// White text on the legacy dark material (auth, streak, session, memory cards)
const ON_DARK = {
  primary: darkTheme.color, // #ffffff
  secondary: darkTheme.colorSecondary, // rgba(255,255,255,0.78)
  tertiary: darkTheme.colorTertiary, // rgba(255,255,255,0.68)
  accentLight: darkTheme.accentLight, // small accent labels on dark glass
}

const UI_ON_LIGHT = {
  checkboxBorder: INK.secondary, // ActionsCard checkbox ring
  checkboxDone: '#065F46', // checked fill (white glyph on top: 7.7:1)
  overdueBorder: '#B91C1C', // overdue ring (also stated in text)
}

// profile.tsx BADGES tints at alpha 0.18 over the light glass — ink label on top
const BADGE_COLORS = ['#f59e0b', '#f97316', '#8b5cf6', '#14b8a6', '#eab308', '#10b981', '#ef4444', '#fbbf24', '#3b82f6']
const STREAK_TILE_TINT = parseColor('rgba(249,115,22,0.14)')

const AA_NORMAL = 4.5
const AA_LARGE = 3.0
const AA_UI = 3.0

// ─── frames ─────────────────────────────────────────────────────────────────

const ffmpegAvailable = hasFfmpeg()

// Synthetic frames prove both floors with zero dependence on video content:
// pure white (worst for dark glass), pure black (worst for light glass),
// plus saturated primaries.
const SYNTHETIC: Frame = {
  theme: 'synthetic-worst-case',
  blocks: [
    [1, 1, 1],
    [0, 0, 0],
    [1, 0, 0],
    [0, 1, 0],
    [0, 0, 1],
    [1, 1, 0],
    [1, 0.5, 0],
  ],
}

const frames: Frame[] = [SYNTHETIC]

beforeAll(() => {
  if (ffmpegAvailable) {
    frames.push(...extractFrames())
    const themes = new Set(frames.map((f) => f.theme))
    // Every shipped theme video must actually be exercised.
    for (const theme of VIDEOS) {
      if (!themes.has(theme)) throw new Error(`No frames extracted for theme "${theme}"`)
    }
  } else {
    console.warn('ffmpeg not found — video-frame contrast cases skipped; synthetic floor still verified.')
  }
})

// ─── tests ──────────────────────────────────────────────────────────────────

describe('light liquid glass (the app material) — no scrim, any video', () => {
  it('ink text keeps AA on plain glass cards', () => {
    for (const frame of frames) {
      const failures: string[] = []
      const check = (name: string, colour: string, min: number) => {
        const ratio = minContrast(colour, frame.blocks, LIGHT_GLASS)
        if (ratio < min) failures.push(`${name} ${ratio.toFixed(2)}:1 < ${min}:1 on ${frame.theme}`)
      }
      check('ink primary (titles, body, stats)', ON_LIGHT.primary, AA_NORMAL)
      check('ink secondary (subtitles, meta, labels)', ON_LIGHT.secondary, AA_NORMAL)
      check('ink tertiary (captions, struck-through)', ON_LIGHT.tertiary, AA_NORMAL)
      check('ink accent (links, insight label)', ON_LIGHT.accent, AA_NORMAL)
      check('focus label deep amber (12.5px bold)', ON_LIGHT.streakAmber, AA_NORMAL)
      check('checkbox border (UI component)', UI_ON_LIGHT.checkboxBorder, AA_UI)
      check('checked checkbox fill (UI component)', UI_ON_LIGHT.checkboxDone, AA_UI)
      check('overdue border (UI, also stated in text)', UI_ON_LIGHT.overdueBorder, AA_UI)
      expect(failures).toEqual([])
    }
  })

  it('streak tile (amber tint) keeps AA', () => {
    for (const frame of frames) {
      const layers = [...LIGHT_GLASS, STREAK_TILE_TINT]
      // 24px bold number → large text; 11.5px subtitle → normal text
      expect(minContrast(ON_LIGHT.streakAmber, frame.blocks, layers)).toBeGreaterThanOrEqual(AA_LARGE)
      expect(minContrast(ON_LIGHT.secondary, frame.blocks, layers)).toBeGreaterThanOrEqual(AA_NORMAL)
    }
  })

  it('earned badge chips (colour tint over glass) keep AA for the ink label', () => {
    for (const frame of frames) {
      for (const badge of BADGE_COLORS) {
        const tint: RGBA = { rgb: parseColor(badge).rgb, a: 0.18 }
        const ratio = minContrast(ON_LIGHT.primary, frame.blocks, [...LIGHT_GLASS, tint])
        // 10px bold label
        expect(`${badge}/${frame.theme}: ${ratio >= AA_NORMAL}`).toBe(`${badge}/${frame.theme}: true`)
      }
    }
  })

  it('bottom nav pill keeps AA: ink inactive items, white-on-theme-colour active bubble', () => {
    for (const frame of frames) {
      // Inactive icons + 8px labels sit directly on the light pill.
      expect(minContrast(INK.secondary, frame.blocks, LIGHT_GLASS)).toBeGreaterThanOrEqual(AA_NORMAL)
      const { brightest, darkest } = extremeBlocks(frame.blocks, LIGHT_GLASS)
      for (const [schemeName, scheme] of Object.entries(COLOR_SCHEMES)) {
        const bubble = parseColor(scheme.primary).rgb
        // Active label/icon is white inside the opaque theme-coloured bubble.
        const active = contrast(parseColor('#ffffff').rgb, bubble)
        expect(`${schemeName} active: ${active >= AA_NORMAL}`).toBe(`${schemeName} active: true`)
        // The bubble itself must read as the selected-state indicator (1.4.11).
        const indicator = Math.min(contrast(bubble, brightest), contrast(bubble, darkest))
        expect(`${schemeName} bubble: ${indicator >= AA_UI}`).toBe(`${schemeName} bubble: true`)
      }
    }
  })

  it('top-control icon buttons keep AA for ink icons', () => {
    for (const frame of frames) {
      expect(minContrast(INK.primary, frame.blocks, LIGHT_GLASS)).toBeGreaterThanOrEqual(AA_NORMAL)
    }
  })

  it('frosted chips (category filter, date pills) keep AA — they float on the video without blur', () => {
    // CategoryFilter chips and ThemedButton "glass" pills use a plain
    // rgba(255,255,255,0.75) fill (no BlurView — too many small instances),
    // so the fill alone must carry AA over the darkest patch of every video.
    const FROST_CHIP = [parseColor('rgba(255,255,255,0.75)')]
    for (const frame of frames) {
      expect(minContrast(INK.primary, frame.blocks, FROST_CHIP)).toBeGreaterThanOrEqual(AA_NORMAL)
    }
  })
})

describe('DailyFocusCard hero (arbitrary session photo + text-protection gradient)', () => {
  // The photo is untouched in its top half; only the bottom band darkens.
  // Floors from the gradient (0 → 0.30@45% → 0.72@100%) at the text zone:
  // ≥0.45 where the title sits, ≥0.58 where the subtitle sits. The photo is
  // arbitrary, so assert over a pure-white worst-case image.
  it('white copy keeps AA over a worst-case bright photo', () => {
    const whiteImage: RGB[] = [[1, 1, 1]]
    const titleZone = [parseColor('rgba(0,0,0,0.45)')]
    const subtitleZone = [parseColor('rgba(0,0,0,0.58)')]
    // 20px bold title → large text; 14px subtitle → normal text
    expect(minContrast('#ffffff', whiteImage, titleZone)).toBeGreaterThanOrEqual(AA_LARGE)
    expect(minContrast('#ffffff', whiteImage, subtitleZone)).toBeGreaterThanOrEqual(AA_NORMAL)
  })
})

describe('legacy dark glass (auth, streak, session panels, memory cards)', () => {
  it('white text keeps AA with no scrim behind the panel', () => {
    for (const frame of frames) {
      const failures: string[] = []
      const check = (name: string, colour: string, min: number) => {
        const ratio = minContrast(colour, frame.blocks, DARK_GLASS)
        if (ratio < min) failures.push(`${name} ${ratio.toFixed(2)}:1 < ${min}:1 on ${frame.theme}`)
      }
      check('white', ON_DARK.primary, AA_NORMAL)
      check('secondary 0.78', ON_DARK.secondary, AA_NORMAL)
      check('tertiary 0.68', ON_DARK.tertiary, AA_NORMAL)
      check('accentLight labels', ON_DARK.accentLight, AA_NORMAL)
      expect(failures).toEqual([])
    }
  })
})

describe('lightness budget — the video stays untouched', () => {
  it('the light material stays frosted, not painted', () => {
    // The white fill may top up the blur tint, but an opaque panel would stop
    // being glass. 0.65 is the ceiling.
    expect(parseColor(GLASS.baseFill).a).toBeLessThanOrEqual(0.65)
    expect(GLASS.tint).toBe('light')
  })

  it('no text may float bare on the video', () => {
    // There is no scrim, so bare text has nothing protecting it: white text
    // dies on sunset sky, dark text dies at night. This documents why every
    // text block sits on glass — including greetings and section labels.
    if (!ffmpegAvailable) return
    const sunset = frames.find((f) => f.theme === 'sunset_beach')!
    const fireplace = frames.find((f) => f.theme === 'fireplace')!
    expect(minContrast('#ffffff', sunset.blocks, [])).toBeLessThan(AA_NORMAL)
    expect(minContrast(INK.primary, fireplace.blocks, [])).toBeLessThan(AA_NORMAL)
  })
})
