import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Info } from 'lucide-react'
import { cn } from '@/lib/utils'

type Side = 'top' | 'bottom'

export function Tooltip({
  content,
  children,
  className,
}: {
  content: ReactNode
  children: ReactNode
  className?: string
}) {
  const [visible, setVisible] = useState(false)
  const [side, setSide] = useState<Side>('top')
  const tipRef = useRef<HTMLSpanElement | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current)
  }, [])

  function show(delay: number) {
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      setVisible(true)
      requestAnimationFrame(() => {
        const tip = tipRef.current
        if (!tip) return
        const rect = tip.getBoundingClientRect()
        setSide(rect.top < 8 ? 'bottom' : 'top')
      })
    }, delay)
  }
  function hide() {
    if (timer.current) clearTimeout(timer.current)
    setVisible(false)
  }

  return (
    <span
      className={cn('relative inline-flex items-center', className)}
      onMouseEnter={() => show(200)}
      onMouseLeave={hide}
      onFocus={() => show(0)}
      onBlur={hide}
    >
      {children}
      {visible && (
        <span
          ref={tipRef}
          role="tooltip"
          className={cn(
            'pointer-events-none absolute left-1/2 z-50 -translate-x-1/2 whitespace-normal',
            'max-w-xs rounded-md border border-border bg-card p-2 text-xs leading-snug',
            'text-card-foreground shadow-md animate-in fade-in-0',
            side === 'top' ? 'bottom-full mb-1.5' : 'top-full mt-1.5',
          )}
        >
          {content}
        </span>
      )}
    </span>
  )
}

export function InfoBadge({
  content,
  children,
  className,
}: {
  content: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <Tooltip content={content} className={className}>
      <span className="inline-flex items-center gap-1">
        {children}
        <Info className="h-3.5 w-3.5 text-muted-foreground/70" aria-hidden />
      </span>
    </Tooltip>
  )
}
