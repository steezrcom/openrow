import { useEffect, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ArrowUp,
  Check,
  Loader2,
  MessageSquare,
  Sparkles,
  Trash2,
  X,
  Wrench,
} from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { useChatStore, type ChatAction } from '@/lib/chat'
import { cn } from '@/lib/utils'

const PROMPT_SUGGESTIONS = [
  'List my entities',
  'Add a test customer',
  'How many customers do I have?',
]

export function ChatPanel() {
  const open = useChatStore((s) => s.open)
  const setOpen = useChatStore((s) => s.setOpen)
  const toggle = useChatStore((s) => s.toggle)

  useEffect(() => {
    function handler(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === '.') {
        e.preventDefault()
        toggle()
      }
      if (e.key === 'Escape' && open) setOpen(false)
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [toggle, open, setOpen])

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="fixed bottom-6 right-6 z-30 flex items-center gap-2 rounded-full border border-border bg-card px-4 py-2.5 text-sm shadow-lg hover:bg-accent"
      >
        <Sparkles className="h-4 w-4 text-primary" />
        Ask Claude
        <kbd className="ml-1 rounded border border-border bg-muted/60 px-1 font-mono text-[10px] text-muted-foreground">⌘.</kbd>
      </button>
    )
  }

  return <ChatOpen onClose={() => setOpen(false)} />
}

function ChatOpen({ onClose }: { onClose: () => void }) {
  const history = useChatStore((s) => s.history)
  const pushUser = useChatStore((s) => s.pushUser)
  const pushAssistant = useChatStore((s) => s.pushAssistant)
  const clear = useChatStore((s) => s.clear)

  const qc = useQueryClient()
  const scrollRef = useRef<HTMLDivElement>(null)

  const { register, handleSubmit, reset, formState: { isSubmitting }, setValue, watch } =
    useForm<{ message: string }>()
  const messageValue = watch('message') || ''

  const send = useMutation({
    mutationFn: (message: string) =>
      api.chat({
        history: history.map((t) => ({ role: t.role, text: t.text })),
        message,
      }),
    onMutate: (message) => pushUser(message),
    onSuccess: async (res) => {
      pushAssistant(res.assistant)
      if (res.assistant.actions?.some(mutates)) {
        await qc.invalidateQueries({ queryKey: ['entities'] })
        await qc.invalidateQueries({ queryKey: ['rows'] })
        await qc.invalidateQueries({ queryKey: ['dashboards'] })
        await qc.invalidateQueries({ queryKey: ['dashboard'] })
        await qc.invalidateQueries({ queryKey: ['report-exec'] })
      }
    },
    onError: (err) => {
      pushAssistant({
        role: 'assistant',
        text:
          err instanceof ApiError
            ? `Error: ${err.message}`
            : 'Something went wrong.',
      })
    },
  })

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [history.length, send.isPending])

  return (
    <aside className="sticky top-0 flex h-screen w-[400px] shrink-0 flex-col border-l border-border bg-card">
      <header className="flex items-center justify-between border-b border-border px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-semibold">
          <Sparkles className="h-4 w-4 text-primary" /> Claude
        </div>
        <div className="flex items-center gap-1">
          {history.length > 0 && (
            <button
              onClick={() => {
                if (confirm('Clear this conversation?')) clear()
              }}
              className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
              title="Clear conversation"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          )}
          <button
            onClick={onClose}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
            title="Close (Esc)"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </header>

      <div ref={scrollRef} className="flex-1 overflow-y-auto px-4 py-5">
        {history.length === 0 && (
          <div className="text-sm text-muted-foreground">
            <div className="mb-2 flex items-center gap-2 text-foreground">
              <MessageSquare className="h-4 w-4" /> Chat with your database
            </div>
            <p>Design entities, add records, run queries, clean up — all in natural language.</p>
            <div className="mt-5 flex flex-col gap-1.5">
              {PROMPT_SUGGESTIONS.map((s) => (
                <button
                  key={s}
                  onClick={() => setValue('message', s)}
                  className="rounded-md border border-border bg-background px-3 py-2 text-left text-xs hover:bg-accent"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}

        <div className="space-y-5">
          {history.map((turn, i) => (
            <Message key={i} turn={turn} />
          ))}
          {send.isPending && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              Thinking.
            </div>
          )}
        </div>
      </div>

      <form
        className="border-t border-border p-3"
        onSubmit={handleSubmit((v) => {
          const m = v.message.trim()
          if (!m) return
          reset()
          send.mutate(m)
        })}
      >
        <div className="relative">
          <textarea
            {...register('message', { required: true })}
            placeholder="Ask Claude to do anything… (Enter to send)"
            className={cn(
              'min-h-[72px] w-full resize-none rounded-md border border-input bg-background px-3 py-2 pr-12 text-sm',
              'placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
            )}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                e.currentTarget.form?.requestSubmit()
              }
            }}
          />
          <button
            type="submit"
            disabled={isSubmitting || send.isPending || !messageValue.trim()}
            className="absolute bottom-2 right-2 inline-flex h-8 w-8 items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            aria-label="Send"
          >
            <ArrowUp className="h-4 w-4" />
          </button>
        </div>
        <p className="mt-1.5 text-[10px] text-muted-foreground">
          Claude can make mistakes. Verify destructive actions.
        </p>
      </form>
    </aside>
  )
}

function Message({ turn }: { turn: { role: 'user' | 'assistant'; text: string; actions?: ChatAction[] } }) {
  if (turn.role === 'user') {
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%] rounded-lg bg-primary/10 px-3 py-2 text-sm">
          {turn.text}
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      {turn.actions && turn.actions.length > 0 && (
        <div className="space-y-1">
          {turn.actions.map((a, i) => <ActionPill key={i} action={a} />)}
        </div>
      )}
      {turn.text && (
        <div className="whitespace-pre-wrap text-sm text-foreground">{turn.text}</div>
      )}
    </div>
  )
}

function ActionPill({ action }: { action: ChatAction }) {
  const failed = Boolean(action.error)
  return (
    <div
      className={cn(
        'flex items-start gap-2 rounded-md border px-2 py-1.5 text-xs',
        failed
          ? 'border-destructive/30 bg-destructive/5 text-destructive'
          : 'border-border bg-muted/30 text-muted-foreground'
      )}
    >
      {failed ? <Wrench className="mt-0.5 h-3 w-3 shrink-0" /> : <Check className="mt-0.5 h-3 w-3 shrink-0 text-primary" />}
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium">
          <span className="font-mono text-foreground/80">{action.tool}</span>
          {action.entity_name && <span className="text-muted-foreground"> · {action.entity_name}</span>}
        </div>
        {action.summary && <div className="truncate">{action.summary}</div>}
        {action.error && <div className="truncate">{action.error}</div>}
      </div>
    </div>
  )
}

function mutates(a: ChatAction): boolean {
  switch (a.tool) {
    case 'create_entity':
    case 'add_row':
    case 'update_row':
    case 'delete_row':
    case 'add_field':
    case 'drop_field':
    case 'create_dashboard':
    case 'add_report':
    case 'update_report':
    case 'delete_report':
    case 'delete_dashboard':
      return true
  }
  return false
}
