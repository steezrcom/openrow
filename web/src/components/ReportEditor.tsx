import { useEffect, useMemo, useRef } from 'react'
import {
  useForm,
  useFieldArray,
  useWatch,
  Controller,
  type Control,
  type UseFormRegister,
  type UseFormSetValue,
} from 'react-hook-form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, X } from 'lucide-react'
import {
  api,
  ApiError,
  type DataType,
  type Field,
  type QuerySpec,
  type Report,
  type ReportOptions,
  type WidgetType,
} from '@/lib/api'
import { useEntities } from '@/hooks/useEntities'
import { useEntityDetail } from '@/hooks/useEntityDetail'
import { useFieldOptions } from '@/hooks/useFieldOptions'
import { Button, Input, Label, Textarea } from '@/components/ui'
import { Drawer } from '@/components/Drawer'
import { useState } from 'react'

const WIDGET_TYPES: WidgetType[] = ['kpi', 'bar', 'line', 'area', 'pie', 'table']
const BUCKETS = ['', 'day', 'week', 'month', 'quarter', 'year'] as const
const AGG_FNS = ['count', 'sum', 'avg', 'min', 'max'] as const
const FILTER_OPS = [
  'eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'contains', 'in', 'is_null', 'is_not_null',
] as const
const NUMERIC_TYPES: DataType[] = ['integer', 'bigint', 'numeric']
const DATE_TYPES: DataType[] = ['date', 'timestamptz']

type Mode =
  | { kind: 'create'; slug: string }
  | { kind: 'edit'; report: Report }

interface FormValues {
  title: string
  subtitle: string
  width: number
  widget_type: WidgetType

  entity: string
  agg_fn: '' | typeof AGG_FNS[number]
  agg_field: string
  group_field: string
  group_bucket: typeof BUCKETS[number]
  series_field: string
  series_bucket: typeof BUCKETS[number]
  sort_field: string
  sort_dir: '' | 'asc' | 'desc'
  limit: number | string
  date_filter_field: string
  compare_period: '' | 'previous_period' | 'previous_year'

  filters: { field: string; op: typeof FILTER_OPS[number]; value: string }[]

  number_format: 'decimal' | 'integer' | 'currency' | 'percent'
  currency_code: string
  stacked: boolean
}

const SELECT_CLASS =
  'flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm'

function blankForm(): FormValues {
  return {
    title: '',
    subtitle: '',
    width: 6,
    widget_type: 'kpi',
    entity: '',
    agg_fn: 'count',
    agg_field: '',
    group_field: '',
    group_bucket: '',
    series_field: '',
    series_bucket: '',
    sort_field: '',
    sort_dir: '',
    limit: '',
    date_filter_field: '',
    compare_period: '',
    filters: [],
    number_format: 'decimal',
    currency_code: '',
    stacked: false,
  }
}

function reportToForm(r: Report): FormValues {
  const spec = r.query_spec
  const opts = r.options ?? {}
  const aggFn = spec.aggregate?.fn ?? ''
  return {
    title: r.title,
    subtitle: r.subtitle ?? '',
    width: r.width,
    widget_type: r.widget_type,
    entity: spec.entity,
    agg_fn: aggFn,
    agg_field: spec.aggregate?.field ?? '',
    group_field: spec.group_by?.field ?? '',
    group_bucket: (spec.group_by?.bucket ?? '') as FormValues['group_bucket'],
    series_field: spec.series_by?.field ?? '',
    series_bucket: (spec.series_by?.bucket ?? '') as FormValues['series_bucket'],
    sort_field: spec.sort?.field ?? '',
    sort_dir: spec.sort?.dir ?? '',
    limit: spec.limit ?? '',
    date_filter_field: spec.date_filter_field ?? '',
    compare_period: (spec.compare_period ?? '') as FormValues['compare_period'],
    filters: (spec.filters ?? []).map((f) => ({
      field: f.field,
      op: f.op as typeof FILTER_OPS[number],
      value: serializeFilterValue(f.value),
    })),
    number_format: (opts.number_format as FormValues['number_format']) ?? 'decimal',
    currency_code: opts.currency_code ?? '',
    stacked: Boolean(opts.stacked),
  }
}

