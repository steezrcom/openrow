package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/llm"
)

func (s *Server) listLLMProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"providers": llm.Providers})
}

func (s *Server) getLLMConfig(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	safe, err := s.llm.GetSafe(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"config": safe})
}

type putLLMConfigReq struct {
	Provider string  `json:"provider"`
	BaseURL  string  `json:"base_url"`
	APIKey   *string `json:"api_key,omitempty"`
	Model    string  `json:"model"`
}

func (s *Server) putLLMConfig(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in putLLMConfigReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	safe, err := s.llm.Set(r.Context(), m.TenantID, llm.SetInput{
		Provider: in.Provider,
		BaseURL:  in.BaseURL,
		APIKey:   in.APIKey,
		Model:    in.Model,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"config": safe})
}

func (s *Server) deleteLLMConfig(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	if err := s.llm.Delete(r.Context(), m.TenantID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listLLMModels calls the provider's /v1/models using the supplied base_url +
// api_key from the request body (not the saved config), so the settings UI can
// preview models before the user commits. When the body's api_key is empty or
// the "__keep__" sentinel, fall back to the tenant's saved key — the UI uses
// the sentinel to mean "use whatever is stored" so it doesn't have to ask the
// user to re-paste the key for every probe.
type listModelsReq struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

func (s *Server) listLLMModels(w http.ResponseWriter, r *http.Request) {
	var in listModelsReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	apiKey, err := s.resolveLLMKey(r, in.APIKey)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	baseURL := in.BaseURL
	if baseURL == "" {
		// Frontend may pass empty when probing the saved config.
		if m, ok := auth.MembershipFromContext(r.Context()); ok {
			if cfg, err := s.llm.Resolve(r.Context(), m.TenantID); err == nil {
				baseURL = cfg.BaseURL
			}
		}
	}
	models, err := llm.ListModels(r.Context(), baseURL, apiKey)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"models": models})
}

type testLLMReq struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// testLLM runs a three-stage probe (models, chat, tool call) against the
// supplied creds. Returns a structured result so the UI can show which stages
// passed: "chat works but tool calling doesn't" is a valid, actionable state.
// Same "__keep__" sentinel handling as listLLMModels.
func (s *Server) testLLM(w http.ResponseWriter, r *http.Request) {
	var in testLLMReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	apiKey, err := s.resolveLLMKey(r, in.APIKey)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	baseURL := in.BaseURL
	model := in.Model
	if (baseURL == "" || model == "") {
		if m, ok := auth.MembershipFromContext(r.Context()); ok {
			if cfg, err := s.llm.Resolve(r.Context(), m.TenantID); err == nil {
				if baseURL == "" {
					baseURL = cfg.BaseURL
				}
				if model == "" {
					model = cfg.Model
				}
			}
		}
	}
	result := llm.Test(r.Context(), baseURL, apiKey, model)
	writeJSON(w, http.StatusOK, map[string]interface{}{"result": result})
}

// resolveLLMKey returns the API key to use for an ad-hoc probe. If the
// caller passed a real key in the body, that wins. If they passed the
// "__keep__" sentinel (or empty), we resolve the tenant's saved key —
// requires an active membership. The sentinel is what the settings UI
// sends when the user is probing the stored config without retyping
// the key.
const llmKeepSentinel = "__keep__"

func (s *Server) resolveLLMKey(r *http.Request, raw string) (string, error) {
	if raw != "" && raw != llmKeepSentinel {
		return raw, nil
	}
	m, ok := auth.MembershipFromContext(r.Context())
	if !ok {
		return "", errAuth("api_key required (no active workspace to resolve a saved key)")
	}
	cfg, err := s.llm.Resolve(r.Context(), m.TenantID)
	if err != nil {
		return "", errAuth("no saved api key for this workspace")
	}
	if cfg.APIKey == "" {
		return "", errAuth("saved config has no api key")
	}
	return cfg.APIKey, nil
}

type httpError struct {
	msg string
}

func (e *httpError) Error() string { return e.msg }
func errAuth(msg string) error     { return &httpError{msg: msg} }

// selfTestLLM runs the same probe against the tenant's saved config and
// persists the outcome on the row. Used when the settings form isn't dirty
// and the user just wants to verify the saved config still works.
func (s *Server) selfTestLLM(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	result, err := s.llm.SelfTest(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"result": result})
}
