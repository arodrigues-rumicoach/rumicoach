import { createContext, useContext, type RefObject } from 'react'
import type { View } from 'react-native'

export const BlurTargetContext = createContext<RefObject<View | null> | undefined>(undefined)

export const useBlurTarget = () => useContext(BlurTargetContext)
