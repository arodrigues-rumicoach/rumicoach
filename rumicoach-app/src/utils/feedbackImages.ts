import * as ImagePicker from 'expo-image-picker'
import { ImageManipulator, SaveFormat } from 'expo-image-manipulator'

/** Server limits, mirrored here so the user learns about a problem while
 *  attaching rather than after writing a report. Exceeding any of them rejects
 *  the whole request with 400 and loses everything they typed. */
export const MAX_IMAGES = 3
/** 5 MB *after* base64 decoding — base64 inflates by ~33%. */
export const MAX_IMAGE_BYTES = 5 * 1024 * 1024

/** Long edge to downscale to before encoding. A full-resolution screenshot is
 *  several MB on its own; this keeps one readable for triage at a fraction of
 *  the size, and of the user's data allowance. */
const MAX_EDGE = 1600
const JPEG_QUALITY = 0.7

export interface PickedImage {
  /** Bare base64, no data: prefix. The server accepts either. */
  base64: string
  /** Decoded size, i.e. what the server measures against the limit. */
  bytes: number
  /** Local uri, for the thumbnail. */
  uri: string
}

/** Bytes a base64 string decodes to, without actually decoding it. */
export function decodedBytes(base64: string): number {
  const body = base64.includes(',') ? base64.slice(base64.indexOf(',') + 1) : base64
  const padding = body.endsWith('==') ? 2 : body.endsWith('=') ? 1 : 0
  return Math.floor((body.length * 3) / 4) - padding
}

/** Downscale to MAX_EDGE on the long side and re-encode as JPEG. */
async function compress(uri: string, width: number, height: number): Promise<{ uri: string; base64: string }> {
  const longEdge = Math.max(width, height)
  const ctx = ImageManipulator.manipulate(uri)
  if (longEdge > MAX_EDGE) {
    const scale = MAX_EDGE / longEdge
    ctx.resize({ width: Math.round(width * scale), height: Math.round(height * scale) })
  }
  const rendered = await ctx.renderAsync()
  const out = await rendered.saveAsync({ compress: JPEG_QUALITY, format: SaveFormat.JPEG, base64: true })
  return { uri: out.uri, base64: out.base64 ?? '' }
}

export type PickResult =
  | { ok: true; images: PickedImage[] }
  | { ok: false; reason: 'permission' | 'too_many' | 'too_large'; index?: number }

/**
 * Open the library, compress what comes back, and enforce the count and size
 * limits locally.
 *
 * `alreadyAttached` is how many the form is holding, so the multi-select cap
 * reflects the remaining room rather than the total.
 */
export async function pickFeedbackImages(alreadyAttached: number): Promise<PickResult> {
  const remaining = MAX_IMAGES - alreadyAttached
  if (remaining <= 0) return { ok: false, reason: 'too_many' }

  const permission = await ImagePicker.requestMediaLibraryPermissionsAsync()
  if (!permission.granted) return { ok: false, reason: 'permission' }

  const result = await ImagePicker.launchImageLibraryAsync({
    mediaTypes: ['images'],
    allowsMultipleSelection: true,
    selectionLimit: remaining,
    quality: 1, // we re-encode ourselves; letting the picker degrade it first compounds the loss
  })
  if (result.canceled) return { ok: true, images: [] }

  const images: PickedImage[] = []
  for (let i = 0; i < result.assets.length; i++) {
    const asset = result.assets[i]
    const { uri, base64 } = await compress(asset.uri, asset.width, asset.height)
    const bytes = decodedBytes(base64)
    // Still too big after compression — a very large photo, or one that does not
    // compress. Name the offending one rather than failing the whole batch.
    if (bytes > MAX_IMAGE_BYTES) return { ok: false, reason: 'too_large', index: alreadyAttached + i + 1 }
    images.push({ base64, bytes, uri })
  }
  return { ok: true, images }
}
