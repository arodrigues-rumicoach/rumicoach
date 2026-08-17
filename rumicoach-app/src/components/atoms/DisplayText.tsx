import { Text, type TextProps } from 'tamagui'

export type DisplayTextProps = TextProps

/**
 * DisplayText atom — renders decorative/quote text in the Manrope font.
 * Use this for blockquotes, pull quotes, and other display typography.
 */
export function DisplayText({ fontWeight = '400', ...props }: DisplayTextProps) {
  return <Text fontFamily="$heading" fontWeight={fontWeight} {...props} />
}
