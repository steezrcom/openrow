import { useEffect } from 'react'
import { create } from 'zustand'
import { CheckCircle2, Info, X, XCircle } from 'lucide-react'
import { cn } from '@/lib/utils'

// Small toast system. Imperative API (toast.success / .error / .info)
// so call sites stay terse; <Toaster /> renders the stack and handles
// auto-dismiss. Mounted once in __root so any route can fire toasts.

type ToastKind = 'success' | 'error' | 'info'

interface ToastItem {
  id: string
  kind: ToastKind
  text: string
  ttl: number
}

interface ToastStore {
  items: ToastItem[]
  push: (text: string, kind: ToastKind, ttl?: number) => string
  dismiss: (id: string) => void
}

const useToastStore = create<ToastStore>((set, get) => ({
  items: [],
  push: (text, kind, ttl = 4000) => {
    const id = Math.random().toString(36).slice(2)
    set((s) => ({ items: [...s.items, { id, kind, text, ttl }] }))
    if (ttl > 0) {
      setTimeout(() => get().dismiss(id), ttl)
    }
    return id
  },
  dismiss: (id) => set((s) => ({ items: s.items.filter((t) => t.id !== id) })),
}))

export const toast = {
  success: (text: string) => useToastStore.getState().push(text, 'success'),
  error: (text: string) => useToastStore.getState().push(text, 'error', 6000),
  info: (text: string) => useToastStore.getState().push(text, 'info'),
}

export function Toaster() {
  const items = useToastStore((s) => s.items)
  const dismiss = useToastStore((s) => s.dismiss)
  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2">
      {items.map((t) => (
        <Toast key={t.id} item={t} onDismiss={() => dismiss(t.id)} />
      ))}
    </div>
  )
}

function Toast({ item, onDismiss }: { item: ToastItem; onDismiss: () => void }) {
  useEffect(() => {
    // The store handles auto-dismiss; this hook is just a touchpoint
    // for the future ("keyboard escape to dismiss latest", etc.).
  }, [item.id])

  const Icon = item.kind === 'success' ? CheckCircle2 : item.kind === 'error' ? XCircle : Info
  return (
    <div
      role="status"
      className={cn(
        'pointer-events-auto flex items-start gap-3 rounded-md border bg-card p-3 text-sm shadow-lg',
        'animate-in slide-in-from-right-2 fade-in-0',
        item.kind === 'success' && 'border-primary/40',
        item.kind === 'error' && 'border-destructive/50',
        item.kind === 'info' && 'border-border',
      )}
    >
      <Icon
        className={cn(
          'mt-0.5 h-4 w-4 shrink-0',
          item.kind === 'success' && 'text-primary',
          item.kind === 'error' && 'text-destructive',
          item.kind === 'info' && 'text-muted-foreground',
        )}
      />
      <p className="flex-1 leading-snug">{item.text}</p>
      <button
        type="button"
        onClick={onDismiss}
        className="text-muted-foreground hover:text-foreground"
        aria-label="dismiss"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}
