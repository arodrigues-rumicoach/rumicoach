import type { ImageSourcePropType } from 'react-native'
import { getThemeAssetUrl } from './assetManager'
import { COLOR_SCHEMES, type ThemeType, type ThemeAsset, type ColorScheme } from './theme.shared'

export type { ThemeType, ColorScheme }
export { COLOR_SCHEMES }

export interface ThemeAsset {
  image: ImageSourcePropType
  video: any
  audio: any
  colors: ColorScheme
}

export const THEME_ASSETS: Record<string, ThemeAsset> = {
  waterfall: {
    image: getThemeAssetUrl('waterfall', 'image'),
    video: getThemeAssetUrl('waterfall', 'video'),
    audio: getThemeAssetUrl('waterfall', 'audio'),
    colors: COLOR_SCHEMES.blue,
  },
  fireplace: {
    image: getThemeAssetUrl('fireplace', 'image'),
    video: getThemeAssetUrl('fireplace', 'video'),
    audio: getThemeAssetUrl('fireplace', 'audio'),
    colors: COLOR_SCHEMES.brown,
  },
  mountain_lake: {
    image: getThemeAssetUrl('mountain_lake', 'image'),
    video: getThemeAssetUrl('mountain_lake', 'video'),
    audio: getThemeAssetUrl('mountain_lake', 'audio'),
    colors: COLOR_SCHEMES.blue,
  },
  rain: {
    image: getThemeAssetUrl('rain', 'image'),
    video: getThemeAssetUrl('rain', 'video'),
    audio: getThemeAssetUrl('rain', 'audio'),
    colors: COLOR_SCHEMES.green,
  },
  sunset_beach: {
    image: getThemeAssetUrl('sunset_beach', 'image'),
    video: getThemeAssetUrl('sunset_beach', 'video'),
    audio: getThemeAssetUrl('sunset_beach', 'audio'),
    colors: COLOR_SCHEMES.brown,
  },
  lavender: {
    image: getThemeAssetUrl('lavender', 'image'),
    video: getThemeAssetUrl('lavender', 'video'),
    audio: getThemeAssetUrl('lavender', 'audio'),
    colors: COLOR_SCHEMES.violet,
  },
}

export function getThemeImageSource(themeId: ThemeType): ImageSourcePropType {
  return { uri: getThemeAssetUrl(themeId, 'image') }
}
