import { useContext } from 'react'
import { RevenueCatContext } from '../context/RevenueCatContext'

export function useRevenueCat() {
  const context = useContext(RevenueCatContext)
  if (!context) {
    throw new Error('useRevenueCat must be used within a RevenueCatProvider')
  }
  return context
}
