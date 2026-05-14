import { createFileRoute } from '@tanstack/react-router'
import { Callout, DocPage, Section, Step, Steps } from '@/components/DocsLayout'

export const Route = createFileRoute('/docs/llm')({
  component: LLMSetup,
})

type Provider = {
  id: string
  name: string
  url: string
  tier: 'cloud' | 'cloud-fast' | 'cloud-cheap' | 'local'
  recommended: string
  notes: string
}

const providers: Provider[] = [
  {
    id: 'anthropic',
    name: 'Anthropic',
    url: 'https://console.anthropic.com',
    tier: 'cloud',
    recommended: 'Claude Sonnet 4.6',
    notes:
      "The default. Best tool-calling discipline in the catalog and the model openrow is most thoroughly tested against. Use Sonnet 4.6 for everything; Opus only if you need the extra reasoning headroom and don't mind the price.",
  },
  {
    id: 'openai',
    name: 'OpenAI',
    url: 'https://platform.openai.com',
    tier: 'cloud',
    recommended: 'GPT-4o or GPT-4o-mini',
    notes:
      'Reliable for tool calling. GPT-4o is the safe default; 4o-mini works well for flows with narrow tool sets and saves real money on high-frequency cron flows.',
  },
  {
    id: 'gemini',
    name: 'Google Gemini',
    url: 'https://aistudio.google.com',
    tier: 'cloud-fast',
    recommended: 'Gemini 2.0 Flash',
    notes:
      'Fast and cheap. 2.0 Flash is solid for tool calling; the Pro models give better reasoning at higher cost.',
  },
  {
    id: 'groq',
    name: 'Groq',
    url: 'https://console.groq.com',
    tier: 'cloud-fast',
    recommended: 'Llama 3.3 70B',
    notes:
      'Cloud inference on custom silicon — typically 5-10× faster than the nearest equivalent. Great for low-latency flows. Tool calling is reliable on Llama 3.3 70B and above.',
  },
  {
    id: 'together',
    name: 'Together AI',
    url: 'https://api.together.xyz',
    tier: 'cloud-cheap',
    recommended: 'Llama 3.3 70B Instruct',
    notes:
      'Cheap cloud inference on a broad model catalog. Same caveat as Groq — pick Llama 3.3 70B+ for tool-calling reliability.',
  },
  {
    id: 'deepseek',
    name: 'DeepSeek',
    url: 'https://platform.deepseek.com',
    tier: 'cloud-cheap',
    recommended: 'deepseek-chat',
    notes:
      'Inexpensive and surprisingly competent at structured tasks. Watch for it occasionally calling the wrong tool when many similar ones are available.',
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    url: 'https://openrouter.ai',
    tier: 'cloud',
    recommended: 'Pick your model',
    notes:
      'A single key + endpoint that fronts every major provider. Pay one bill, switch models without changing the openrow config. Useful for trialling models before committing.',
  },
  {
    id: 'ollama',
    name: 'Ollama',
    url: 'https://ollama.com',
    tier: 'local',
    recommended: 'qwen2.5:14b or llama3.1:8b+',
    notes:
      "Best path for local. Run ollama, pull a model that's at least 7-8B, point openrow at http://localhost:11434/v1. Anything smaller drops tool-call structure and behaves erratically.",
  },
  {
    id: 'lmstudio',
    name: 'LM Studio',
    url: 'https://lmstudio.ai',
    tier: 'local',
    recommended: 'Same constraints as Ollama',
    notes:
      'GUI-driven local inference. Exposes an OpenAI-compatible server at http://localhost:1234/v1. Useful if you prefer not to use the terminal.',
  },
  {
    id: 'custom',
    name: 'Custom / any OpenAI-compatible',
    url: '',
    tier: 'cloud',
    recommended: 'whatever the server supports',
    notes:
      "Drop-in for any server that speaks /v1/chat/completions: vLLM, llama.cpp's llama-server, LocalAI, Jan, Open WebUI, LiteLLM. Pick Custom in the provider list and paste the base URL.",
  },
]

