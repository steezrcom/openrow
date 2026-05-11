import type { LucideIcon } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { Button, Card } from '@/components/ui'

type ActionProp = {
  label: string
  onClick?: () => void
  to?: any
  params?: any
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  secondary,
}: {
  icon: LucideIcon
  title: string
  description?: string
  action?: ActionProp
  secondary?: ActionProp
}) {
  return (
    <Card className="flex flex-col items-center gap-3 p-8 text-center">
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary">
        <Icon className="h-5 w-5" />
      </div>
      <h2 className="font-medium">{title}</h2>
      {description && (
        <p className="max-w-md text-sm text-muted-foreground">{description}</p>
      )}
      {(action || secondary) && (
        <div className="mt-2 flex flex-wrap items-center justify-center gap-2">
          {action && <ActionButton action={action} variant="primary" />}
          {secondary && <ActionButton action={secondary} variant="ghost" />}
        </div>
      )}
    </Card>
  )
}

function ActionButton({
  action,
  variant,
}: {
  action: ActionProp
  variant: 'primary' | 'ghost'
}) {
  if (action.to) {
    return (
      <Link to={action.to} params={action.params}>
        <Button variant={variant} type="button">
          {action.label}
        </Button>
      </Link>
    )
  }
  return (
    <Button variant={variant} type="button" onClick={action.onClick}>
      {action.label}
    </Button>
  )
}
