export const isMac =
  typeof navigator !== 'undefined' &&
  /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent)

export function mod(): string {
  return isMac ? '⌘' : 'Ctrl'
}
