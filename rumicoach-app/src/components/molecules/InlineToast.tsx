import Reanimated, { FadeInLeft, FadeOut } from 'react-native-reanimated'
import { Text } from 'tamagui'

interface InlineToastProps {
  message: string
  visible: boolean
}

export function InlineToast({ message, visible }: InlineToastProps) {
  if (!visible) return null

  return (
    <Reanimated.View
      entering={FadeInLeft.duration(220).springify().damping(20).stiffness(200)}
      exiting={FadeOut.duration(140)}
      style={{
        backgroundColor: 'rgba(239,68,68,0.1)',
        padding: 12,
        borderRadius: 8,
        borderWidth: 1,
        borderColor: 'rgba(239,68,68,0.3)',
      }}
    >
      <Text color="#ef4444" fontSize={13} textAlign="center">{message}</Text>
    </Reanimated.View>
  )
}
