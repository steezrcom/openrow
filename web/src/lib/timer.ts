import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export interface TimerState {
  running: boolean
  startedAt: number | null
  projectId: string | null
  projectLabel: string | null
  taskId: string | null
  taskLabel: string | null
  description: string
  person: string
  billable: boolean
  start: (input: {
    projectId: string
    projectLabel: string
    taskId?: string
    taskLabel?: string
    description?: string
    person: string
    billable?: boolean
  }) => void
  stop: () => {
    startedAt: number
    endedAt: number
    hours: number
    projectId: string
    taskId: string | null
    description: string
    person: string
    billable: boolean
  } | null
  cancel: () => void
  updateDescription: (description: string) => void
}

export const useTimer = create<TimerState>()(
  persist(
    (set, get) => ({
      running: false,
      startedAt: null,
      projectId: null,
      projectLabel: null,
      taskId: null,
      taskLabel: null,
      description: '',
      person: '',
      billable: true,
      start: (input) =>
        set({
          running: true,
          startedAt: Date.now(),
          projectId: input.projectId,
          projectLabel: input.projectLabel,
          taskId: input.taskId ?? null,
          taskLabel: input.taskLabel ?? null,
          description: input.description ?? '',
          person: input.person,
          billable: input.billable ?? true,
        }),
      stop: () => {
        const s = get()
        if (!s.running || !s.startedAt || !s.projectId) return null
        const endedAt = Date.now()
        const hours = round2((endedAt - s.startedAt) / 3_600_000)
        const out = {
          startedAt: s.startedAt,
          endedAt,
          hours,
          projectId: s.projectId,
          taskId: s.taskId,
          description: s.description,
          person: s.person,
          billable: s.billable,
        }
        set({
          running: false,
          startedAt: null,
          projectId: null,
          projectLabel: null,
          taskId: null,
          taskLabel: null,
          description: '',
        })
        return out
      },
      cancel: () =>
        set({
          running: false,
          startedAt: null,
          projectId: null,
          projectLabel: null,
          taskId: null,
          taskLabel: null,
          description: '',
        }),
      updateDescription: (description) => set({ description }),
    }),
    {
      name: 'steezr.timer',
      version: 1,
    }
  )
)

function round2(n: number) {
  return Math.round(n * 100) / 100
}

export function formatElapsed(startedAt: number, now: number): string {
  const ms = Math.max(0, now - startedAt)
  const s = Math.floor(ms / 1000)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(h)}:${pad(m)}:${pad(sec)}`
}
