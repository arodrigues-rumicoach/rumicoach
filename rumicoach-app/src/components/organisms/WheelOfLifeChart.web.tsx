import { WithSkiaWeb } from '@shopify/react-native-skia/lib/module/web'
import type WheelOfLifeChartImpl from './WheelOfLifeChartImpl'

export default function WheelOfLifeChart(
  props: React.ComponentProps<typeof WheelOfLifeChartImpl>,
) {
  return (
    <WithSkiaWeb
      getComponent={() => import('./WheelOfLifeChartImpl')}
      componentProps={props}
      fallback={null}
      opts={{
        locateFile: (file: string) => `/${file}`,
      }}
    />
  )
}
