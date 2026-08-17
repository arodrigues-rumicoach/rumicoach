import { useState, useEffect } from 'react'
import { getThemeAssetUri, getThemeAssetUrl } from '../utils/assetManager'
import { isWeb } from '../adapters/platform'
import type { ThemeType } from '../utils/theme'

interface ThemeAssetUris {
  imageUri: string
  videoUri: string
  audioUri: string
  isLoading: boolean
}

export function useThemeAssetUri(themeId: ThemeType): ThemeAssetUris {
  const [uris, setUris] = useState<ThemeAssetUris>({
    imageUri: '',
    videoUri: '',
    audioUri: '',
    isLoading: true,
  })

  useEffect(() => {
    let cancelled = false

    async function resolve() {
      if (isWeb) {
        setUris({
          imageUri: getThemeAssetUrl(themeId, 'image'),
          videoUri: getThemeAssetUrl(themeId, 'video'),
          audioUri: getThemeAssetUrl(themeId, 'audio'),
          isLoading: false,
        })
        return
      }

      const [imageUri, videoUri, audioUri] = await Promise.all([
        getThemeAssetUri(themeId, 'image'),
        getThemeAssetUri(themeId, 'video'),
        getThemeAssetUri(themeId, 'audio'),
      ])

      if (!cancelled) {
        setUris({ imageUri, videoUri, audioUri, isLoading: false })
      }
    }

    resolve()

    return () => { cancelled = true }
  }, [themeId])

  return uris
}
