import { useEffect, useState } from 'react'
import {
  DndContext,
  PointerSensor,
  useSensor,
  useSensors,
  closestCenter,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  useSortable,
  arrayMove,
  rectSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Dashboard, type Report } from '@/lib/api'
import { ReportCard } from '@/components/ReportCard'

export function SortableReports({
  dashboard,
  range,
  onEditReport,
}: {
  dashboard: Dashboard
  range?: { from?: string; to?: string }
  onEditReport: (r: Report) => void
}) {
  const reports = dashboard.reports ?? []
  // Local optimistic order; synced from server when reports array changes.
  const [order, setOrder] = useState<Report[]>(reports)
  useEffect(() => {
    setOrder(reports)
  }, [reports])

  const qc = useQueryClient()
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } })
  )

  const reorder = useMutation({
    mutationFn: (ids: string[]) => api.reorderReports(dashboard.slug, ids),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['dashboard', dashboard.slug] }),
    onError: () => setOrder(reports), // revert on failure
  })

  function onDragEnd(e: DragEndEvent) {
    const { active, over } = e
    if (!over || active.id === over.id) return
    const oldIdx = order.findIndex((r) => r.id === active.id)
    const newIdx = order.findIndex((r) => r.id === over.id)
    if (oldIdx < 0 || newIdx < 0) return
    const next = arrayMove(order, oldIdx, newIdx)
    setOrder(next)
    reorder.mutate(next.map((r) => r.id))
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
      <SortableContext items={order.map((r) => r.id)} strategy={rectSortingStrategy}>
        <div className="grid grid-cols-12 gap-4">
          {order.map((r) => (
            <SortableReportCard
              key={r.id}
              report={r}
              range={range}
              onEdit={() => onEditReport(r)}
            />
          ))}
        </div>
      </SortableContext>
    </DndContext>
  )
}

function SortableReportCard({
  report,
  range,
  onEdit,
}: {
  report: Report
  range?: { from?: string; to?: string }
  onEdit: () => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
    useSortable({ id: report.id })
  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.6 : 1,
    zIndex: isDragging ? 20 : undefined,
  }
  return (
    <div ref={setNodeRef} style={style} className={widthOuterClass(report.width)}>
      <div className="relative h-full">
        <button
          {...attributes}
          {...listeners}
          className="absolute left-1 top-1 z-10 cursor-grab rounded p-1 text-muted-foreground/50 opacity-0 transition-opacity hover:bg-accent hover:text-foreground group-hover:opacity-100"
          title="Drag to reorder"
          onClick={(e) => e.preventDefault()}
        >
          <GripVertical className="h-3.5 w-3.5" />
        </button>
        <ReportCard report={report} range={range} onEdit={onEdit} />
      </div>
    </div>
  )
}

function widthOuterClass(width: number): string {
  const safe = Math.max(1, Math.min(12, width))
  if (safe <= 3) return 'col-span-12 md:col-span-3 group'
  if (safe <= 4) return 'col-span-12 md:col-span-4 group'
  if (safe <= 6) return 'col-span-12 md:col-span-6 group'
  if (safe <= 8) return 'col-span-12 md:col-span-8 group'
  return 'col-span-12 group'
}
