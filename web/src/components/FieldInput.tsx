import type { UseFormRegister } from 'react-hook-form'
import type { DataType, Field, RefOption } from '@/lib/api'
import { Input, Label, Textarea } from '@/components/ui'

type RegisterFn = UseFormRegister<Record<string, string>>

export function FieldInput({
  field,
  register,
  refOptions,
}: {
  field: Field
  register: RegisterFn
  refOptions: RefOption[]
}) {
  const props = register(field.name, { required: field.is_required })
  const label = (
    <Label className="text-xs">
      {field.display_name}
      {field.is_required && <span className="text-destructive"> *</span>}
    </Label>
  )
  const baseSelect =
    'flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm'

  switch (field.data_type as DataType) {
    case 'boolean':
      return (
        <label className="flex items-center gap-3 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border-input"
            {...register(field.name)}
          />
          <span>
            {field.display_name}
            {field.is_required && <span className="text-destructive"> *</span>}
          </span>
        </label>
      )
    case 'integer':
    case 'bigint':
      return (
        <div className="space-y-1.5">{label}<Input type="number" step="1" {...props} /></div>
      )
    case 'numeric':
      return (
        <div className="space-y-1.5">{label}<Input type="number" step="any" {...props} /></div>
      )
    case 'date':
      return <div className="space-y-1.5">{label}<Input type="date" {...props} /></div>
    case 'timestamptz':
      return <div className="space-y-1.5">{label}<Input type="datetime-local" {...props} /></div>
    case 'jsonb':
      return (
        <div className="space-y-1.5">
          {label}
          <Textarea placeholder='{"key":"value"}' rows={3} className="font-mono text-xs" {...props} />
        </div>
      )
    case 'reference':
      return (
        <div className="space-y-1.5">
          {label}
          <select className={baseSelect} {...props}>
            <option value="">— pick {field.reference_entity} —</option>
            {refOptions.map((o) => (
              <option key={o.ID} value={o.ID}>{o.Label}</option>
            ))}
          </select>
        </div>
      )
    default:
      return <div className="space-y-1.5">{label}<Input type="text" {...props} /></div>
  }
}
