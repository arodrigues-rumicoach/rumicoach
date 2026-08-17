import { Spinner, type SpinnerProps } from 'tamagui'
import { useSettings } from '@/hooks/useSettings'

export function ThemedSpinner({ color, ...props }: SpinnerProps) {
  const { colorScheme } = useSettings()

  return <Spinner color={color ?? colorScheme.primary} {...props} />
}
