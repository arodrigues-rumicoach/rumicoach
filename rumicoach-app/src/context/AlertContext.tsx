import { createContext, useCallback, useState, useMemo, type ReactNode } from 'react'
import { ConfirmModal } from '@/components/organisms/ConfirmModal'

export interface ConfirmOptions {
  title: string
  message?: string
  confirmLabel: string
  destructive?: boolean
  onConfirm: () => void | Promise<void>
  type?: 'error' | 'info' | 'superError'
}

interface AlertContextValue {
  showConfirm: (options: ConfirmOptions) => void
}

export const AlertContext = createContext<AlertContextValue | undefined>(undefined)

export function AlertProvider({ children }: { children: ReactNode }) {
  const [visible, setVisible] = useState(false)
  const [options, setOptions] = useState<ConfirmOptions | null>(null)

  const showConfirm = useCallback((opts: ConfirmOptions) => {
    setOptions(opts)
    setVisible(true)
  }, [])

  const handleConfirm = useCallback(async () => {
    if (options?.onConfirm) {
      await options.onConfirm()
    }
    setVisible(false)
    setOptions(null)
  }, [options])

  const handleCancel = useCallback(() => {
    setVisible(false)
    setOptions(null)
  }, [])

  const value = useMemo(() => ({
    showConfirm,
  }), [showConfirm])

  return (
    <AlertContext.Provider value={value}>
      {children}
      {options && (
        <ConfirmModal
          visible={visible}
          title={options.title}
          message={options.message}
          confirmLabel={options.confirmLabel}
          destructive={options.destructive}
          type={options.type || 'error'}
          onConfirm={handleConfirm}
          onCancel={handleCancel}
        />
      )}
    </AlertContext.Provider>
  )
}
