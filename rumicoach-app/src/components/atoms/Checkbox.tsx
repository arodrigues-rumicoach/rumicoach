import { useSettings } from '@/hooks/useSettings'
import { type ReactNode } from 'react'
import { Checkbox as TamaguiCheckbox, type CheckboxProps } from 'tamagui'
import { Check } from 'lucide-react-native'

interface CheckboxProps2 extends Omit<CheckboxProps, 'checked' | 'onCheckedChange' | 'size'> {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  children?: ReactNode
}

export function Checkbox({
  checked,
  onCheckedChange,
  children,
  ...props
}: CheckboxProps2) {
  const { colorScheme } = useSettings()
  return (
    <TamaguiCheckbox
      checked={checked}
      onCheckedChange={onCheckedChange}
      size="$3"
      width={24}
      height={24}
      borderRadius={8}
      // The box sits on the light glass card, which can be almost white over a
      // bright video frame — it needs an opaque fill and a solid border of its
      // own to stay visible. Every scheme's primary is a dark saturated tone,
      // so it carries the border unchecked and the fill checked.
      backgroundColor={checked ? colorScheme.primary : 'rgba(255,255,255,0.9)'}
      borderWidth={2}
      borderColor={colorScheme.primary}
      pressStyle={{
        backgroundColor: checked ? colorScheme.tertiary : '#fff',
        borderColor: colorScheme.tertiary,
      }}
      cursor='pointer'
      {...props}
    >
      <TamaguiCheckbox.Indicator justifyContent="center" alignItems="center">
        {checked && <Check size={16} strokeWidth={3} color="#fff" />}
      </TamaguiCheckbox.Indicator>
    </TamaguiCheckbox>
  )
}
