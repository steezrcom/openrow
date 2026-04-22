import { Plus, Table2, LayoutGrid, Columns, Image as ImageIcon } from 'lucide-react'
import type { EntityView, ViewType } from '@/lib/api'
import { cn } from '@/lib/utils'

const ICONS: Record<ViewType, typeof Table2> = {
  table: Table2,
  cards: LayoutGrid,
  kanban: Columns,
  gallery: ImageIcon,
}

export function ViewTabs({
  views,
  activeId,
  onSelect,
  onNew,
}: {
  views: EntityView[]
  activeId: string
  onSelect: (id: string) => void
  onNew: () => void
}) {
  return (
    <div className="flex items-center gap-0.5 border-b border-border bg-card/30 px-6">
      <Tab
        id="table"
        active={activeId === 'table'}
        onClick={() => onSelect('table')}
        icon={<Table2 className="h-3.5 w-3.5" />}
        label="Table"
      />
      {views.map((v) => {
        const Icon = ICONS[v.view_type]
        return (
          <Tab
            key={v.id}
            id={v.id}
            active={activeId === v.id}
            onClick={() => onSelect(v.id)}
            icon={<Icon className="h-3.5 w-3.5" />}
            label={v.name}
          />
        )
      })}
      <button
        onClick={onNew}
        className="ml-1 inline-flex items-center gap-1 rounded-t-md px-3 py-2 text-xs text-muted-foreground hover:text-foreground"
        title="New view"
      >
        <Plus className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

function Tab({
  active,
  onClick,
  icon,
  label,
}: {
  id: string
  active: boolean
  onClick: () => void
  icon: React.ReactNode
  label: string
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        '-mb-px inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs transition-colors',
        active
          ? 'border-primary text-foreground'
          : 'border-transparent text-muted-foreground hover:text-foreground'
      )}
    >
      {icon}
      {label}
    </button>
  )
}
