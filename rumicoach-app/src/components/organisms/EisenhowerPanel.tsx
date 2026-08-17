import i18n from '@/i18n'
import { Heading } from '@/components/atoms'
import { GlassPanel } from '@/components/molecules/GlassPanel'
import { EisenhowerGrid } from './EisenhowerGrid'
import type { EisenhowerMatrixData } from '@/api'

interface EisenhowerPanelProps {
  data: Record<string, unknown>
}

export function EisenhowerPanel({ data }: EisenhowerPanelProps) {
  return (
    <GlassPanel>
      <Heading color="#fff" fontSize={24} textAlign="center">{i18n.t('eisenhower_matrix') || 'Eisenhower Matrix'}</Heading>
      <EisenhowerGrid data={data as unknown as EisenhowerMatrixData} />
    </GlassPanel>
  )
}
