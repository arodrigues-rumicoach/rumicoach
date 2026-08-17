import i18n from '@/i18n'
import { Heading } from '@/components/atoms'
import { GlassPanel } from '@/components/molecules/GlassPanel'
import WheelOfLifeChart from '@/components/organisms/WheelOfLifeChart'
import Reanimated, { FadeInDown } from 'react-native-reanimated'
import type { WheelCategory } from '@/api'

interface WheelPanelProps {
  data: WheelCategory[]
}

export function WheelPanel({ data }: WheelPanelProps) {
  if (data.length === 0) return null

  return (
    <Reanimated.View entering={FadeInDown.duration(500).springify().damping(20).stiffness(150)} style={{ width: '100%' }}>
      <GlassPanel margin={12} tint='default'>
        <Heading color="#fff" fontSize={24} textAlign="center">{i18n.t('wheel_of_life') || 'Wheel of Life'}</Heading>
        <WheelOfLifeChart data={data} />
      </GlassPanel>
    </Reanimated.View>
  )
}
