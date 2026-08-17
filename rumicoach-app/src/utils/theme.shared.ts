import type { ImageSourcePropType } from 'react-native'

export type ThemeType = 'lavender' | 'fireplace' | 'mountain_lake' | 'rain' | 'sunset_beach' | 'waterfall'

export interface ColorScheme {
  primary: string
  secondary: string
  tertiary: string
  accent: string
  navIcon: string
  navIconText: string
  navIconBlur: string
  navBlur: string
}

export const COLOR_SCHEMES: Record<string, ColorScheme> = {
  green: {
    primary: '#1a5f4f', secondary: '#d4f0ea', tertiary: '#0d3d32', accent: '#2d8a6e',
    navIcon: '#e0e0e0', navIconText: '#ffffff', navIconBlur: '#1a5f4f80', navBlur: '#1a5f4f60',
  },
  blue: {
    primary: '#1b4f7d', secondary: '#d4e8f5', tertiary: '#0d3254', accent: '#2d6a9e',
    navIcon: '#d0d0d0', navIconText: '#ffffff', navIconBlur: '#1b4f7d80', navBlur: '#1b4f7d60',
  },
  brown: {
    primary: '#5e3b08', secondary: '#f5e6d3', tertiary: '#3d2605', accent: '#8b5a2b',
    navIcon: '#cccccc', navIconText: '#ffffff', navIconBlur: '#5e3b0880', navBlur: '#5e3b0860',
  },
  violet: {
    primary: '#6e4060', secondary: '#e8d4dc', tertiary: '#4a2a42', accent: '#9e6b87',
    navIcon: '#c8c8c8', navIconText: '#ffffff', navIconBlur: '#6e406080', navBlur: '#6e406060',
  },
}

export interface ThemeAsset {
  image: ImageSourcePropType
  video: any
  audio: any
  colors: ColorScheme
}