function serializeFilterValue(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (Array.isArray(v)) return v.join(', ')
  if (typeof v === 'boolean' || typeof v === 'number') return String(v)
  return String(v)
}

function parseFilterValue(op: string, raw: string): unknown {
  if (op === 'is_null' || op === 'is_not_null') return undefined
  if (op === 'in') {
    return raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
      .map((s) => (isFinite(Number(s)) && s !== '' ? Number(s) : s))
  }
  if (raw === 'true') return true
  if (raw === 'false') return false
  if (raw !== '' && isFinite(Number(raw))) return Number(raw)
  return raw
}

function validateBeforeSubmit(v: FormValues): string | null {
  if (!v.title.trim()) return 'Title is required.'
  if (!v.entity) return 'Pick an entity.'
  if (v.widget_type !== 'table' && !v.agg_fn) {
    return `${v.widget_type} needs an aggregate.`
  }
  if ((v.agg_fn === 'sum' || v.agg_fn === 'avg') && !v.agg_field) {
    return `${v.agg_fn} needs a numeric field.`
  }
  if (
    (v.widget_type === 'bar' ||
      v.widget_type === 'line' ||
      v.widget_type === 'area' ||
      v.widget_type === 'pie') &&
    !v.group_field
  ) {
    return `${v.widget_type} needs a group_by field.`
  }
  if (v.widget_type === 'kpi' && v.compare_period && !v.date_filter_field) {
    return 'compare_period requires a date filter field.'
  }
  return null
}

function formToSpec(v: FormValues): { spec: QuerySpec; options: ReportOptions } {
  const spec: QuerySpec = { entity: v.entity.trim() }

  const widgetNeedsAggregate = v.widget_type !== 'table'
  if (widgetNeedsAggregate && v.agg_fn) {
    spec.aggregate = {
      fn: v.agg_fn as typeof AGG_FNS[number],
      field: v.agg_field || undefined,
    }
  }

  const widgetNeedsGroup =
    v.widget_type === 'bar' || v.widget_type === 'line' ||
    v.widget_type === 'area' || v.widget_type === 'pie'
  if (widgetNeedsGroup && v.group_field) {
    spec.group_by = {
      field: v.group_field,
      bucket: v.group_bucket || undefined,
    }
  }

  const widgetAllowsSeries =
    v.widget_type === 'bar' || v.widget_type === 'line' || v.widget_type === 'area'
  if (widgetAllowsSeries && v.series_field) {
    spec.series_by = {
      field: v.series_field,
      bucket: v.series_bucket || undefined,
    }
  }

  if (v.sort_field && v.sort_dir) {
    spec.sort = { field: v.sort_field, dir: v.sort_dir as 'asc' | 'desc' }
  }

  const limitNum = typeof v.limit === 'number' ? v.limit : Number(v.limit)
  if (isFinite(limitNum) && limitNum > 0) spec.limit = limitNum

  if (v.date_filter_field) spec.date_filter_field = v.date_filter_field
  if (v.widget_type === 'kpi' && v.compare_period) spec.compare_period = v.compare_period

  const validFilters = v.filters.filter((f) => f.field && f.op)
  if (validFilters.length) {
    spec.filters = validFilters.map((f) => ({
      field: f.field,
      op: f.op,
      value: parseFilterValue(f.op, f.value),
    }))
  }

  const options: ReportOptions = {
    number_format: v.number_format,
  }
  if (v.number_format === 'currency') {
    options.currency_code = v.currency_code || 'USD'
  }
  if ((v.widget_type === 'bar' || v.widget_type === 'area') && v.stacked) {
    options.stacked = true
  }

  return { spec, options }
}

