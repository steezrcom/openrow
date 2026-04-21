import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pause, Play, X } from 'lucide-react'
import { api, type RefOption } from '@/lib/api'
import { useFieldOptions } from '@/hooks/useFieldOptions'
import { useMe } from '@/hooks/useMe'
import { useTimer, formatElapsed } from '@/lib/timer'
import { Input } from '@/components/ui'
import { cn } from '@/lib/utils'
import { useT } from '@/lib/i18n'

// TimerWidget lives in the app header. Shows either a "Start" button (idle)
// or a pulsing elapsed-time display (running). Clicking either toggles a
// popover with the project/task/description fields.
export function TimerWidget() {
  // Check whether the time_entries entity exists in this workspace. If the
  // agency template hasn't been installed yet, we hide the widget.
  const entities = useQuery({ queryKey: ['entities'], queryFn: api.listEntities })
  const hasTimeEntries = Boolean(entities.data?.some((e) => e.name === 'time_entries'))
  const hasProjects = Boolean(entities.data?.some((e) => e.name === 'projects'))

  if (!hasTimeEntries || !hasProjects) return null
  return <TimerWidgetInner />
}

function TimerWidgetInner() {
  const me = useMe()
  const timer = useTimer()
  const t = useT()
  const running = timer.running
  const [open, setOpen] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!running) return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [running])

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    if (open) document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [open])

  const elapsed = running && timer.startedAt ? formatElapsed(timer.startedAt, now) : null

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'inline-flex items-center gap-2 rounded-md border border-border px-3 py-1.5 text-sm',
          running
            ? 'bg-primary/10 text-primary hover:bg-primary/15'
            : 'bg-background text-muted-foreground hover:bg-accent hover:text-foreground'
        )}
        title={running ? t('timer.running') : t('timer.startTimer')}
      >
        {running ? (
          <>
            <span className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-60" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-primary" />
            </span>
            <span className="font-mono text-xs">{elapsed}</span>
            <span className="hidden max-w-[120px] truncate sm:inline text-xs text-muted-foreground">
              {timer.projectLabel}
            </span>
          </>
        ) : (
          <>
            <Play className="h-3.5 w-3.5" />
            <span className="text-xs">{t('timer.start')}</span>
          </>
        )}
      </button>
      {open && (
        <div className="absolute bottom-full left-0 z-30 mb-1 w-[340px] rounded-md border border-border bg-card shadow-lg">
          {running ? (
            <RunningPanel elapsed={elapsed ?? ''} onClose={() => setOpen(false)} />
          ) : (
            <StartPanel
              person={me.data?.user.name ?? ''}
              onClose={() => setOpen(false)}
            />
          )}
        </div>
      )}
    </div>
  )
}

function RunningPanel({ elapsed, onClose }: { elapsed: string; onClose: () => void }) {
  const qc = useQueryClient()
  const timer = useTimer()
  const [error, setError] = useState<string | null>(null)

  const stopAndSave = useMutation({
    mutationFn: async () => {
      const result = timer.stop()
      if (!result) return
      const date = new Date(result.startedAt)
      const yyyy = date.getFullYear()
      const mm = String(date.getMonth() + 1).padStart(2, '0')
      const dd = String(date.getDate()).padStart(2, '0')
      const values: Record<string, string> = {
        project: result.projectId,
        person: result.person,
        date: `${yyyy}-${mm}-${dd}`,
        hours: String(result.hours),
        description: result.description,
        billable: result.billable ? 'true' : 'false',
      }
      if (result.taskId) values.task = result.taskId
      await api.createRow('time_entries', values)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rows', 'time_entries'] })
      qc.invalidateQueries({ queryKey: ['time-entries'] })
      qc.invalidateQueries({ queryKey: ['report-exec'] })
      onClose()
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'failed'),
  })

  return (
    <div className="p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div className="font-mono text-lg tabular-nums">{elapsed}</div>
        <button
          onClick={() => {
            if (confirm('Discard this running timer?')) {
              useTimer.getState().cancel()
              onClose()
            }
          }}
          className="rounded p-1 text-muted-foreground hover:bg-accent"
          title="Discard"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
      <p className="text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{timer.projectLabel}</span>
        {timer.taskLabel ? <span> · {timer.taskLabel}</span> : null}
      </p>
      <Input
        value={timer.description}
        onChange={(e) => useTimer.getState().updateDescription(e.target.value)}
        placeholder="What are you working on?"
      />
      <label className="flex items-center gap-2 text-xs text-muted-foreground">
        <input
          type="checkbox"
          className="h-3.5 w-3.5"
          checked={timer.billable}
          onChange={(e) => useTimer.setState({ billable: e.target.checked })}
        />
        Billable
      </label>
      {error && <p className="text-xs text-destructive">{error}</p>}
      <button
        onClick={() => stopAndSave.mutate()}
        disabled={stopAndSave.isPending}
        className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        <Pause className="h-3.5 w-3.5" />
        {stopAndSave.isPending ? 'Saving…' : 'Stop and log'}
      </button>
    </div>
  )
}

