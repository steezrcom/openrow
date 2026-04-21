package llm

// Provider is a hardcoded preset. Users can still pick "custom" and supply a
// raw base URL; the OpenAI-format client handles them the same way.
type Provider struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	BaseURL        string `json:"base_url"`
	DefaultModel   string `json:"default_model"`
	RequiresAPIKey bool   `json:"requires_api_key"`
	Local          bool   `json:"local,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

// Providers is the ordered list we show to users in the settings UI.
// Order roughly follows popularity + expected setup simplicity.
var Providers = []Provider{
	{
		ID:             "openai",
		Name:           "OpenAI",
		BaseURL:        "https://api.openai.com/v1",
		DefaultModel:   "gpt-4o",
		RequiresAPIKey: true,
	},
	{
		ID:             "anthropic",
		Name:           "Anthropic (Claude)",
		BaseURL:        "https://api.anthropic.com/v1",
		DefaultModel:   "claude-sonnet-4-5",
		RequiresAPIKey: true,
		Notes:          "Uses Anthropic's OpenAI-compatibility endpoint. Tool calling supported.",
	},
	{
		ID:             "openrouter",
		Name:           "OpenRouter",
		BaseURL:        "https://openrouter.ai/api/v1",
		DefaultModel:   "anthropic/claude-sonnet-4-5",
		RequiresAPIKey: true,
		Notes:          "Aggregator. Access 300+ models with one key.",
	},
	{
		ID:             "groq",
		Name:           "Groq",
		BaseURL:        "https://api.groq.com/openai/v1",
		DefaultModel:   "llama-3.3-70b-versatile",
		RequiresAPIKey: true,
	},
	{
		ID:             "together",
		Name:           "Together AI",
		BaseURL:        "https://api.together.xyz/v1",
		DefaultModel:   "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		RequiresAPIKey: true,
	},
	{
		ID:             "deepseek",
		Name:           "DeepSeek",
		BaseURL:        "https://api.deepseek.com/v1",
		DefaultModel:   "deepseek-chat",
		RequiresAPIKey: true,
	},
	{
		ID:             "gemini",
		Name:           "Google Gemini",
		BaseURL:        "https://generativelanguage.googleapis.com/v1beta/openai",
		DefaultModel:   "gemini-2.0-flash",
		RequiresAPIKey: true,
	},
	{
		ID:             "ollama",
		Name:           "Ollama (local)",
		BaseURL:        "http://localhost:11434/v1",
		DefaultModel:   "llama3.1:8b",
		RequiresAPIKey: false,
		Local:          true,
		Notes:          "Point at your Ollama server. Tool calling works with Llama 3.1+, Qwen 2.5+, Mistral Nemo. Smaller models struggle with the tool schema.",
	},
	{
		ID:             "lmstudio",
		Name:           "LM Studio (local)",
		BaseURL:        "http://localhost:1234/v1",
		DefaultModel:   "",
		RequiresAPIKey: false,
		Local:          true,
	},
	{
		ID:             "vllm",
		Name:           "vLLM / LocalAI / custom OpenAI-compat",
		BaseURL:        "",
		DefaultModel:   "",
		RequiresAPIKey: false,
		Local:          true,
		Notes:          "Any server implementing /v1/chat/completions. Supply the full base URL.",
	},
	{
		ID:             "custom",
		Name:           "Custom endpoint",
		BaseURL:        "",
		DefaultModel:   "",
		RequiresAPIKey: false,
	},
}

func GetProvider(id string) (*Provider, bool) {
	for i := range Providers {
		if Providers[i].ID == id {
			return &Providers[i], true
		}
	}
	return nil, false
}