function LLMSetup() {
  return (
    <DocPage
      kicker="Bring your own key"
      title="LLM setup"
      lede="openrow doesn't sell tokens. Every workspace plugs in its own provider — the keys are encrypted at rest, and your usage bills land in your provider's account."
    >
      <Section id="picking" title="Picking a provider">
        <p className="text-muted-foreground">
          The agent uses tool calling for everything — entity creation, row edits, dashboard updates, connector
          actions. Not every model handles tool calling well. The shortlist below is what we know works:
        </p>
        <div className="overflow-hidden rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead className="bg-muted/30 text-xs uppercase tracking-wider text-muted-foreground">
              <tr>
                <th className="px-3 py-2 text-left font-medium">Provider</th>
                <th className="px-3 py-2 text-left font-medium">Tier</th>
                <th className="px-3 py-2 text-left font-medium">Recommended model</th>
                <th className="px-3 py-2 text-left font-medium">Notes</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {providers.map((p) => (
                <tr key={p.id} className="align-top">
                  <td className="whitespace-nowrap px-3 py-3 font-medium">
                    {p.url ? (
                      <a href={p.url} target="_blank" rel="noreferrer" className="text-foreground hover:text-primary">
                        {p.name}
                      </a>
                    ) : (
                      p.name
                    )}
                  </td>
                  <td className="px-3 py-3">
                    <TierBadge tier={p.tier} />
                  </td>
                  <td className="whitespace-nowrap px-3 py-3 text-muted-foreground">{p.recommended}</td>
                  <td className="px-3 py-3 text-muted-foreground">{p.notes}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <Section id="setup" title="Setup">
        <Steps>
          <Step title="Get a key from your provider">
            Anthropic → console.anthropic.com → Settings → API Keys → Create Key. OpenAI → platform.openai.com →
            API keys → Create. The other providers all have an equivalent.
          </Step>
          <Step title="Open Settings → LLM">
            In openrow, click the workspace menu, pick Settings, then the LLM tab.
          </Step>
          <Step title="Pick the provider preset">
            Picks the right base URL and any provider-specific quirks. Use Custom for anything not on the list.
          </Step>
          <Step title="Paste the key and click Fetch models">
            openrow calls the provider's models endpoint with your key. The dropdown fills with the ones you
            have access to.
          </Step>
          <Step title="Pick a model and click Test connection">
            Test connection fires a synthetic tool-call to confirm the model not only chats but actually picks
            tools when asked. A green tick means you're good to go.
          </Step>
          <Step title="Save">
            The key is encrypted at rest with the deployment's root key before it touches Postgres.
          </Step>
        </Steps>
      </Section>

      <Section id="tool-calls" title="Why tool-calling reliability matters">
        <p className="text-muted-foreground">
          openrow's agent is a planner with a strict tool surface. Every meaningful action it takes — creating a
          column, adding a row, drafting a flow — is a JSON-schema-typed tool call. If the model drifts (calls
          the wrong tool, omits a required argument, hallucinates a tool name), the operation fails and the user
          sees a confusing error.
        </p>
        <p className="text-muted-foreground">
          Models smaller than ~7B parameters mis-call tools frequently. Models around 8-14B are usable for narrow
          tool sets but get worse as the tool catalogue grows (a fully-loaded workspace exposes 60+ tools). For
          comfortable use across the whole app, pick a frontier-class model.
        </p>
      </Section>

      <Section id="docker" title="Local models, openrow in Docker">
        <p className="text-muted-foreground">
          If openrow runs in Docker and your local LLM server runs on the host:
        </p>
        <ul className="list-disc space-y-1 pl-5 text-muted-foreground">
          <li>
            <strong>macOS / Windows:</strong> point the base URL at{' '}
            <code>http://host.docker.internal:11434/v1</code>.
          </li>
          <li>
            <strong>Linux:</strong> add <code>network_mode: host</code> to the app service in{' '}
            <code>docker-compose.app.yml</code>, or bind your LLM to <code>0.0.0.0</code> and use the host's LAN IP.
          </li>
        </ul>
        <Callout tone="warn" title="Local inference is slow.">
          The HTTP client waits up to 180s per turn. Flows on local models that take six tool calls per run can
          take real minutes. Fine for a once-a-day cron; not great for row-trigger flows on a busy entity.
        </Callout>
      </Section>

      <Section id="fallback" title="Workspace key vs. server fallback">
        <p className="text-muted-foreground">
          Self-hosters can set <code>ANTHROPIC_API_KEY</code> in the server's env as a fallback for workspaces
          that haven't configured an LLM yet. Useful for trying things out — but in production, every workspace
          should bring its own key so cost is attributed and rate limits don't bleed across tenants.
        </p>
      </Section>
    </DocPage>
  )
}

function TierBadge({ tier }: { tier: Provider['tier'] }) {
  const map = {
    cloud: { label: 'Cloud', class: 'bg-primary/10 text-primary' },
    'cloud-fast': { label: 'Cloud · fast', class: 'bg-sky-500/15 text-sky-700 dark:text-sky-300' },
    'cloud-cheap': { label: 'Cloud · cheap', class: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300' },
    local: { label: 'Local', class: 'bg-muted text-muted-foreground' },
  } as const
  const m = map[tier]
  return (
    <span className={`inline-flex rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider ${m.class}`}>
      {m.label}
    </span>
  )
}
