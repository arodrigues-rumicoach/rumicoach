import { Text, type TextProps } from 'tamagui'

export type HeadingProps = TextProps

/**
 * Heading atom — renders text in the Manrope heading font.
 * Use this for screen titles, section headings, and any text that should
 * match the h1–h4 style. Defaults to semi-bold weight.
 */
export function Heading({ fontWeight = '600', ...props }: HeadingProps) {
  return <Text fontFamily="$heading" fontWeight={fontWeight} {...props} />
}
