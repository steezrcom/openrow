import { create } from 'zustand'
import { QueryClient } from '@tanstack/react-query'

export interface ChatAction {
  tool: string
  input: unknown
  summary: string
  entity_name?: string
  error?: string
}

export interface ChatTurn {
  role: 'user' | 'assistant'
  text: string
  actions?: ChatAction[]
}

// Server-sent stream events from POST /api/v1/chat/messages/stream.
// Each SSE `data:` line carries one of these.
interface StreamEvent {
  type: 'text_delta' | 'tool_start' | 'tool_end' | 'done' | 'error'
  delta?: string
  tool?: ChatAction
  text?: string
  actions?: ChatAction[]
  message?: string
}

interface ChatStore {
  open: boolean
  setOpen: (v: boolean | ((prev: boolean) => boolean)) => void
  toggle: () => void
  history: ChatTurn[]
  streaming: boolean
  send: (message: string, qc: QueryClient) => Promise<void>
  clear: () => void
}

const MUTATING_TOOLS = new Set([
  'create_entity',
  'add_row',
  'update_row',
  'delete_row',
  'add_field',
  'drop_field',
  'create_dashboard',
  'add_report',
  'update_report',
  'delete_report',
  'delete_dashboard',
  'apply_template',
])

export const useChatStore = create<ChatStore>((set, get) => ({
  open: false,
  setOpen: (v) =>
    set((s) => ({ open: typeof v === 'function' ? v(s.open) : v })),
  toggle: () => set((s) => ({ open: !s.open })),
  history: [],
  streaming: false,
  clear: () => set({ history: [], streaming: false }),

  send: async (message, qc) => {
    // Push the user message + a placeholder assistant turn we'll fill as
    // stream events arrive.
    const currentHistory = get().history
    const wireHistory = currentHistory.map((t) => ({ role: t.role, text: t.text }))
    set({
      history: [
        ...currentHistory,
        { role: 'user', text: message },
        { role: 'assistant', text: '', actions: [] },
      ],
      streaming: true,
    })

    function updateLast(mutator: (t: ChatTurn) => ChatTurn) {
      set((s) => {
        const last = s.history[s.history.length - 1]
        if (!last || last.role !== 'assistant') return s
        return { history: [...s.history.slice(0, -1), mutator(last)] }
      })
    }

    let sawMutation = false

    function handle(ev: StreamEvent) {
      switch (ev.type) {
        case 'text_delta':
          updateLast((t) => ({ ...t, text: (t.text ?? '') + (ev.delta ?? '') }))
          break
        case 'tool_start':
          if (!ev.tool) break
          updateLast((t) => ({
            ...t,
            actions: [
              ...(t.actions ?? []),
              { ...ev.tool!, summary: ev.tool!.summary || '…' },
            ],
          }))
          break
        case 'tool_end':
          if (!ev.tool) break
          if (MUTATING_TOOLS.has(ev.tool.tool)) sawMutation = true
          updateLast((t) => {
            const actions = [...(t.actions ?? [])]
            // Replace the last in-flight stub for this tool. Stubs have
            // summary '…' from tool_start; completed ones carry the real summary.
            for (let i = actions.length - 1; i >= 0; i--) {
              if (actions[i].summary === '…') {
                actions[i] = ev.tool!
                break
              }
            }
            return { ...t, actions }
          })
          break
        case 'done':
          updateLast((t) => ({
            ...t,
            text: ev.text ?? t.text,
            actions: ev.actions ?? t.actions,
          }))
          break
        case 'error':
          updateLast((t) => ({
            ...t,
            text: t.text + (t.text ? '\n\n' : '') + `Error: ${ev.message ?? 'failed'}`,
          }))
          break
      }
    }

    try {
      const resp = await fetch('/api/v1/chat/messages/stream', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ history: wireHistory, message }),
      })
      if (!resp.ok || !resp.body) {
        const text = await resp.text().catch(() => '')
        throw new Error(`chat failed (${resp.status}): ${text || resp.statusText}`)
      }
      const reader = resp.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        // SSE messages are separated by blank lines (\n\n).
        let idx = buf.indexOf('\n\n')
        while (idx !== -1) {
          const raw = buf.slice(0, idx)
          buf = buf.slice(idx + 2)
          idx = buf.indexOf('\n\n')
          // Lines within an event; we only emit one `data: {json}` per event.
          const dataLines = raw
            .split('\n')
            .filter((l) => l.startsWith('data:'))
            .map((l) => l.slice(5).trim())
          if (dataLines.length === 0) continue
          const payload = dataLines.join('')
          try {
            handle(JSON.parse(payload) as StreamEvent)
          } catch {
            // ignore malformed events
          }
        }
      }
    } catch (err) {
      updateLast((t) => ({
        ...t,
        text: `Error: ${err instanceof Error ? err.message : 'stream failed'}`,
      }))
    } finally {
      set({ streaming: false })
      if (sawMutation) {
        await qc.invalidateQueries({ queryKey: ['entities'] })
        await qc.invalidateQueries({ queryKey: ['rows'] })
        await qc.invalidateQueries({ queryKey: ['dashboards'] })
        await qc.invalidateQueries({ queryKey: ['dashboard'] })
        await qc.invalidateQueries({ queryKey: ['report-exec'] })
      }
    }
  },
}))
