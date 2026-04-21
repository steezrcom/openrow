import { create } from 'zustand'

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

interface ChatStore {
  open: boolean
  setOpen: (v: boolean | ((prev: boolean) => boolean)) => void
  toggle: () => void
  history: ChatTurn[]
  pushUser: (text: string) => void
  pushAssistant: (turn: ChatTurn) => void
  clear: () => void
}

export const useChatStore = create<ChatStore>((set) => ({
  open: false,
  setOpen: (v) =>
    set((s) => ({ open: typeof v === 'function' ? v(s.open) : v })),
  toggle: () => set((s) => ({ open: !s.open })),
  history: [],
  pushUser: (text) =>
    set((s) => ({ history: [...s.history, { role: 'user', text }] })),
  pushAssistant: (turn) => set((s) => ({ history: [...s.history, turn] })),
  clear: () => set({ history: [] }),
}))
