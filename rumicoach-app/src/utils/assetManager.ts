import { isWeb } from '../adapters/platform'

const CDN_BASE_URL = process.env.EXPO_PUBLIC_CDN_URL || ''
const CDN_ROOT = CDN_BASE_URL.replace(/\/$/, '')

const THEME_FILE_MAP: Record<string, Record<string, string>> = {
  lavender:       { image: 'lavender.jpg',         video: 'lavender.mp4',         audio: 'lavender.m4a' },
  fireplace:      { image: 'fireplace.jpg',         video: 'fireplace.mp4',         audio: 'fireplace.m4a' },
  mountain_lake:  { image: 'mountain_lake.jpg',     video: 'mountain_lake.mp4',     audio: 'mountain_lake.m4a' },
  rain:           { image: 'rain.jpg',              video: 'rain.mp4',              audio: 'rain.m4a' },
  sunset_beach:   { image: 'sunset_beach.jpg',      video: 'sunset_beach.mp4',      audio: 'sunset_beach.m4a' },
  waterfall:      { image: 'waterfall.jpg',          video: 'waterfall.mp4',          audio: 'waterfall.m4a' },
}

export const CDN_FOLDER_MAP: Record<string, string> = {
  fireplace: 'fireplace',
  lavender: 'lavender',
  mountain_lake: 'mountain_lake',
  rain: 'rain',
  sunset_beach: 'sunset_beach',
  waterfall: 'waterfall',
}

export const CDN_THEME_URLS: Record<string, { image: string; video: string; audio: string }> = {}
for (const [themeId, files] of Object.entries(THEME_FILE_MAP)) {
  const cdnFolder = CDN_FOLDER_MAP[themeId] || themeId
  CDN_THEME_URLS[themeId] = {
    image: `${CDN_ROOT}/themes/${cdnFolder}/${files.image}`,
    video: `${CDN_ROOT}/themes/${cdnFolder}/${files.video}`,
    audio: `${CDN_ROOT}/themes/${cdnFolder}/${files.audio}`,
  }
}

export function getThemeAssetUrl(themeId: string, assetType: 'image' | 'video' | 'audio'): string {
  const urls = CDN_THEME_URLS[themeId] ?? CDN_THEME_URLS.waterfall
  return urls[assetType]
}

let cacheDir: any | null = null

async function getCacheDir() {
  if (cacheDir) return cacheDir
  if (isWeb) return null
  const { Directory, Paths } = await import('expo-file-system')
  const dir = new Directory(Paths.cache, 'theme_assets')
  try { dir.create({ intermediates: true, idempotent: true }) } catch {}
  cacheDir = dir
  return dir
}

function getLocalFileName(themeId: string, assetType: 'image' | 'video' | 'audio'): string {
  const files = THEME_FILE_MAP[themeId] ?? THEME_FILE_MAP.waterfall
  return `theme_${themeId}_${files[assetType]}`
}

async function getLocalFile(themeId: string, assetType: 'image' | 'video' | 'audio') {
  const { File } = await import('expo-file-system')
  const dir = await getCacheDir()
  return new File(dir, getLocalFileName(themeId, assetType))
}

export async function isAssetCached(themeId: string, assetType: 'image' | 'video' | 'audio'): Promise<boolean> {
  if (isWeb) return false
  const file = await getLocalFile(themeId, assetType)
  return file.exists
}

export async function downloadAsset(themeId: string, assetType: 'image' | 'video' | 'audio'): Promise<string> {
  if (isWeb) return getThemeAssetUrl(themeId, assetType)

  const url = getThemeAssetUrl(themeId, assetType)
  if (!url) return ''

  const file = await getLocalFile(themeId, assetType)
  if (file.exists) return file.uri

  try {
    const { File } = await import('expo-file-system')
    const downloaded = await File.downloadFileAsync(url, file, { idempotent: true })
    return downloaded.uri
  } catch {
    return ''
  }
}

export async function getThemeAssetUri(themeId: string, assetType: 'image' | 'video' | 'audio'): Promise<string> {
  if (isWeb) return getThemeAssetUrl(themeId, assetType)

  const file = await getLocalFile(themeId, assetType)
  if (file.exists) return file.uri

  return getThemeAssetUrl(themeId, assetType)
}

export async function ensureThemeAssets(themeId: string): Promise<void> {
  if (isWeb) return
  const assetTypes: ('image' | 'video' | 'audio')[] = ['image', 'video', 'audio']
  await Promise.allSettled(assetTypes.map(t => downloadAsset(themeId, t)))
}

export async function clearOldCache(maxAgeDays: number = 30): Promise<void> {
  if (isWeb) return
  const { File } = await import('expo-file-system')
  const dir = await getCacheDir()
  if (!dir) return
  const cutoff = Date.now() - maxAgeDays * 24 * 60 * 60 * 1000

  try {
    const entries = dir.list()
    for (const entry of entries) {
      if (entry instanceof File && entry.exists) {
        const modTime = entry.modificationTime
        if (modTime && modTime < cutoff) {
          entry.delete()
        }
      }
    }
  } catch {}
}
