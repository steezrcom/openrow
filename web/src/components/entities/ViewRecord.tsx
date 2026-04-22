import { Pencil } from 'lucide-react'
import type { Entity } from '@/lib/api'
import { Button } from '@/components/ui'
import { formatCell } from '@/lib/format'

export function ViewRecord({
  row,
  entity,
  refLookup,
  onEdit,
}: {
  row: Record<string, unknown>
  entity: Entity
  refLookup: (entityName: string, id: string) => string | null
  onEdit: () => void
}) {
  return (
    <div>
      <div className="mb-4 flex justify-end">
        <Button variant="ghost" onClick={onEdit}>
          <Pencil className="mr-1 h-3.5 w-3.5" /> Edit
        </Button>
      </div>
      <dl className="space-y-4">
        {entity.fields.map((f) => {
          const value = row[f.name]
          return (
            <div key={f.id} className="grid grid-cols-[140px_1fr] gap-4">
              <dt className="truncate text-xs text-muted-foreground">{f.display_name}</dt>
              <dd className="break-words text-sm">
                {value == null || value === '' ? (
                  <span className="text-muted-foreground/40">—</span>
                ) : f.data_type === 'boolean' ? (
                  value ? 'yes' : 'no'
                ) : f.data_type === 'reference' && typeof value === 'string' && f.reference_entity ? (
                  <span className="inline-flex items-center rounded-md bg-primary/10 px-2 py-0.5 text-xs text-primary">
                    {refLookup(f.reference_entity, value) ?? value}
                  </span>
                ) : f.data_type === 'jsonb' ? (
                  <pre className="whitespace-pre-wrap rounded-md bg-muted/40 p-2 font-mono text-[11px]">
                    {typeof value === 'string' ? value : JSON.stringify(value, null, 2)}
                  </pre>
                ) : (
                  formatCell(value, f, refLookup)
                )}
              </dd>
            </div>
          )
        })}
        <div className="grid grid-cols-[140px_1fr] gap-4 border-t border-border pt-4 text-xs text-muted-foreground">
          <dt>id</dt>
          <dd className="break-all font-mono">{String(row.id ?? '')}</dd>
          <dt>created</dt>
          <dd>{formatCell(row.created_at, undefined, refLookup)}</dd>
          <dt>updated</dt>
          <dd>{formatCell(row.updated_at, undefined, refLookup)}</dd>
        </div>
      </dl>
    </div>
  )
}
