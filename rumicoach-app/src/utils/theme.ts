import type { ImageSourcePropType } from 'react-native'
import { getThemeAssetUrl } from './assetManager'
import { COLOR_SCHEMES, type ThemeType, type ThemeAsset } from './theme.shared'

export type { ThemeType }
export { COLOR_SCHEMES }

export const THEME_ASSETS: Record<string, ThemeAsset> = {
  waterfall: {
    image: require('../../assets/theme/waterfall.jpg'),
    video: getThemeAssetUrl('waterfall', 'video'),
    audio: getThemeAssetUrl('waterfall', 'audio'),
    colors: COLOR_SCHEMES.blue,
  },
  fireplace: {
    image: require('../../assets/theme/fireplace.jpg'),
    video: getThemeAssetUrl('fireplace', 'video'),
    audio: getThemeAssetUrl('fireplace', 'audio'),
    colors: COLOR_SCHEMES.brown,
  },
  mountain_lake: {
    image: require('../../assets/theme/mountain_lake.jpg'),
    video: getThemeAssetUrl('mountain_lake', 'video'),
    audio: getThemeAssetUrl('mountain_lake', 'audio'),
    colors: COLOR_SCHEMES.blue,
  },
  rain: {
    image: require('../../assets/theme/rain.jpg'),
    video: getThemeAssetUrl('rain', 'video'),
    audio: getThemeAssetUrl('rain', 'audio'),
    colors: COLOR_SCHEMES.green,
  },
  sunset_beach: {
    image: require('../../assets/theme/sunset_beach.jpg'),
    video: require('../../assets/theme/sunset_beach.mp4'),
    audio: require('../../assets/theme/sunset_beach_audio.m4a'),
    colors: COLOR_SCHEMES.brown,
  },
  lavender: {
    image: require('../../assets/theme/lavender.jpg'),
    video: getThemeAssetUrl('lavender', 'video'),
    audio: getThemeAssetUrl('lavender', 'audio'),
    colors: COLOR_SCHEMES.violet,
  },
}

export function getThemeImageSource(themeId: ThemeType): ImageSourcePropType {
  return THEME_ASSETS[themeId]?.image ?? THEME_ASSETS.sunset_beach.image
}