export function ReportEditor({
  mode,
  onClose,
}: {
  mode: Mode | null
  onClose: () => void
}) {
  const qc = useQueryClient()
  const entities = useEntities()
  const { register, handleSubmit, reset, watch, setValue, control, formState: { isSubmitting } } =
    useForm<FormValues>({ defaultValues: blankForm() })
  const [error, setError] = useState<string | null>(null)

  const filtersArray = useFieldArray({ control, name: 'filters' })
  const entity = watch('entity')
  const widgetType = watch('widget_type')
  const aggFn = watch('agg_fn')
  const groupField = watch('group_field')
  const numberFormat = watch('number_format')

  // Auto-clean form fields that don't apply to the current widget type so
  // switching kpi→bar (or vice-versa) doesn't leave stale state that would
  // either fail validation or render oddly.
  const prevWidgetRef = useRef<WidgetType | null>(null)
  useEffect(() => {
    if (prevWidgetRef.current === null) {
      prevWidgetRef.current = widgetType
      return
    }
    if (prevWidgetRef.current === widgetType) return
    if (widgetType === 'kpi') {
      setValue('group_field', '')
      setValue('group_bucket', '')
      setValue('series_field', '')
      setValue('series_bucket', '')
      if (!watch('agg_fn')) setValue('agg_fn', 'count')
    }
    if (widgetType === 'pie') {
      setValue('series_field', '')
      setValue('series_bucket', '')
      setValue('compare_period', '')
    }
    if (widgetType === 'bar' || widgetType === 'line' || widgetType === 'area') {
      setValue('compare_period', '')
    }
    if (widgetType === 'table') {
      setValue('agg_fn', '')
      setValue('agg_field', '')
      setValue('group_field', '')
      setValue('group_bucket', '')
      setValue('series_field', '')
      setValue('series_bucket', '')
      setValue('compare_period', '')
    }
    prevWidgetRef.current = widgetType
  }, [widgetType, setValue, watch])

  const entityDetail = useEntityDetail(entity)
  const entityFields = entityDetail.data?.fields ?? []
  const numericFields = entityFields.filter((f) => NUMERIC_TYPES.includes(f.data_type))
  const dateFields = entityFields.filter((f) => DATE_TYPES.includes(f.data_type))
  const allFieldNames = useMemo(
    () => ['id', 'created_at', 'updated_at', ...entityFields.map((f) => f.name)],
    [entityFields]
  )
  const groupFieldInfo = entityFields.find((f) => f.name === groupField)
  const isDateGroup = groupFieldInfo && DATE_TYPES.includes(groupFieldInfo.data_type)

  useEffect(() => {
    if (!mode) return
    if (mode.kind === 'edit') {
      reset(reportToForm(mode.report))
    } else {
      reset(blankForm())
    }
    setError(null)
  }, [mode, reset])

  const mut = useMutation({
    mutationFn: async (v: FormValues) => {
      const vErr = validateBeforeSubmit(v)
      if (vErr) throw new Error(vErr)
      const { spec, options } = formToSpec(v)
      if (!mode) return
      if (mode.kind === 'edit') {
        await api.updateReport(mode.report.id, {
          title: v.title,
          subtitle: v.subtitle,
          width: Number(v.width),
          widget_type: v.widget_type,
          query_spec: spec,
          options,
        })
      } else {
        await api.addReport(mode.slug, {
          title: v.title,
          subtitle: v.subtitle || undefined,
          widget_type: v.widget_type,
          query_spec: spec,
          options,
          width: Number(v.width),
        })
      }
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['dashboard'] })
      await qc.invalidateQueries({ queryKey: ['report-exec'] })
      onClose()
    },
    onError: (err) =>
      setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : 'failed'),
  })

  if (!mode) return null

  const needsAggregate = widgetType !== 'table'
  const needsGroup = widgetType === 'bar' || widgetType === 'line' ||
    widgetType === 'area' || widgetType === 'pie'
  const allowsSeries = widgetType === 'bar' || widgetType === 'line' || widgetType === 'area'
  const allowsStack = widgetType === 'bar' || widgetType === 'area'
  const isKPI = widgetType === 'kpi'

  return (
    <Drawer
      open={true}
      onClose={onClose}
      title={mode.kind === 'edit' ? 'Edit report' : 'New report'}
      subtitle={mode.kind === 'edit' ? mode.report.title : undefined}
      widthClass="w-[560px]"
    >
      <form className="space-y-6" onSubmit={handleSubmit((v) => { setError(null); mut.mutate(v) })}>
        <Section title="Basics">
          <Field label="Title">
            <Input autoFocus {...register('title', { required: true })} />
          </Field>
          <Field label="Subtitle">
            <Input {...register('subtitle')} />
          </Field>
          <Two>
            <Field label="Widget type">
              <select className={SELECT_CLASS} {...register('widget_type')}>
                {WIDGET_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
              </select>
            </Field>
            <Field label="Width (1–12)">
              <Input type="number" min={1} max={12} step={1} {...register('width', { required: true })} />
            </Field>
          </Two>
        </Section>

        <Section title="Data">
          <Field label="Entity">
            <select className={SELECT_CLASS} {...register('entity', { required: true })}>
              <option value="">— pick an entity —</option>
              {(entities.data ?? []).map((e) => (
                <option key={e.id} value={e.name}>{e.display_name} ({e.name})</option>
              ))}
            </select>
          </Field>

          {needsAggregate && (
            <Two>
              <Field label="Aggregate">
                <select className={SELECT_CLASS} {...register('agg_fn')}>
                  {AGG_FNS.map((fn) => <option key={fn} value={fn}>{fn}</option>)}
                </select>
              </Field>
              <Field label={aggFn === 'count' ? 'Field (optional)' : 'Field'}>
                <select className={SELECT_CLASS} {...register('agg_field')}>
                  <option value="">— none (count *) —</option>
                  {(aggFn === 'count' ? entityFields : aggFn === 'min' || aggFn === 'max' ? entityFields : numericFields)
                    .map((f) => <option key={f.id} value={f.name}>{f.display_name}</option>)}
                </select>
              </Field>
            </Two>
          )}

          {needsGroup && (
            <Two>
              <Field label="Group by">
                <select className={SELECT_CLASS} {...register('group_field')}>
                  <option value="">— pick a field —</option>
                  {allFieldNames.map((n) => <option key={n} value={n}>{n}</option>)}
                </select>
              </Field>
              <Field label="Bucket (dates only)">
                <select
                  className={SELECT_CLASS}
                  disabled={!isDateGroup && groupField !== 'created_at' && groupField !== 'updated_at'}
                  {...register('group_bucket')}
                >
                  {BUCKETS.map((b) => <option key={b} value={b}>{b || '— none —'}</option>)}
                </select>
              </Field>
            </Two>
          )}

          {allowsSeries && (
            <Two>
              <Field label="Series by (optional)">
                <select className={SELECT_CLASS} {...register('series_field')}>
                  <option value="">— none —</option>
                  {allFieldNames.map((n) => <option key={n} value={n}>{n}</option>)}
                </select>
              </Field>
              <Field label="Series bucket">
                <select className={SELECT_CLASS} {...register('series_bucket')}>
                  {BUCKETS.map((b) => <option key={b} value={b}>{b || '— none —'}</option>)}
                </select>
              </Field>
            </Two>
          )}

          {(needsGroup || widgetType === 'table') && (
            <Two>
              <Field label="Sort by">
                <select className={SELECT_CLASS} {...register('sort_field')}>
                  <option value="">— default —</option>
                  {needsGroup && <option value="label">label</option>}
                  {needsGroup && <option value="value">value</option>}
                  {allFieldNames.map((n) => <option key={n} value={n}>{n}</option>)}
                </select>
              </Field>
              <Field label="Direction">
                <select className={SELECT_CLASS} {...register('sort_dir')}>
                  <option value="">—</option>
                  <option value="asc">asc</option>
                  <option value="desc">desc</option>
                </select>
              </Field>
            </Two>
          )}

          <Field label="Limit (optional)">
            <Input type="number" min={1} placeholder={widgetType === 'table' ? '100' : '500'} {...register('limit')} />
          </Field>

          <Field label="Date filter field">
            <select className={SELECT_CLASS} {...register('date_filter_field')}>
              <option value="">— ignore dashboard date range —</option>
              <option value="created_at">created_at</option>
              <option value="updated_at">updated_at</option>
              {dateFields.map((f) => <option key={f.id} value={f.name}>{f.name}</option>)}
            </select>
            <Hint>When set, the dashboard's date range picker filters this report on this field.</Hint>
          </Field>

          {isKPI && (
            <Field label="Compare to">
              <select className={SELECT_CLASS} {...register('compare_period')}>
                <option value="">— no comparison —</option>
                <option value="previous_period">Previous period (same length)</option>
                <option value="previous_year">Previous year</option>
              </select>
              <Hint>Requires a date filter field and an actual range selected on the dashboard.</Hint>
            </Field>
          )}
        </Section>

        <Section title="Filters">
          <div className="space-y-2">
            {filtersArray.fields.map((field, i) => (
              <FilterRow
                key={field.id}
                index={i}
                register={register}
                setValue={setValue}
                control={control}
                entityName={entity}
                fields={entityFields}
                onRemove={() => filtersArray.remove(i)}
              />
            ))}
            <button
              type="button"
              onClick={() => filtersArray.append({ field: '', op: 'eq', value: '' })}
              className="inline-flex items-center gap-1.5 rounded-md border border-dashed border-border px-3 py-1.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              <Plus className="h-3 w-3" /> Add filter
            </button>
          </div>
        </Section>

        <Section title="Display">
          <Two>
            <Field label="Number format">
              <select className={SELECT_CLASS} {...register('number_format')}>
                <option value="decimal">Decimal</option>
                <option value="integer">Integer</option>
                <option value="currency">Currency</option>
                <option value="percent">Percent</option>
              </select>
            </Field>
            <Field label="Currency code">
              <Input
                disabled={numberFormat !== 'currency'}
                placeholder="CZK / USD / EUR"
                {...register('currency_code')}
              />
            </Field>
          </Two>
          {allowsStack && (
            <label className="flex items-center gap-2 text-sm">
              <Controller
                control={control}
                name="stacked"
                render={({ field }) => (
                  <input
                    type="checkbox"
                    className="h-4 w-4 rounded border-input"
                    checked={Boolean(field.value)}
                    onChange={(e) => field.onChange(e.target.checked)}
                  />
                )}
              />
              <span>Stacked (series stack on top of each other)</span>
            </label>
          )}
        </Section>

        {error && (
          <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
            {error}
          </div>
        )}

        <div className="sticky bottom-0 -mx-6 flex items-center gap-2 border-t border-border bg-card/95 px-6 py-3 backdrop-blur">
          <Button type="submit" disabled={isSubmitting || mut.isPending}>
            {mut.isPending
              ? 'Saving…'
              : mode.kind === 'edit'
              ? 'Save changes'
              : 'Create report'}
          </Button>
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </form>
    </Drawer>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <p className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
        {title}
      </p>
      <div className="space-y-3">{children}</div>
    </section>
  )
}

