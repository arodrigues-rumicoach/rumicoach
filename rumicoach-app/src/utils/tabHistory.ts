let history: string[] = ['journey']
const listeners = new Set<() => void>()

export function pushTab(tab: string) {
  const current = history[history.length - 1]
  if (current !== tab) {
    history.push(tab)
  }
  listeners.forEach(l => l())
}

export function canGoBack(): boolean {
  return history.length > 1
}

export function goBack(): string | null {
  if (history.length > 1) {
    history.pop()
    listeners.forEach(l => l())
    return history[history.length - 1]
  }
  return null
}

export function subscribeToHistory(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function getHistory(): string[] {
  return history
}