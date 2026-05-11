import { cn } from '@/lib/utils'

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded-md bg-muted/30', className)} />
}

export function SkeletonRows({ count, height = 'h-9' }: { count: number; height?: string }) {
  return (
    <div className="flex flex-col gap-2">
      {Array.from({ length: count }, (_, i) => (
        <Skeleton key={i} className={height} />
      ))}
    </div>
  )
}

export function SkeletonCard({ className }: { className?: string }) {
  return <div className={cn('h-24 rounded-md bg-muted/30 animate-pulse', className)} />
}
