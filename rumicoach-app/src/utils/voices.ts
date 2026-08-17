export interface VoiceAsset {
  id: string
  label: string
  gender: 'male' | 'female'
}

export const VOICE_ASSETS: Record<string, VoiceAsset> = {
  enceladus: { id: 'enceladus', label: 'voice_enceladus', gender: 'male' },
  algieba: { id: 'algieba', label: 'voice_algieba', gender: 'male' },
  gacrux: { id: 'gacrux', label: 'voice_gacrux', gender: 'female' },
  charon: { id: 'charon', label: 'voice_charon', gender: 'male' },
  vindemiatrix: { id: 'vindemiatrix', label: 'voice_vindemiatrix', gender: 'female' },
  aoede: { id: 'aoede', label: 'voice_aoede', gender: 'female' },
}

const CDN_BASE_URL = process.env.EXPO_PUBLIC_CDN_URL || ''
const CDN_ROOT = CDN_BASE_URL.replace(/\/$/, '')

function capitalizeVoiceId(voiceId: string): string {
  return voiceId.charAt(0).toUpperCase() + voiceId.slice(1)
}

export function getVoiceSampleUrl(voiceId: string, locale: string): string {
  const name = capitalizeVoiceId(voiceId)
  return `${CDN_ROOT}/voices_samples/${name}_${locale}.m4a`
}

export async function getVoiceSample(voiceId: string, locale: string): Promise<string | null> {
  const asset = VOICE_ASSETS[voiceId]
  if (!asset) return null
  return getVoiceSampleUrl(voiceId, locale)
}

export function getVoicesByGender(gender: 'male' | 'female'): VoiceAsset[] {
  return Object.values(VOICE_ASSETS).filter(v => v.gender === gender)
}
