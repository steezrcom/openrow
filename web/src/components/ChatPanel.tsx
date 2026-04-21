import { useEffect, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useQueryClient } from '@tanstack/react-query'
import {
  ArrowUp,
  Check,
  Loader2,
  MessageSquare,
  Sparkles,
  Trash2,
  Wrench,
  X,
} from 'lucide-react'
import { useChatStore, type ChatAction } from '@/lib/chat'
import { Markdown } from '@/components/Markdown'
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
        Ask the assistant
        <kbd className="ml-1 rounded border border-border bg-muted/60 px-1 font-mono text-[10px] text-muted-foreground">⌘.</kbd>
      </button>
    )
  }

  return <ChatOpen onClose={() => setOpen(false)} />
}

function ChatOpen({ onClose }: { onClose: () => void }) {
  const history = useChatStore((s) => s.history)
  const streaming = useChatStore((s) => s.streaming)
  const clear = useChatStore((s) => s.clear)
  const send = useChatStore((s) => s.send)

  const qc = useQueryClient()
  const scrollRef = useRef<HTMLDivElement>(null)

  const { register, handleSubmit, reset, setValue, watch } = useForm<{ message: string }>()
  const messageValue = watch('message') || ''

  // Auto-scroll to the bottom whenever new content arrives.
  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [history.length, history[history.length - 1]?.text, streaming])

  return (
    <aside className="sticky top-0 flex h-screen w-[400px] shrink-0 flex-col border-l border-border bg-card">
      <header className="flex items-center justify-between border-b border-border px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-semibold">
          <Sparkles className="h-4 w-4 text-primary" /> Assistant
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
            <Message
              key={i}
              turn={turn}
              isLastAssistant={
                i === history.length - 1 && turn.role === 'assistant' && streaming
              }
            />
          ))}
          {streaming && history[history.length - 1]?.text === '' && (
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
          void send(m, qc)
        })}
      >
        <div className="relative">
          <textarea
            {...register('message', { required: true })}
            placeholder="Ask the assistant to do anything… (Enter to send)"
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
            disabled={streaming || !messageValue.trim()}
            className="absolute bottom-2 right-2 inline-flex h-8 w-8 items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            aria-label="Send"
          >
            <ArrowUp className="h-4 w-4" />
          </button>
        </div>
        <p className="mt-1.5 text-[10px] text-muted-foreground">
          The assistant can make mistakes. Verify destructive actions.
        </p>
      </form>
    </aside>
  )
}

function Message({
  turn,
  isLastAssistant,
}: {
  turn: { role: 'user' | 'assistant'; text: string; actions?: ChatAction[] }
  isLastAssistant?: boolean
}) {
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
      {turn.text && <Markdown>{turn.text}</Markdown>}
      {isLastAssistant && (
        <span className="inline-block h-3 w-[2px] animate-pulse bg-muted-foreground/60" />
      )}
    </div>
  )
}

function ActionPill({ action }: { action: ChatAction }) {
  const failed = Boolean(action.error)
  const pending = action.summary === '…'
  return (
    <div
      className={cn(
        'flex items-start gap-2 rounded-md border px-2 py-1.5 text-xs',
        failed
          ? 'border-destructive/30 bg-destructive/5 text-destructive'
          : pending
          ? 'border-border bg-muted/30 text-muted-foreground'
          : 'border-border bg-muted/30 text-muted-foreground'
      )}
    >
      {pending ? (
        <Loader2 className="mt-0.5 h-3 w-3 shrink-0 animate-spin text-muted-foreground" />
      ) : failed ? (
        <Wrench className="mt-0.5 h-3 w-3 shrink-0" />
      ) : (
        <Check className="mt-0.5 h-3 w-3 shrink-0 text-primary" />
      )}
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium">
          <span className="font-mono text-foreground/80">{action.tool}</span>
          {action.entity_name && <span className="text-muted-foreground"> · {action.entity_name}</span>}
        </div>
        {action.summary && action.summary !== '…' && <div className="truncate">{action.summary}</div>}
        {action.error && <div className="truncate">{action.error}</div>}
      </div>
    </div>
  )
}
