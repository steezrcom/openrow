import { forwardRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type Ref, type TextareaHTMLAttributes } from 'react'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils'

export const Button = forwardRef<
  HTMLButtonElement,
  ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'ghost' | 'danger' }
>(({ className, variant = 'primary', ...props }, ref) => {
  const base =
    'inline-flex items-center justify-center rounded-md px-4 py-2 text-sm font-medium ' +
    'disabled:opacity-50 disabled:pointer-events-none transition-colors focus-visible:outline-none ' +
    'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background'
  const variants = {
    primary: 'bg-primary text-primary-foreground hover:bg-primary/90',
    ghost: 'bg-transparent text-foreground hover:bg-accent',
    danger: 'bg-destructive text-destructive-foreground hover:bg-destructive/90',
  }
  return <button ref={ref} className={cn(base, variants[variant], className)} {...props} />
})
Button.displayName = 'Button'

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        'flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm',
        'placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2',
        'focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
        className
      )}
      {...props}
    />
  )
)
Input.displayName = 'Input'

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(
  ({ className, ...props }, ref) => (
    <textarea
      ref={ref}
      className={cn(
        'flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm',
        'placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2',
        'focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
        className
      )}
      {...props}
    />
  )
)
Textarea.displayName = 'Textarea'

export function Label({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return <label className={cn('text-sm font-medium text-muted-foreground', className)} {...props} />
}

export function Card({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn('rounded-lg border border-border bg-card text-card-foreground', className)}
      {...props}
    />
  )
}

export function Pill({ className, ...props }: React.HTMLAttributes<HTMLSpanElement>) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full border border-border bg-muted/50',
        'px-2 py-0.5 font-mono text-[11px] text-muted-foreground',
        className
      )}
      {...props}
    />
  )
}

export function ErrorAlert({ className, children, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  if (!children) return null
  return (
    <div
      role="alert"
      className={cn(
        'rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive',
        className,
      )}
      {...props}
    >
      {children}
    </div>
  )
}

// FormActions is a sticky drawer/modal footer that hosts the submit +
// cancel buttons. Pass className overrides when the parent has its own
// horizontal padding you need to bleed through (see ReportEditor).
export function FormActions({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'sticky bottom-0 flex items-center gap-2 border-t border-border bg-card/95 py-3 backdrop-blur',
        className,
      )}
      {...props}
    />
  )
}

export type Chip = {
  id: string
  label: string
  onRemove: () => void
  tone?: 'primary' | 'muted'
}

// ChipRow renders a bordered row of removable chips followed by children
// (typically an <input> and, optionally, a sibling dropdown). The parent
// owns the input state + keyboard handling — this primitive only
// standardises the shell and chip markup shared by tag / multi-select
// widgets.
export function ChipRow({
  chips,
  children,
  onClick,
  containerRef,
}: {
  chips: Chip[]
  children?: ReactNode
  onClick?: () => void
  containerRef?: Ref<HTMLDivElement>
}) {
  return (
    <div
      ref={containerRef}
      onClick={onClick}
      className="flex min-h-[40px] flex-wrap items-center gap-1 rounded-md border border-input bg-background p-1.5 text-sm focus-within:ring-2 focus-within:ring-ring"
    >
      {chips.map((c) => {
        const primary = c.tone === 'primary'
        return (
          <span
            key={c.id}
            className={cn(
              'inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs',
              primary ? 'bg-primary/10 text-primary' : 'bg-muted/60',
            )}
          >
            {c.label}
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                c.onRemove()
              }}
              className={cn('-mr-1 rounded p-0.5', primary ? 'hover:bg-primary/20' : 'hover:bg-accent')}
              aria-label={`Remove ${c.label}`}
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        )
      })}
      {children}
    </div>
  )
}