function Two({ children }: { children: React.ReactNode }) {
  return <div className="grid grid-cols-2 gap-3">{children}</div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {children}
    </div>
  )
}

function Hint({ children }: { children: React.ReactNode }) {
  return <p className="text-[11px] text-muted-foreground">{children}</p>
}

function FilterRow({
  index,
  register,
  setValue,
  control,
  entityName,
  fields,
  onRemove,
}: {
  index: number
  register: UseFormRegister<FormValues>
  setValue: UseFormSetValue<FormValues>
  control: Control<FormValues>
  entityName: string
  fields: Field[]
  onRemove: () => void
}) {
  const current = useWatch({ control, name: `filters.${index}` })
  const field = fields.find((f) => f.name === current?.field)
  const op = current?.op ?? 'eq'
  const isNullOp = op === 'is_null' || op === 'is_not_null'
  const isInOp = op === 'in'
  const dataType: DataType | undefined = field?.data_type

  // When the user changes the filter field, clear the value so a stale entry
  // from a different type doesn't linger.
  const prevFieldRef = useRef<string | undefined>(current?.field)
  useEffect(() => {
    if (prevFieldRef.current !== current?.field) {
      setValue(`filters.${index}.value` as const, '')
      prevFieldRef.current = current?.field
    }
  }, [current?.field, index, setValue])

  return (
    <div className="grid grid-cols-[1fr_auto_1fr_auto] items-start gap-2">
      <select
        className={SELECT_CLASS}
        {...register(`filters.${index}.field` as const)}
      >
        <option value="">— field —</option>
        {fields.map((f) => (
          <option key={f.id} value={f.name}>{f.name}</option>
        ))}
      </select>
      <select
        className={SELECT_CLASS + ' w-32'}
        {...register(`filters.${index}.op` as const)}
      >
        {FILTER_OPS.map((op) => <option key={op} value={op}>{op}</option>)}
      </select>
      <FilterValueInput
        index={index}
        op={op}
        isNullOp={isNullOp}
        isInOp={isInOp}
        dataType={dataType}
        entityName={entityName}
        fieldName={current?.field}
        referenceEntity={field?.reference_entity}
        register={register}
        control={control}
      />
      <button
        type="button"
        onClick={onRemove}
        className="rounded-md p-2 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
        title="Remove filter"
      >
        <Trash2 className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

function FilterValueInput({
  index,
  op,
  isNullOp,
  isInOp,
  dataType,
  entityName,
  fieldName,
  referenceEntity,
  register,
  control,
}: {
  index: number
  op: string
  isNullOp: boolean
  isInOp: boolean
  dataType: DataType | undefined
  entityName: string | undefined
  fieldName: string | undefined
  referenceEntity: string | undefined
  register: UseFormRegister<FormValues>
  control: Control<FormValues>
}) {
  const name = `filters.${index}.value` as const
  const binding = register(name)

  if (isNullOp) {
    return <Input disabled placeholder="—" />
  }

  if (isInOp) {
    if (dataType === 'reference' && entityName && fieldName && referenceEntity) {
      return (
        <Controller
          control={control}
          name={name}
          render={({ field }) => (
            <RefMultiSelect
              entityName={entityName}
              fieldName={fieldName}
              value={String(field.value ?? '')}
              onChange={field.onChange}
            />
          )}
        />
      )
    }
    return (
      <Controller
        control={control}
        name={name}
        render={({ field }) => (
          <TagsInput
            value={String(field.value ?? '')}
            onChange={field.onChange}
            placeholder="add value + Enter"
          />
        )}
      />
    )
  }

  if (dataType === 'reference' && entityName && fieldName && referenceEntity) {
    return <RefValueSelect entityName={entityName} fieldName={fieldName} bindingName={name} register={register} />
  }
  if (dataType === 'boolean') {
    return (
      <select className={SELECT_CLASS} {...binding}>
        <option value="">— pick —</option>
        <option value="true">true</option>
        <option value="false">false</option>
      </select>
    )
  }
  if (dataType === 'date') {
    return <Input type="date" {...binding} />
  }
  if (dataType === 'timestamptz') {
    return <Input type="datetime-local" {...binding} />
  }
  if (dataType === 'integer' || dataType === 'bigint') {
    return <Input type="number" step={1} {...binding} />
  }
  if (dataType === 'numeric') {
    return <Input type="number" step="any" {...binding} />
  }
  if (dataType === 'jsonb') {
    return <Textarea rows={2} placeholder='{"key":"value"}' className="font-mono text-xs" {...binding} />
  }
  return <Input placeholder={op === 'contains' ? 'substring' : 'value'} {...binding} />
}

function RefValueSelect({
  entityName,
  fieldName,
  bindingName,
  register,
}: {
  entityName: string
  fieldName: string
  bindingName: `filters.${number}.value`
  register: UseFormRegister<FormValues>
}) {
  const { data, isLoading } = useFieldOptions(entityName, fieldName)
  return (
    <select className={SELECT_CLASS} {...register(bindingName)} disabled={isLoading}>
      <option value="">{isLoading ? 'loading…' : '— pick —'}</option>
      {(data ?? []).map((o) => (
        <option key={o.ID} value={o.ID}>{o.Label}</option>
      ))}
    </select>
  )
}

function splitCSV(v: string): string[] {
  return v
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

function joinCSV(items: string[]): string {
  return items.join(', ')
}

function RefMultiSelect({
  entityName,
  fieldName,
  value,
  onChange,
}: {
  entityName: string
  fieldName: string
  value: string
  onChange: (next: string) => void
}) {
  const { data, isLoading } = useFieldOptions(entityName, fieldName)
  const options = data ?? []
  const selectedIds = splitCSV(value)
  const selected = selectedIds.map(
    (id) => options.find((o) => o.ID === id) ?? { ID: id, Label: id.slice(0, 8) + '…' }
  )

  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    if (open) document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [open])

  const q = search.trim().toLowerCase()
  const filtered = options.filter(
    (o) => !selectedIds.includes(o.ID) && (!q || o.Label.toLowerCase().includes(q))
  )

  function add(id: string) {
    if (selectedIds.includes(id)) return
    onChange(joinCSV([...selectedIds, id]))
    setSearch('')
  }
  function remove(id: string) {
    onChange(joinCSV(selectedIds.filter((x) => x !== id)))
  }

  return (
    <div ref={ref} className="relative">
      <div
        className="flex min-h-[40px] flex-wrap items-center gap-1 rounded-md border border-input bg-background p-1.5 text-sm focus-within:ring-2 focus-within:ring-ring"
        onClick={() => setOpen(true)}
      >
        {selected.map((o) => (
          <span
            key={o.ID}
            className="inline-flex items-center gap-1 rounded-md bg-primary/10 px-2 py-0.5 text-xs text-primary"
          >
            {o.Label}
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                remove(o.ID)
              }}
              className="-mr-1 rounded p-0.5 hover:bg-primary/20"
              aria-label={`Remove ${o.Label}`}
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
        <input
          className="flex-1 min-w-[80px] bg-transparent outline-none"
          placeholder={
            isLoading ? 'loading…' : selected.length ? '' : 'type to filter…'
          }
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setOpen(true)
          }}
          onFocus={() => setOpen(true)}
          onKeyDown={(e) => {
            if (e.key === 'Backspace' && search === '' && selectedIds.length) {
              remove(selectedIds[selectedIds.length - 1])
            }
            if (e.key === 'Enter') {
              e.preventDefault()
              if (filtered.length) add(filtered[0].ID)
            }
            if (e.key === 'Escape') setOpen(false)
          }}
        />
      </div>
      {open && (filtered.length > 0 || isLoading) && (
        <div className="absolute left-0 right-0 z-30 mt-1 max-h-52 overflow-auto rounded-md border border-border bg-card shadow-lg">
          {isLoading && (
            <div className="px-3 py-2 text-xs text-muted-foreground">loading…</div>
          )}
          {filtered.map((o) => (
            <button
              key={o.ID}
              type="button"
              onClick={() => add(o.ID)}
              className="block w-full px-3 py-1.5 text-left text-sm hover:bg-accent"
            >
              {o.Label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function TagsInput({
  value,
  onChange,
  placeholder,
}: {
  value: string
  onChange: (next: string) => void
  placeholder?: string
}) {
  const tags = splitCSV(value)
  const [input, setInput] = useState('')

  function addTag(raw: string) {
    const clean = raw.trim()
    if (!clean || tags.includes(clean)) {
      setInput('')
      return
    }
    onChange(joinCSV([...tags, clean]))
    setInput('')
  }
  function removeTag(t: string) {
    onChange(joinCSV(tags.filter((x) => x !== t)))
  }

  return (
    <div className="flex min-h-[40px] flex-wrap items-center gap-1 rounded-md border border-input bg-background p-1.5 text-sm focus-within:ring-2 focus-within:ring-ring">
      {tags.map((t) => (
        <span
          key={t}
          className="inline-flex items-center gap-1 rounded-md bg-muted/60 px-2 py-0.5 text-xs"
        >
          {t}
          <button
            type="button"
            onClick={() => removeTag(t)}
            className="-mr-1 rounded p-0.5 hover:bg-accent"
            aria-label={`Remove ${t}`}
          >
            <X className="h-3 w-3" />
          </button>
        </span>
      ))}
      <input
        className="flex-1 min-w-[80px] bg-transparent outline-none"
        placeholder={placeholder}
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ',') {
            e.preventDefault()
            addTag(input)
          }
          if (e.key === 'Backspace' && input === '' && tags.length) {
            removeTag(tags[tags.length - 1])
          }
        }}
        onBlur={() => {
          if (input.trim()) addTag(input)
        }}
      />
    </div>
  )
}