function StartPanel({ person: defaultPerson, onClose }: { person: string; onClose: () => void }) {
  const projects = useProjectOptions()
  const [projectId, setProjectId] = useState('')
  const taskOpts = useTaskOptionsForProject(projectId)
  const [taskId, setTaskId] = useState('')
  const [description, setDescription] = useState('')
  const [person, setPerson] = useState(defaultPerson)
  const [billable, setBillable] = useState(true)

  const projectLabel =
    projects.data?.find((o) => o.ID === projectId)?.Label ?? ''
  const taskLabel =
    taskOpts.data?.find((o) => o.ID === taskId)?.Label ?? ''

  return (
    <div className="p-4 space-y-3">
      <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        Start timer
      </p>
      <Field label="Project">
        <select
          className="flex h-9 w-full rounded-md border border-input bg-background px-2 text-sm"
          value={projectId}
          onChange={(e) => {
            setProjectId(e.target.value)
            setTaskId('')
          }}
        >
          <option value="">{projects.isLoading ? 'loading…' : '— pick —'}</option>
          {(projects.data ?? []).map((p) => (
            <option key={p.ID} value={p.ID}>{p.Label}</option>
          ))}
        </select>
      </Field>
      {projectId && (taskOpts.data ?? []).length > 0 && (
        <Field label="Task (optional)">
          <select
            className="flex h-9 w-full rounded-md border border-input bg-background px-2 text-sm"
            value={taskId}
            onChange={(e) => setTaskId(e.target.value)}
          >
            <option value="">— none —</option>
            {(taskOpts.data ?? []).map((t) => (
              <option key={t.ID} value={t.ID}>{t.Label}</option>
            ))}
          </select>
        </Field>
      )}
      <Field label="Description">
        <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="What you'll work on" />
      </Field>
      <Field label="Logged by">
        <Input value={person} onChange={(e) => setPerson(e.target.value)} />
      </Field>
      <label className="flex items-center gap-2 text-xs text-muted-foreground">
        <input
          type="checkbox"
          className="h-3.5 w-3.5"
          checked={billable}
          onChange={(e) => setBillable(e.target.checked)}
        />
        Billable
      </label>
      <button
        disabled={!projectId || !person}
        onClick={() => {
          useTimer.getState().start({
            projectId,
            projectLabel,
            taskId: taskId || undefined,
            taskLabel: taskLabel || undefined,
            description,
            person,
            billable,
          })
          onClose()
        }}
        className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        <Play className="h-3.5 w-3.5" />
        Start
      </button>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <label className="text-xs text-muted-foreground">{label}</label>
      {children}
    </div>
  )
}

// Reuses the existing field-options endpoint (reference label lookup) against
// the time_entries.project field, which gives us {ID, Label} pairs of projects.
function useProjectOptions() {
  return useFieldOptions('time_entries', 'project')
}

// Fetches all tasks via listRows and filters to the selected project on the client.
function useTaskOptionsForProject(projectId: string) {
  return useQuery<RefOption[]>({
    queryKey: ['tasks-for-project', projectId],
    enabled: Boolean(projectId),
    staleTime: 30_000,
    queryFn: async () => {
      const res = await api.listRows('tasks', { sort: 'name', dir: 'asc', limit: 200 })
      const rows = res.rows.filter((r) => r.project === projectId)
      return rows.map((r) => ({
        ID: String(r.id),
        Label: String(r.name ?? r.id),
      }))
    },
  })
}
